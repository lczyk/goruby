package object

// Builtin class pointers, the single canonical Class instance for each
// core ruby type. Method dispatch consults these via every builtin
// type's Class() method. Created once at package init; evaluator
// registers them in each ruby environment under their ruby names so
// `Integer`, `String`, etc. resolve from user code.
//
// Hierarchy mirrors MRI:
//
//	BasicObject
//	  Object
//	    Numeric
//	      Integer
//	      Float
//	    String, Symbol, Array, Hash, Proc, Range,
//	    NilClass, TrueClass, FalseClass,
//	    Module
//	      Class
//
// Future eigenclass / singleton work will plug in via the same Class
// pointers; nothing here forbids attaching per-object singletons later.
var (
	BasicObjectClass *Class
	ObjectClass      *Class
	NumericClass     *Class
	IntegerClass     *Class
	FloatClass       *Class
	StringClass      *Class
	SymbolClass      *Class
	ArrayClass       *Class
	HashClass        *Class
	ProcClass        *Class
	RangeClass       *Class
	NilClassClass    *Class
	TrueClassClass   *Class
	FalseClassClass  *Class
	ModuleClass      *Class
	ClassClass       *Class
	RegexpClass      *Class
)

func init() {
	BasicObjectClass = NewClass("BasicObject", nil)
	ObjectClass = NewClass("Object", BasicObjectClass)
	NumericClass = NewClass("Numeric", ObjectClass)
	IntegerClass = NewClass("Integer", NumericClass)
	FloatClass = NewClass("Float", NumericClass)
	StringClass = NewClass("String", ObjectClass)
	SymbolClass = NewClass("Symbol", ObjectClass)
	ArrayClass = NewClass("Array", ObjectClass)
	HashClass = NewClass("Hash", ObjectClass)
	ProcClass = NewClass("Proc", ObjectClass)
	RangeClass = NewClass("Range", ObjectClass)
	NilClassClass = NewClass("NilClass", ObjectClass)
	TrueClassClass = NewClass("TrueClass", ObjectClass)
	FalseClassClass = NewClass("FalseClass", ObjectClass)
	ModuleClass = NewClass("Module", ObjectClass)
	ClassClass = NewClass("Class", ModuleClass)
	RegexpClass = NewClass("Regexp", ObjectClass)
}
