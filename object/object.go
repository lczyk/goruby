// Package object implements the runtime value types for the goruby
// evaluator.
//
// Design note: leaf value types (Integer, Float, Symbol, the Nil and Boolean
// singletons) are pointer-free so their heap allocations land in
// noscan-eligible classes -- the Go GC walks past them without inspecting
// fields. Same discipline as the AST leaf nodes in package ast.
//
// Compound types (Array, Hash, String, Class, Proc, etc.) inherently hold
// pointers and are scanned. To keep their pointer footprint small, prefer
// dense []entry layouts over map[string]Object where ordering and cardinality
// allow.
package object

// Type is a coarse runtime-type discriminator. Used for fast switches in
// the evaluator before falling back to method dispatch.
type Type uint8

const (
	NIL_OBJ Type = iota
	BOOL_OBJ
	INTEGER_OBJ
	FLOAT_OBJ
	SYMBOL_OBJ
	STRING_OBJ
	ARRAY_OBJ
	HASH_OBJ
	CLASS_OBJ
	MODULE_OBJ
	OBJECT_OBJ
	PROC_OBJ
	EXCEPTION_OBJ
	RETURN_VALUE_OBJ
)

// RubyObject is the runtime interface every value implements. The zero
// alloc / fast path lives in concrete types; methods on this interface are
// the dispatch entry points.
type RubyObject interface {
	// Inspect returns the value's `inspect` representation under the
	// evaluator's latest-version conventions. For version-aware formatting
	// (currently only Hash and Array inspect output, which changed in 3.4)
	// call the free function Inspect(obj, v) in this package.
	Inspect() string

	// Type returns the coarse type tag.
	Type() Type

	// Class returns the value's runtime class. The placeholder returns nil
	// for now; populated once the class machinery lands.
	Class() RubyClass
}

// RubyClass is the interface implemented by class objects. Kept minimal
// until method dispatch lands; flesh out as the evaluator needs it.
type RubyClass interface {
	Name() string
	Super() RubyClass
}
