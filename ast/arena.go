package ast

// Arena bump-allocates AST nodes for every concrete AST type. The parser
// would otherwise pay one heap alloc per node (~240k allocs on a real
// corpus); Arena replaces that with one alloc per chunk of N items per
// type. mallocgc cost amortises ~128x.
//
// Layout per type:
//
//	type slab[T any] struct {
//	    head *chunk[T]
//	    idx  int
//	}
//	type chunk[T any] struct {
//	    items [arenaChunkSize]T   // first field: items[0] aligns to chunk start
//	    next  *chunk[T]           // last field: never touched during iteration
//	}
//
// Invariant: chunks are never reallocated. NewX returns a stable *T into a
// fixed-size array inside a chunk. Once a chunk fills, a new chunk is
// linked at the head; existing pointers stay valid for the Arena's lifetime.
// Program retains the *Arena reference so the GC keeps chunks reachable
// as long as the AST is.
//
// Mixing: callers without an arena (tests, hand-built fixtures) can keep
// using `&ast.T{}`. NewX falls back to `new(T)` on a nil receiver -- the
// arena is an opt-in optimisation, not an API requirement.
//
// Cache-line alignment: items live at chunk offset 0, so items[0] inherits
// the chunk's allocator-provided alignment. Go's mallocgc aligns
// >8KB-sized allocations to page boundaries and smaller ones to size-class
// boundaries (>=8B; typically 16-32B). For the workloads here -- linear
// traversal of items[] during parsing -- the prefetcher handles within-
// chunk strides easily; what matters is that `next` lives at the tail
// (last field) and isn't touched during normal access, so it doesn't
// pollute the items cache footprint.
// arenaChunkSize tunes the bump-allocator chunk size. Larger amortises
// mallocgc cost over more nodes per chunk; smaller wastes less memory
// when a parse uses a type only a few times. Empirically 16 sits at the
// best throughput/memory point on the integration corpus: chunks=8 use
// slightly more allocations, chunks=32+ pay too much memory for the tail
// padding multiplied across ~60 node types and many small files.
const arenaChunkSize = 16

// slab is a per-type bump allocator with rewind support.
//
// chunks holds every chunk this slab has ever allocated, in order of
// allocation. cur is the index of the chunk currently being filled; idx
// is the next free slot within chunks[cur]. Arena.Reset() rewinds cur
// and idx back to 0 so a subsequent parse re-fills the same chunks from
// the start -- no new mallocs while the chunk count stays under peak.
//
// Generic over the element type so every per-type slab on Arena shares
// one implementation.
type slab[T any] struct {
	chunks []*chunk[T]
	cur    int // current chunk index in chunks (0..len(chunks)-1)
	idx    int // next free slot in chunks[cur]
}

type chunk[T any] struct {
	items [arenaChunkSize]T
}

// arenaAlloc returns a pointer to the next free slot in slab s, growing
// the chunk list only when no previously-allocated chunk has room. The
// returned slot is NOT zeroed -- the caller (NewX) explicitly clears
// each field, using `:0` truncation on slice headers so backing arrays
// survive Reset and the next parse re-fills them in place. This is the
// steady-state win for the arena-reuse path: slice-grow allocations
// move to first-parse warmup only.
func arenaAlloc[T any](s *slab[T]) *T {
	if s.idx == arenaChunkSize {
		s.cur++
		s.idx = 0
	}
	if s.cur >= len(s.chunks) {
		s.chunks = append(s.chunks, &chunk[T]{})
	}
	p := &s.chunks[s.cur].items[s.idx]
	s.idx++
	return p
}

// reset rewinds the slab cursor. Item slots are not touched -- the
// per-type NewX methods own clearing logic so they can preserve slice
// capacities across Reset.
//
// Contract: callers MUST NOT retain pointers into AST nodes built before
// the Reset that owns this slab. Slots get re-handed out on the next
// allocation; stale references silently observe next-parse data.
func (s *slab[T]) reset() {
	s.cur = 0
	s.idx = 0
}

