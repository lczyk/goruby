package ast

import "github.com/lczyk/goruby/token"

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

func (a *Arena) NewReturnStatement() *ReturnStatement {
	if a == nil {
		return new(ReturnStatement)
	}
	p := arenaAlloc(&a.returnStatementSlab)
	p.Token = token.Token{}
	p.ReturnValue = nil
	return p
}

func (a *Arena) NewExpressionStatement() *ExpressionStatement {
	if a == nil {
		return new(ExpressionStatement)
	}
	p := arenaAlloc(&a.expressionStatementSlab)
	p.Token = token.Token{}
	p.Expression = nil
	return p
}

func (a *Arena) NewBlockStatement() *BlockStatement {
	if a == nil {
		return new(BlockStatement)
	}
	p := arenaAlloc(&a.blockStatementSlab)
	p.Token = token.Token{}
	p.EndPos = 0
	p.Statements = p.Statements[:0]
	return p
}

func (a *Arena) NewExceptionHandlingBlock() *ExceptionHandlingBlock {
	if a == nil {
		return new(ExceptionHandlingBlock)
	}
	p := arenaAlloc(&a.exceptionHandlingBlockSlab)
	p.BeginToken = token.Token{}
	p.EndPos = 0
	p.TryBody = nil
	p.Rescues = p.Rescues[:0]
	p.ElseBody = nil
	p.EnsureBody = nil
	return p
}

func (a *Arena) NewRescueBlock() *RescueBlock {
	if a == nil {
		return new(RescueBlock)
	}
	p := arenaAlloc(&a.rescueBlockSlab)
	p.Token = token.Token{}
	p.ExceptionClasses = p.ExceptionClasses[:0]
	p.Exception = nil
	p.Body = nil
	return p
}

func (a *Arena) NewAssignment() *Assignment {
	if a == nil {
		return new(Assignment)
	}
	p := arenaAlloc(&a.assignmentSlab)
	p.Token = token.Token{}
	p.Left = nil
	p.Right = nil
	return p
}

func (a *Arena) NewInstanceVariable() *InstanceVariable {
	if a == nil {
		return new(InstanceVariable)
	}
	p := arenaAlloc(&a.instanceVariableSlab)
	p.Token = token.Token{}
	p.Name = nil
	return p
}

func (a *Arena) NewClassVariable() *ClassVariable {
	if a == nil {
		return new(ClassVariable)
	}
	p := arenaAlloc(&a.classVariableSlab)
	p.Token = token.Token{}
	p.Name = nil
	return p
}

func (a *Arena) NewMultiAssignment() *MultiAssignment {
	if a == nil {
		return new(MultiAssignment)
	}
	p := arenaAlloc(&a.multiAssignmentSlab)
	p.Variables = p.Variables[:0]
	p.Values = p.Values[:0]
	return p
}

func (a *Arena) NewSelf() *Self {
	if a == nil {
		return new(Self)
	}
	p := arenaAlloc(&a.selfSlab)
	p.PosOff = 0
	return p
}

func (a *Arena) NewYieldExpression() *YieldExpression {
	if a == nil {
		return new(YieldExpression)
	}
	p := arenaAlloc(&a.yieldExpressionSlab)
	p.Token = token.Token{}
	p.Arguments = p.Arguments[:0]
	p.Block = nil
	return p
}

func (a *Arena) NewSuperExpression() *SuperExpression {
	if a == nil {
		return new(SuperExpression)
	}
	p := arenaAlloc(&a.superExpressionSlab)
	p.Token = token.Token{}
	p.Arguments = p.Arguments[:0]
	p.Block = nil
	return p
}

func (a *Arena) NewBeginBlock() *BeginBlock {
	if a == nil {
		return new(BeginBlock)
	}
	p := arenaAlloc(&a.beginBlockSlab)
	p.Token = token.Token{}
	p.Body = nil
	return p
}

func (a *Arena) NewEndBlock() *EndBlock {
	if a == nil {
		return new(EndBlock)
	}
	p := arenaAlloc(&a.endBlockSlab)
	p.Token = token.Token{}
	p.Body = nil
	return p
}

func (a *Arena) NewKeyword__FILE__() *Keyword__FILE__ {
	if a == nil {
		return new(Keyword__FILE__)
	}
	p := arenaAlloc(&a.keywordFILESlab)
	p.Token = token.Token{}
	p.Filename = ""
	return p
}

func (a *Arena) NewKeyword__LINE__() *Keyword__LINE__ {
	if a == nil {
		return new(Keyword__LINE__)
	}
	p := arenaAlloc(&a.keywordLINESlab)
	p.PosOff = 0
	return p
}

func (a *Arena) NewKeyword__DIR__() *Keyword__DIR__ {
	if a == nil {
		return new(Keyword__DIR__)
	}
	p := arenaAlloc(&a.keywordDIRSlab)
	p.PosOff = 0
	return p
}

