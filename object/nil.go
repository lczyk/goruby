package object

// Nil is the type of the nil singleton.
// Pointer-free; noscan-eligible.
type Nil struct{}

// NIL is the canonical nil value. Use pointer comparison against this
// singleton rather than allocating fresh Nil values.
var NIL = &Nil{}

func (n *Nil) Inspect() string  { return "nil" }
func (n *Nil) Type() Type       { return NIL_OBJ }
func (n *Nil) Class() RubyClass { return NilClassClass }
