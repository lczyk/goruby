package object

// Enumerator is a minimal stand-in for ruby's Enumerator class.
// Holds a receiver + the name of the method that produced the
// Enumerator (e.g. "sort_by", "map", "each"). Operators like
// with_index inspect the method name to decide how to combine
// the block-supplied transformation with the receiver iteration.
//
// Not a faithful re-implementation of MRI's Enumerator (no lazy
// chaining, no Generator, no rewind). Covers the .with_index +
// .map + .to_a + .each forms the corpus hits.
type Enumerator struct {
	Receiver RubyObject
	Method   string
	// Args holds the arguments the producing method was originally
	// called with (e.g. each_slice's slice size). Not always used.
	Args []RubyObject
}

func (e *Enumerator) Type() Type       { return OBJECT_OBJ }
func (e *Enumerator) Class() RubyClass { return EnumeratorClass }
func (e *Enumerator) Inspect() string  { return "#<Enumerator>" }

// EnumeratorClass is the dispatch target for Enumerator instances.
// Populated in the evaluator package's bootstrap.
var EnumeratorClass = NewClass("Enumerator", nil)
