package object

// Boolean represents a ruby true / false value.
// Pointer-free; noscan-eligible.
type Boolean struct {
	Value bool
}

// TRUE and FALSE are the canonical boolean values. Pointer comparison
// against these singletons is the cheap truthiness test.
var (
	TRUE  = &Boolean{Value: true}
	FALSE = &Boolean{Value: false}
)

// BooleanOf returns the singleton matching v. Use in preference to
// allocating fresh Boolean values.
func BooleanOf(v bool) *Boolean {
	if v {
		return TRUE
	}
	return FALSE
}

func (b *Boolean) Inspect() string {
	if b.Value {
		return "true"
	}
	return "false"
}

func (b *Boolean) Type() Type { return BOOL_OBJ }
func (b *Boolean) Class() RubyClass {
	if b.Value {
		return TrueClassClass
	}
	return FalseClassClass
}
