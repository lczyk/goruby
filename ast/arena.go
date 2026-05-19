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

// slab tracks the most recently allocated chunk and the index of the next
// free slot. Generic over the element type so all per-type slabs share one
// implementation.
type slab[T any] struct {
	head *chunk[T]
	idx  int
}

type chunk[T any] struct {
	items [arenaChunkSize]T
	next  *chunk[T]
}

// arenaAlloc returns a pointer to the next free slot in slab s, growing
// the linked chunk list when the current head fills.
func arenaAlloc[T any](s *slab[T]) *T {
	if s.head == nil || s.idx == arenaChunkSize {
		s.head = &chunk[T]{next: s.head}
		s.idx = 0
	}
	p := &s.head.items[s.idx]
	s.idx++
	return p
}

// Arena holds one slab per AST node type. Slabs grow lazily -- a slab that
// is never used costs only its zero-value struct field overhead in Arena.
type Arena struct {
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

// --- per-type constructors ---------------------------------------------------

func (a *Arena) NewReturnStatement() *ReturnStatement {
	if a == nil {
		return new(ReturnStatement)
	}
	return arenaAlloc(&a.returnStatementSlab)
}
func (a *Arena) NewExpressionStatement() *ExpressionStatement {
	if a == nil {
		return new(ExpressionStatement)
	}
	return arenaAlloc(&a.expressionStatementSlab)
}
func (a *Arena) NewBlockStatement() *BlockStatement {
	if a == nil {
		return new(BlockStatement)
	}
	return arenaAlloc(&a.blockStatementSlab)
}
func (a *Arena) NewExceptionHandlingBlock() *ExceptionHandlingBlock {
	if a == nil {
		return new(ExceptionHandlingBlock)
	}
	return arenaAlloc(&a.exceptionHandlingBlockSlab)
}
func (a *Arena) NewRescueBlock() *RescueBlock {
	if a == nil {
		return new(RescueBlock)
	}
	return arenaAlloc(&a.rescueBlockSlab)
}
func (a *Arena) NewAssignment() *Assignment {
	if a == nil {
		return new(Assignment)
	}
	return arenaAlloc(&a.assignmentSlab)
}
func (a *Arena) NewInstanceVariable() *InstanceVariable {
	if a == nil {
		return new(InstanceVariable)
	}
	return arenaAlloc(&a.instanceVariableSlab)
}
func (a *Arena) NewClassVariable() *ClassVariable {
	if a == nil {
		return new(ClassVariable)
	}
	return arenaAlloc(&a.classVariableSlab)
}
func (a *Arena) NewMultiAssignment() *MultiAssignment {
	if a == nil {
		return new(MultiAssignment)
	}
	return arenaAlloc(&a.multiAssignmentSlab)
}
func (a *Arena) NewSelf() *Self {
	if a == nil {
		return new(Self)
	}
	return arenaAlloc(&a.selfSlab)
}
func (a *Arena) NewYieldExpression() *YieldExpression {
	if a == nil {
		return new(YieldExpression)
	}
	return arenaAlloc(&a.yieldExpressionSlab)
}
func (a *Arena) NewSuperExpression() *SuperExpression {
	if a == nil {
		return new(SuperExpression)
	}
	return arenaAlloc(&a.superExpressionSlab)
}
func (a *Arena) NewBeginBlock() *BeginBlock {
	if a == nil {
		return new(BeginBlock)
	}
	return arenaAlloc(&a.beginBlockSlab)
}
func (a *Arena) NewEndBlock() *EndBlock {
	if a == nil {
		return new(EndBlock)
	}
	return arenaAlloc(&a.endBlockSlab)
}
func (a *Arena) NewKeyword__FILE__() *Keyword__FILE__ {
	if a == nil {
		return new(Keyword__FILE__)
	}
	return arenaAlloc(&a.keywordFILESlab)
}
func (a *Arena) NewKeyword__DIR__() *Keyword__DIR__ {
	if a == nil {
		return new(Keyword__DIR__)
	}
	return arenaAlloc(&a.keywordDIRSlab)
}
func (a *Arena) NewKeyword__CALLEE__() *Keyword__CALLEE__ {
	if a == nil {
		return new(Keyword__CALLEE__)
	}
	return arenaAlloc(&a.keywordCALLEESlab)
}
func (a *Arena) NewKeyword__METHOD__() *Keyword__METHOD__ {
	if a == nil {
		return new(Keyword__METHOD__)
	}
	return arenaAlloc(&a.keywordMETHODSlab)
}
func (a *Arena) NewKeyword__ENCODING__() *Keyword__ENCODING__ {
	if a == nil {
		return new(Keyword__ENCODING__)
	}
	return arenaAlloc(&a.keywordENCODINGSlab)
}
func (a *Arena) NewUsingExpression() *UsingExpression {
	if a == nil {
		return new(UsingExpression)
	}
	return arenaAlloc(&a.usingExpressionSlab)
}
func (a *Arena) NewRefineExpression() *RefineExpression {
	if a == nil {
		return new(RefineExpression)
	}
	return arenaAlloc(&a.refineExpressionSlab)
}
func (a *Arena) NewIdentifier() *Identifier {
	if a == nil {
		return new(Identifier)
	}
	return arenaAlloc(&a.identifierSlab)
}
func (a *Arena) NewGlobal() *Global {
	if a == nil {
		return new(Global)
	}
	return arenaAlloc(&a.globalSlab)
}
func (a *Arena) NewScopedIdentifier() *ScopedIdentifier {
	if a == nil {
		return new(ScopedIdentifier)
	}
	return arenaAlloc(&a.scopedIdentifierSlab)
}
func (a *Arena) NewIntegerLiteral() *IntegerLiteral {
	if a == nil {
		return new(IntegerLiteral)
	}
	return arenaAlloc(&a.integerLiteralSlab)
}
func (a *Arena) NewFloatLiteral() *FloatLiteral {
	if a == nil {
		return new(FloatLiteral)
	}
	return arenaAlloc(&a.floatLiteralSlab)
}
func (a *Arena) NewNil() *Nil {
	if a == nil {
		return new(Nil)
	}
	return arenaAlloc(&a.nilSlab)
}
func (a *Arena) NewBoolean() *Boolean {
	if a == nil {
		return new(Boolean)
	}
	return arenaAlloc(&a.booleanSlab)
}
func (a *Arena) NewStringLiteral() *StringLiteral {
	if a == nil {
		return new(StringLiteral)
	}
	return arenaAlloc(&a.stringLiteralSlab)
}
func (a *Arena) NewStringContent() *StringContent {
	if a == nil {
		return new(StringContent)
	}
	return arenaAlloc(&a.stringContentSlab)
}
func (a *Arena) NewEmbeddedVariable() *EmbeddedVariable {
	if a == nil {
		return new(EmbeddedVariable)
	}
	return arenaAlloc(&a.embeddedVariableSlab)
}
func (a *Arena) NewRegexLiteral() *RegexLiteral {
	if a == nil {
		return new(RegexLiteral)
	}
	return arenaAlloc(&a.regexLiteralSlab)
}
func (a *Arena) NewComment() *Comment {
	if a == nil {
		return new(Comment)
	}
	return arenaAlloc(&a.commentSlab)
}
func (a *Arena) NewSymbolLiteral() *SymbolLiteral {
	if a == nil {
		return new(SymbolLiteral)
	}
	return arenaAlloc(&a.symbolLiteralSlab)
}
func (a *Arena) NewConditionalExpression() *ConditionalExpression {
	if a == nil {
		return new(ConditionalExpression)
	}
	return arenaAlloc(&a.conditionalExpressionSlab)
}
func (a *Arena) NewLoopExpression() *LoopExpression {
	if a == nil {
		return new(LoopExpression)
	}
	return arenaAlloc(&a.loopExpressionSlab)
}
func (a *Arena) NewImplicitRest() *ImplicitRest {
	if a == nil {
		return new(ImplicitRest)
	}
	return arenaAlloc(&a.implicitRestSlab)
}
func (a *Arena) NewArrayLiteral() *ArrayLiteral {
	if a == nil {
		return new(ArrayLiteral)
	}
	return arenaAlloc(&a.arrayLiteralSlab)
}
func (a *Arena) NewHashLiteral() *HashLiteral {
	if a == nil {
		return new(HashLiteral)
	}
	return arenaAlloc(&a.hashLiteralSlab)
}
func (a *Arena) NewBlockCapture() *BlockCapture {
	if a == nil {
		return new(BlockCapture)
	}
	return arenaAlloc(&a.blockCaptureSlab)
}
func (a *Arena) NewFunctionLiteral() *FunctionLiteral {
	if a == nil {
		return new(FunctionLiteral)
	}
	return arenaAlloc(&a.functionLiteralSlab)
}
func (a *Arena) NewFunctionParameter() *FunctionParameter {
	if a == nil {
		return new(FunctionParameter)
	}
	return arenaAlloc(&a.functionParameterSlab)
}
func (a *Arena) NewIndexExpression() *IndexExpression {
	if a == nil {
		return new(IndexExpression)
	}
	return arenaAlloc(&a.indexExpressionSlab)
}
func (a *Arena) NewContextCallExpression() *ContextCallExpression {
	if a == nil {
		return new(ContextCallExpression)
	}
	return arenaAlloc(&a.contextCallExpressionSlab)
}
func (a *Arena) NewBlockExpression() *BlockExpression {
	if a == nil {
		return new(BlockExpression)
	}
	return arenaAlloc(&a.blockExpressionSlab)
}
func (a *Arena) NewModuleExpression() *ModuleExpression {
	if a == nil {
		return new(ModuleExpression)
	}
	return arenaAlloc(&a.moduleExpressionSlab)
}
func (a *Arena) NewClassExpression() *ClassExpression {
	if a == nil {
		return new(ClassExpression)
	}
	return arenaAlloc(&a.classExpressionSlab)
}
func (a *Arena) NewSingletonClassExpression() *SingletonClassExpression {
	if a == nil {
		return new(SingletonClassExpression)
	}
	return arenaAlloc(&a.singletonClassExpressionSlab)
}
func (a *Arena) NewSplatExpression() *SplatExpression {
	if a == nil {
		return new(SplatExpression)
	}
	return arenaAlloc(&a.splatExpressionSlab)
}
func (a *Arena) NewArgumentForwarding() *ArgumentForwarding {
	if a == nil {
		return new(ArgumentForwarding)
	}
	return arenaAlloc(&a.argumentForwardingSlab)
}
func (a *Arena) NewCaseExpression() *CaseExpression {
	if a == nil {
		return new(CaseExpression)
	}
	return arenaAlloc(&a.caseExpressionSlab)
}
func (a *Arena) NewWhenClause() *WhenClause {
	if a == nil {
		return new(WhenClause)
	}
	return arenaAlloc(&a.whenClauseSlab)
}
func (a *Arena) NewDefinedExpression() *DefinedExpression {
	if a == nil {
		return new(DefinedExpression)
	}
	return arenaAlloc(&a.definedExpressionSlab)
}
func (a *Arena) NewJumpExpression() *JumpExpression {
	if a == nil {
		return new(JumpExpression)
	}
	return arenaAlloc(&a.jumpExpressionSlab)
}
func (a *Arena) NewAliasExpression() *AliasExpression {
	if a == nil {
		return new(AliasExpression)
	}
	return arenaAlloc(&a.aliasExpressionSlab)
}
func (a *Arena) NewUndefExpression() *UndefExpression {
	if a == nil {
		return new(UndefExpression)
	}
	return arenaAlloc(&a.undefExpressionSlab)
}
func (a *Arena) NewPrefixExpression() *PrefixExpression {
	if a == nil {
		return new(PrefixExpression)
	}
	return arenaAlloc(&a.prefixExpressionSlab)
}
func (a *Arena) NewInfixExpression() *InfixExpression {
	if a == nil {
		return new(InfixExpression)
	}
	return arenaAlloc(&a.infixExpressionSlab)
}
func (a *Arena) NewRightwardAssignment() *RightwardAssignment {
	if a == nil {
		return new(RightwardAssignment)
	}
	return arenaAlloc(&a.rightwardAssignmentSlab)
}
func (a *Arena) NewParenExpression() *ParenExpression {
	if a == nil {
		return new(ParenExpression)
	}
	return arenaAlloc(&a.parenExpressionSlab)
}