func (a *Arena) NewKeyword__CALLEE__() *Keyword__CALLEE__ {
	if a == nil {
		return new(Keyword__CALLEE__)
	}
	p := arenaAlloc(&a.keywordCALLEESlab)
	p.PosOff = 0
	return p
}

func (a *Arena) NewKeyword__METHOD__() *Keyword__METHOD__ {
	if a == nil {
		return new(Keyword__METHOD__)
	}
	p := arenaAlloc(&a.keywordMETHODSlab)
	p.PosOff = 0
	return p
}

func (a *Arena) NewKeyword__ENCODING__() *Keyword__ENCODING__ {
	if a == nil {
		return new(Keyword__ENCODING__)
	}
	p := arenaAlloc(&a.keywordENCODINGSlab)
	p.PosOff = 0
	return p
}

func (a *Arena) NewUsingExpression() *UsingExpression {
	if a == nil {
		return new(UsingExpression)
	}
	p := arenaAlloc(&a.usingExpressionSlab)
	p.Token = token.Token{}
	p.Expr = nil
	return p
}

func (a *Arena) NewRefineExpression() *RefineExpression {
	if a == nil {
		return new(RefineExpression)
	}
	p := arenaAlloc(&a.refineExpressionSlab)
	p.Token = token.Token{}
	p.EndPos = 0
	p.Expr = nil
	p.Body = nil
	return p
}

func (a *Arena) NewIdentifier() *Identifier {
	if a == nil {
		return new(Identifier)
	}
	p := arenaAlloc(&a.identifierSlab)
	p.Token = token.Token{}
	p.Value = ""
	return p
}

func (a *Arena) NewGlobal() *Global {
	if a == nil {
		return new(Global)
	}
	p := arenaAlloc(&a.globalSlab)
	p.Token = token.Token{}
	p.Value = ""
	return p
}

func (a *Arena) NewScopedIdentifier() *ScopedIdentifier {
	if a == nil {
		return new(ScopedIdentifier)
	}
	p := arenaAlloc(&a.scopedIdentifierSlab)
	p.Token = token.Token{}
	p.Outer = nil
	p.Inner = nil
	return p
}

func (a *Arena) NewIntegerLiteral() *IntegerLiteral {
	if a == nil {
		return new(IntegerLiteral)
	}
	p := arenaAlloc(&a.integerLiteralSlab)
	p.PosOff = 0
	p.Value = 0
	p.BigInt = nil
	p.Base = 0
	p.HadWhitespace = false
	p.Rational = false
	p.Imaginary = false
	return p
}

func (a *Arena) NewFloatLiteral() *FloatLiteral {
	if a == nil {
		return new(FloatLiteral)
	}
	p := arenaAlloc(&a.floatLiteralSlab)
	p.PosOff = 0
	p.Value = 0
	p.HadWhitespace = false
	p.Rational = false
	p.Imaginary = false
	return p
}

func (a *Arena) NewNil() *Nil {
	if a == nil {
		return new(Nil)
	}
	p := arenaAlloc(&a.nilSlab)
	p.PosOff = 0
	return p
}

func (a *Arena) NewBoolean() *Boolean {
	if a == nil {
		return new(Boolean)
	}
	p := arenaAlloc(&a.booleanSlab)
	p.PosOff = 0
	p.Value = false
	return p
}

func (a *Arena) NewStringLiteral() *StringLiteral {
	if a == nil {
		return new(StringLiteral)
	}
	p := arenaAlloc(&a.stringLiteralSlab)
	p.Token = token.Token{}
	p.Value = ""
	p.Parts = p.Parts[:0]
	p.HeredocTagSource = ""
	p.HeredocStripped = false
	p.Adjacent = p.Adjacent[:0]
	return p
}

func (a *Arena) NewStringContent() *StringContent {
	if a == nil {
		return new(StringContent)
	}
	p := arenaAlloc(&a.stringContentSlab)
	p.Token = token.Token{}
	p.Value = ""
	return p
}

func (a *Arena) NewEmbeddedVariable() *EmbeddedVariable {
	if a == nil {
		return new(EmbeddedVariable)
	}
	p := arenaAlloc(&a.embeddedVariableSlab)
	p.Variable = nil
	return p
}

func (a *Arena) NewRegexLiteral() *RegexLiteral {
	if a == nil {
		return new(RegexLiteral)
	}
	p := arenaAlloc(&a.regexLiteralSlab)
	p.Token = token.Token{}
	p.Value = ""
	p.Parts = p.Parts[:0]
	p.Options = ""
	return p
}

func (a *Arena) NewComment() *Comment {
	if a == nil {
		return new(Comment)
	}
	p := arenaAlloc(&a.commentSlab)
	p.Token = token.Token{}
	p.Value = ""
	return p
}

