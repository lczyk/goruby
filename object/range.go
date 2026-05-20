package object

// Range represents a ruby Range. Begin and End are arbitrary RubyObject
// to allow non-numeric ranges later; the evaluator only uses integer
// ranges today.
type Range struct {
	Begin     RubyObject
	End       RubyObject
	Exclusive bool // true for `a...b`, false for `a..b`
}

func NewRange(begin, end RubyObject, exclusive bool) *Range {
	return &Range{Begin: begin, End: end, Exclusive: exclusive}
}

func (r *Range) Type() Type       { return OBJECT_OBJ }
func (r *Range) Class() RubyClass { return RangeClass }

func (r *Range) Inspect() string {
	op := ".."
	if r.Exclusive {
		op = "..."
	}
	var b, e string
	if r.Begin != nil {
		b = r.Begin.Inspect()
	}
	if r.End != nil {
		e = r.End.Inspect()
	}
	return b + op + e
}