// Arena holds one slab per AST node type. Slabs grow lazily -- a slab that
// is never used costs only its zero-value struct field overhead in Arena.
//
// LitPool is the lexer's per-token literal pool, reused across parses
// just like the AST slabs. The parser passes the arena's current pool
// to the lexer at init (zero-length view of the warmed-up array), the
// lexer appends through parse, the parser writes back the grown slice
// at parse end. Reset() truncates to zero length while keeping capacity,
// so the next parse re-fills the same backing array.
//
// Same contract as the AST slabs: callers MUST NOT call Reset while a
// previously-returned Program is still being read -- the program's
// LitPool aliases the arena's slice, so resetting / re-parsing will
// overwrite its contents.
type Arena struct {
	LitPool []string

	returnStatementSlab          slab[ReturnStatement]
	expressionStatementSlab      slab[ExpressionStatement]
	blockStatementSlab           slab[BlockStatement]
	exceptionHandlingBlockSlab   slab[ExceptionHandlingBlock]
	rescueBlockSlab              slab[RescueBlock]
	assignmentSlab               slab[Assignment]
	instanceVariableSlab         slab[InstanceVariable]
	classVariableSlab            slab[ClassVariable]
	multiAssignmentSlab          slab[MultiAssignment]
	selfSlab                     slab[Self]
	yieldExpressionSlab          slab[YieldExpression]
	superExpressionSlab          slab[SuperExpression]
	beginBlockSlab               slab[BeginBlock]
	endBlockSlab                 slab[EndBlock]
	keywordFILESlab              slab[Keyword__FILE__]
	keywordLINESlab              slab[Keyword__LINE__]
	keywordDIRSlab               slab[Keyword__DIR__]
	keywordCALLEESlab            slab[Keyword__CALLEE__]
	keywordMETHODSlab            slab[Keyword__METHOD__]
	keywordENCODINGSlab          slab[Keyword__ENCODING__]
	usingExpressionSlab          slab[UsingExpression]
	refineExpressionSlab         slab[RefineExpression]
	identifierSlab               slab[Identifier]
	globalSlab                   slab[Global]
	scopedIdentifierSlab         slab[ScopedIdentifier]
	integerLiteralSlab           slab[IntegerLiteral]
	floatLiteralSlab             slab[FloatLiteral]
	nilSlab                      slab[Nil]
	booleanSlab                  slab[Boolean]
	stringLiteralSlab            slab[StringLiteral]
	stringContentSlab            slab[StringContent]
	embeddedVariableSlab         slab[EmbeddedVariable]
	regexLiteralSlab             slab[RegexLiteral]
	commentSlab                  slab[Comment]
	symbolLiteralSlab            slab[SymbolLiteral]
	conditionalExpressionSlab    slab[ConditionalExpression]
	loopExpressionSlab           slab[LoopExpression]
	implicitRestSlab             slab[ImplicitRest]
	arrayLiteralSlab             slab[ArrayLiteral]
	hashLiteralSlab              slab[HashLiteral]
	blockCaptureSlab             slab[BlockCapture]
	functionLiteralSlab          slab[FunctionLiteral]
	functionParameterSlab        slab[FunctionParameter]
	indexExpressionSlab          slab[IndexExpression]
	contextCallExpressionSlab    slab[ContextCallExpression]
	blockExpressionSlab          slab[BlockExpression]
	moduleExpressionSlab         slab[ModuleExpression]
	classExpressionSlab          slab[ClassExpression]
	singletonClassExpressionSlab slab[SingletonClassExpression]
	splatExpressionSlab          slab[SplatExpression]
	argumentForwardingSlab       slab[ArgumentForwarding]
	caseExpressionSlab           slab[CaseExpression]
	whenClauseSlab               slab[WhenClause]
	definedExpressionSlab        slab[DefinedExpression]
	jumpExpressionSlab           slab[JumpExpression]
	aliasExpressionSlab          slab[AliasExpression]
	undefExpressionSlab          slab[UndefExpression]
	prefixExpressionSlab         slab[PrefixExpression]
	infixExpressionSlab          slab[InfixExpression]
	rightwardAssignmentSlab      slab[RightwardAssignment]
	parenExpressionSlab          slab[ParenExpression]
	flipFlopSlab                 slab[FlipFlop]
}

