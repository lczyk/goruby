package evaluator

import (
	"github.com/lczyk/goruby/object"
)

// instanceIncludesEnumerable reports whether inst's class transitively
// includes the Enumerable module, OR defines a user-level `each` method
// somewhere on its own class chain (excluding ObjectClass). Duck-typed
// Enumerable: MRI requires the explicit include, but goruby's
// Enumerable derivations sit on ObjectClass, so any class that has an
// `each` should get the derivations against it. Rake::FileList relies
// on this -- it defines each via class_eval delegation without
// including Enumerable.
func instanceIncludesEnumerable(env *object.Environment, inst *object.Instance) bool {
	if enum, ok := env.Get("Enumerable"); ok {
		if mod, ok := enum.(*object.Class); ok {
			if inst.C.IsAncestor(mod) {
				return true
			}
		}
	}
	// Walk own class + super up to (but not including) ObjectClass.
	for cur := inst.C; cur != nil && cur != object.ObjectClass; cur = cur.Super {
		if _, ok := cur.Methods["each"]; ok {
			return true
		}
		for _, inc := range cur.Includes {
			if _, ok := inc.Methods["each"]; ok {
				return true
			}
		}
	}
	return false
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