func (a *Arena) NewSymbolLiteral() *SymbolLiteral {
	if a == nil {
		return new(SymbolLiteral)
	}
	p := arenaAlloc(&a.symbolLiteralSlab)
	p.Token = token.Token{}
	p.Value = nil
	p.LabelText = ""
	return p
}

func (a *Arena) NewConditionalExpression() *ConditionalExpression {
	if a == nil {
		return new(ConditionalExpression)
	}
	p := arenaAlloc(&a.conditionalExpressionSlab)
	p.Token = token.Token{}
	p.EndPos = 0
	p.Condition = nil
	p.Consequence = nil
	p.Alternative = nil
	return p
}

func (a *Arena) NewLoopExpression() *LoopExpression {
	if a == nil {
		return new(LoopExpression)
	}
	p := arenaAlloc(&a.loopExpressionSlab)
	p.Token = token.Token{}
	p.EndPos = 0
	p.Condition = nil
	p.Block = nil
	p.PostTest = false
	return p
}

func (a *Arena) NewImplicitRest() *ImplicitRest {
	if a == nil {
		return new(ImplicitRest)
	}
	p := arenaAlloc(&a.implicitRestSlab)
	p.PosOff = 0
	return p
}

func (a *Arena) NewArrayLiteral() *ArrayLiteral {
	if a == nil {
		return new(ArrayLiteral)
	}
	p := arenaAlloc(&a.arrayLiteralSlab)
	p.Token = token.Token{}
	p.EndPos = 0
	p.Elements = p.Elements[:0]
	p.Multiline = false
	p.PercentChar = 0
	return p
}

func (a *Arena) NewHashLiteral() *HashLiteral {
	if a == nil {
		return new(HashLiteral)
	}
	p := arenaAlloc(&a.hashLiteralSlab)
	p.Token = token.Token{}
	p.EndPos = 0
	p.Map.Reset()
	p.Splats = p.Splats[:0]
	p.Implicit = false
	return p
}

func (a *Arena) NewBlockCapture() *BlockCapture {
	if a == nil {
		return new(BlockCapture)
	}
	p := arenaAlloc(&a.blockCaptureSlab)
	p.Token = token.Token{}
	p.Name = nil
	p.Expr = nil
	return p
}

func (a *Arena) NewFunctionLiteral() *FunctionLiteral {
	if a == nil {
		return new(FunctionLiteral)
	}
	p := arenaAlloc(&a.functionLiteralSlab)
	p.Token = token.Token{}
	p.EndPos = 0
	p.Receiver = nil
	p.Name = nil
	p.Parameters = p.Parameters[:0]
	p.CapturedBlock = nil
	p.Body = nil
	p.Rescues = p.Rescues[:0]
	p.ElseBody = nil
	p.EnsureBody = nil
	p.IsLambda = false
	p.IsEndless = false
	p.ExplicitParens = false
	return p
}

func (a *Arena) NewFunctionParameter() *FunctionParameter {
	if a == nil {
		return new(FunctionParameter)
	}
	p := arenaAlloc(&a.functionParameterSlab)
	p.Name = nil
	p.Default = nil
	p.IsSplat = false
	p.IsKeyword = false
	p.IsKeywordRest = false
	p.IsNoKeywords = false
	p.IsForwarding = false
	p.IsImplicitRest = false
	return p
}

func (a *Arena) NewIndexExpression() *IndexExpression {
	if a == nil {
		return new(IndexExpression)
	}
	p := arenaAlloc(&a.indexExpressionSlab)
	p.Token = token.Token{}
	p.Left = nil
	p.Arguments = p.Arguments[:0]
	return p
}

func (a *Arena) NewContextCallExpression() *ContextCallExpression {
	if a == nil {
		return new(ContextCallExpression)
	}
	p := arenaAlloc(&a.contextCallExpressionSlab)
	p.OpType = 0
	p.ExplicitParens = false
	p.Context = nil
	p.Function = nil
	p.Block = nil
	p.Arguments = p.Arguments[:0]
	return p
}

func (a *Arena) NewBlockExpression() *BlockExpression {
	if a == nil {
		return new(BlockExpression)
	}
	p := arenaAlloc(&a.blockExpressionSlab)
	p.Token = token.Token{}
	p.EndPos = 0
	p.Parameters = p.Parameters[:0]
	p.BlockLocals = p.BlockLocals[:0]
	p.CapturedBlock = nil
	p.Body = nil
	p.Rescues = p.Rescues[:0]
	p.ElseBody = nil
	p.EnsureBody = nil
	p.HasParameterBars = false
	return p
}

func (a *Arena) NewModuleExpression() *ModuleExpression {
	if a == nil {
		return new(ModuleExpression)
	}
	p := arenaAlloc(&a.moduleExpressionSlab)
	p.Token = token.Token{}
	p.EndPos = 0
	p.Name = nil
	p.Body = nil
	p.Rescues = p.Rescues[:0]
	return p
}

