package evaluator

import (
	"github.com/lczyk/goruby/object"
)

// instanceIncludesEnumerable reports whether inst's class transitively
// includes the Enumerable module (or `each` is the only thing defined
// and the user clearly meant it -- but we stick to explicit include
// for now).
func instanceIncludesEnumerable(env *object.Environment, inst *object.Instance) bool {
	enum, ok := env.Get("Enumerable")
	if !ok {
		return false
	}
	mod, ok := enum.(*object.Class)
	if !ok {
		return false
	}
	return inst.C.IsAncestor(mod)
}

// joinYieldArgs collapses a yield's args into a single value: empty
// yields -> nil, single -> the value, multiple -> an Array.
func joinYieldArgs(a []object.RubyObject) object.RubyObject {
	switch len(a) {
	case 0:
		return object.NIL
	case 1:
		return a[0]
	}
	return object.NewArray(a...)
}