// NewArena returns a fresh, empty Arena.
func NewArena() *Arena { return &Arena{} }

// Reset rewinds every slab so subsequent allocations re-fill the chunks
// allocated by prior parses. The chunks themselves stay in memory; only
// the cur/idx pointers move back to 0. Callers MUST NOT retain pointers
// into AST nodes built before the Reset -- those slots get overwritten
// by the next parse.
//
// Use case: long-lived parser sessions parsing many files. The first
// parse warms each slab up to its peak chunk count; subsequent parses
// pay zero mallocgc for AST nodes until they exceed the warmed footprint.
func (a *Arena) Reset() {
	a.returnStatementSlab.reset()
	a.expressionStatementSlab.reset()
	a.blockStatementSlab.reset()
	a.exceptionHandlingBlockSlab.reset()
	a.rescueBlockSlab.reset()
	a.assignmentSlab.reset()
	a.instanceVariableSlab.reset()
	a.classVariableSlab.reset()
	a.multiAssignmentSlab.reset()
	a.selfSlab.reset()
	a.yieldExpressionSlab.reset()
	a.superExpressionSlab.reset()
	a.beginBlockSlab.reset()
	a.endBlockSlab.reset()
	a.keywordFILESlab.reset()
	a.keywordLINESlab.reset()
	a.keywordDIRSlab.reset()
	a.keywordCALLEESlab.reset()
	a.keywordMETHODSlab.reset()
	a.keywordENCODINGSlab.reset()
	a.usingExpressionSlab.reset()
	a.refineExpressionSlab.reset()
	a.identifierSlab.reset()
	a.globalSlab.reset()
	a.scopedIdentifierSlab.reset()
	a.integerLiteralSlab.reset()
	a.floatLiteralSlab.reset()
	a.nilSlab.reset()
	a.booleanSlab.reset()
	a.stringLiteralSlab.reset()
	a.stringContentSlab.reset()
	a.embeddedVariableSlab.reset()
	a.regexLiteralSlab.reset()
	a.commentSlab.reset()
	a.symbolLiteralSlab.reset()
	a.conditionalExpressionSlab.reset()
	a.loopExpressionSlab.reset()
	a.implicitRestSlab.reset()
	a.arrayLiteralSlab.reset()
	a.hashLiteralSlab.reset()
	a.blockCaptureSlab.reset()
	a.functionLiteralSlab.reset()
	a.functionParameterSlab.reset()
	a.indexExpressionSlab.reset()
	a.contextCallExpressionSlab.reset()
	a.blockExpressionSlab.reset()
	a.moduleExpressionSlab.reset()
	a.classExpressionSlab.reset()
	a.singletonClassExpressionSlab.reset()
	a.splatExpressionSlab.reset()
	a.argumentForwardingSlab.reset()
	a.caseExpressionSlab.reset()
	a.whenClauseSlab.reset()
	a.definedExpressionSlab.reset()
	a.jumpExpressionSlab.reset()
	a.aliasExpressionSlab.reset()
	a.undefExpressionSlab.reset()
	a.prefixExpressionSlab.reset()
	a.infixExpressionSlab.reset()
	a.rightwardAssignmentSlab.reset()
	a.parenExpressionSlab.reset()
	a.flipFlopSlab.reset()
	a.LitPool = a.LitPool[:0]
}

// Init copies src into the slot dst points at and returns dst. Lets
// callers keep struct-literal field syntax at the construction site while
// routing the allocation through Arena.NewX():
//
//	ast.Init(p.arena.NewIdentifier(), ast.Identifier{Token: t, Value: v})
//
// dst must not be nil; the typical caller passes the return of a NewX()
// method which is guaranteed non-nil.
func Init[T any](dst *T, src T) *T {
	*dst = src
	return dst
}

// The per-type NewX constructors live in arena_gen.go, regenerated from
// this file's Arena.slab[T] field list + ast.go's struct definitions.
//
//go:generate go run ../cmd/gen-arena -in . -out arena_gen.go