func (a *Arena) NewClassExpression() *ClassExpression {
	if a == nil {
		return new(ClassExpression)
	}
	p := arenaAlloc(&a.classExpressionSlab)
	p.Token = token.Token{}
	p.EndPos = 0
	p.Name = nil
	p.SuperClass = nil
	p.Body = nil
	p.Rescues = p.Rescues[:0]
	return p
}

func (a *Arena) NewSingletonClassExpression() *SingletonClassExpression {
	if a == nil {
		return new(SingletonClassExpression)
	}
	p := arenaAlloc(&a.singletonClassExpressionSlab)
	p.Token = token.Token{}
	p.EndPos = 0
	p.Expr = nil
	p.Body = nil
	p.Rescues = p.Rescues[:0]
	return p
}

func (a *Arena) NewSplatExpression() *SplatExpression {
	if a == nil {
		return new(SplatExpression)
	}
	p := arenaAlloc(&a.splatExpressionSlab)
	p.Token = token.Token{}
	p.Operator = ""
	p.Right = nil
	return p
}

func (a *Arena) NewArgumentForwarding() *ArgumentForwarding {
	if a == nil {
		return new(ArgumentForwarding)
	}
	p := arenaAlloc(&a.argumentForwardingSlab)
	p.PosOff = 0
	return p
}

func (a *Arena) NewCaseExpression() *CaseExpression {
	if a == nil {
		return new(CaseExpression)
	}
	p := arenaAlloc(&a.caseExpressionSlab)
	p.Token = token.Token{}
	p.EndPos = 0
	p.Condition = nil
	p.WhenClauses = p.WhenClauses[:0]
	p.InClauses = p.InClauses[:0]
	p.ElseBody = nil
	return p
}

func (a *Arena) NewWhenClause() *WhenClause {
	if a == nil {
		return new(WhenClause)
	}
	p := arenaAlloc(&a.whenClauseSlab)
	p.Token = token.Token{}
	p.Conditions = p.Conditions[:0]
	p.Body = nil
	return p
}

func (a *Arena) NewDefinedExpression() *DefinedExpression {
	if a == nil {
		return new(DefinedExpression)
	}
	p := arenaAlloc(&a.definedExpressionSlab)
	p.Token = token.Token{}
	p.Expr = nil
	return p
}

func (a *Arena) NewJumpExpression() *JumpExpression {
	if a == nil {
		return new(JumpExpression)
	}
	p := arenaAlloc(&a.jumpExpressionSlab)
	p.Token = token.Token{}
	p.Value = nil
	return p
}

func (a *Arena) NewAliasExpression() *AliasExpression {
	if a == nil {
		return new(AliasExpression)
	}
	p := arenaAlloc(&a.aliasExpressionSlab)
	p.Token = token.Token{}
	p.NewName = nil
	p.OldName = nil
	return p
}

func (a *Arena) NewUndefExpression() *UndefExpression {
	if a == nil {
		return new(UndefExpression)
	}
	p := arenaAlloc(&a.undefExpressionSlab)
	p.Token = token.Token{}
	p.Names = p.Names[:0]
	return p
}

func (a *Arena) NewPrefixExpression() *PrefixExpression {
	if a == nil {
		return new(PrefixExpression)
	}
	p := arenaAlloc(&a.prefixExpressionSlab)
	p.Token = token.Token{}
	p.Operator = ""
	p.Right = nil
	return p
}

func (a *Arena) NewInfixExpression() *InfixExpression {
	if a == nil {
		return new(InfixExpression)
	}
	p := arenaAlloc(&a.infixExpressionSlab)
	p.Token = token.Token{}
	p.Left = nil
	p.Operator = ""
	p.Right = nil
	return p
}

func (a *Arena) NewRightwardAssignment() *RightwardAssignment {
	if a == nil {
		return new(RightwardAssignment)
	}
	p := arenaAlloc(&a.rightwardAssignmentSlab)
	p.Token = token.Token{}
	p.Left = nil
	p.Right = nil
	return p
}

func (a *Arena) NewParenExpression() *ParenExpression {
	if a == nil {
		return new(ParenExpression)
	}
	p := arenaAlloc(&a.parenExpressionSlab)
	p.Token = token.Token{}
	p.EndPos = 0
	p.Expr = nil
	p.Stmts = p.Stmts[:0]
	p.MultipleStmts = false
	return p
}

// Reset rewinds every slab so subsequent allocations refill the chunks
// allocated by prior parses. Slot contents are not touched here -- the
// per-type NewX methods clear fields on each construction (including
// :0 truncation on slice fields so backing arrays survive Reset).
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
	a.LitPool = a.LitPool[:0]
}
