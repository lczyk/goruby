package object

// LazyEnumerator is a pull-driven chain of operations over a source.
// Chainable through map / select / reject / take / take_while /
// reject / drop / drop_while / filter_map; forced through first(n) /
// to_a / each / force.
//
// The source is opaque: a closure that yields one element at a time.
// Each op records its kind plus a callback (and AST block for env
// scoping). The evaluator drives forcing: it walks Ops, threading
// each source element through, emitting transformed / filtered
// values to the consumer. Stateful ops (take(n), drop(n),
// take_while, drop_while) keep their state in the driver, not on
// the op itself, so re-forcing a Lazy from a previous chain is
// possible (matches MRI semantics where each force re-runs the
// pipeline).
//
// Holds Go closures as `any` to keep object package free of the
// evaluator's block / callback types.
type LazyEnumerator struct {
	// Next pulls the next value from the source. Returns
	// (value, more). `more=false` signals exhausted source.
	Next func() (RubyObject, bool)
	// Ops is the pipeline applied to each source value, in order.
	Ops []LazyOp
}

// LazyOp describes one stage in a Lazy pipeline.
type LazyOp struct {
	Kind string // "map" | "select" | "reject" | "take" | "take_while" | "drop" | "drop_while" | "filter_map" | "flat_map"
	// Fn is the block callback. Stored as `any` to avoid pulling
	// the evaluator's block.go types into the object package.
	Fn any
	// Block is the source-level AST block. Carried for env scoping
	// in chained ops (Symbol-to-Proc, etc.); may be nil.
	Block any
	// N is the integer arg for take(n) / drop(n). Unused otherwise.
	N int64
}

func (l *LazyEnumerator) Type() Type       { return OBJECT_OBJ }
func (l *LazyEnumerator) Class() RubyClass { return LazyClass }
func (l *LazyEnumerator) Inspect() string  { return "#<Enumerator::Lazy>" }

// LazyClass is the dispatch class for LazyEnumerator instances.
// Populated in the evaluator package's bootstrap.
var LazyClass *Class

func init() {
	LazyClass = NewClass("Enumerator::Lazy", ObjectClass)
}
