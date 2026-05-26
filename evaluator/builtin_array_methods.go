package evaluator

import (
	"github.com/lczyk/goruby/object"
)

// arrayCoerce extracts an *Array from obj, calling #to_ary on
// non-Array Instances as MRI does for the binary Array ops. Returns
// (nil, false) when neither path yields an Array.
func arrayCoerce(env *object.Environment, obj object.RubyObject) (*object.Array, bool) {
	if a, ok := obj.(*object.Array); ok {
		return a, true
	}
	if inst, ok := obj.(*object.Instance); ok {
		if m, found := inst.C.LookupMethod("to_ary"); found {
			var v object.RubyObject
			var err error
			switch mm := m.(type) {
			case *object.UserMethod:
				v, err = invokeMethodOn(env, inst, mm, nil, nil)
			case *object.BuiltinMethod:
				v, err = mm.Fn(env, inst, nil, nil)
			}
			if err == nil {
				if a, ok := v.(*object.Array); ok {
					return a, true
				}
			}
		}
	}
	return nil, false
}

var arrayMethodNames = []string{
	"length", "size", "count", "first", "last", "push", "append",
	"pop", "shift", "unshift", "prepend", "delete", "delete_at",
	"concat", "reverse", "sort", "include?", "member?", "empty?", "any?", "all?",
	"none?", "one?", "to_h", "to_a", "dig", "each_with_index",
	"each_slice", "each_cons", "zip", "take", "drop", "join", "min",
	"minmax", "max", "sum", "grep", "uniq", "compact", "flatten", "fetch",
	"reduce", "inject", "tally", "clear",
	"fill", "rotate", "rotate!", "transpose", "sample",
	"reverse!", "sort!", "uniq!", "compact!", "flatten!",
	"index", "find_index", "rindex",
	"assoc", "rassoc",
	"product", "combination", "permutation",
	"lazy",
	"slice", "[]", "values_at",
	"to_ary", "replace", "reverse_each",
	"shuffle",
}

func init() {
	c := object.ArrayClass
	for _, name := range arrayMethodNames {
		n := name
		c.AddMethod(n, &object.BuiltinMethod{
			Name: n,
			Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
				return callArrayMethod(env, recv.(*object.Array), n, args)
			},
		})
	}
	// Operator methods. Mirror the infix paths in eval.go so
	// arr.send(:+, other) resolves the same as arr + other.
	c.AddMethod("+", &object.BuiltinMethod{Name: "+", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		l, _ := recv.(*object.Array)
		r, ok := arrayCoerce(env, args[0])
		if !ok {
			return nil, errorf("evaluator: Array#+: expected Array, got %T", args[0])
		}
		out := make([]object.RubyObject, 0, len(l.Elements)+len(r.Elements))
		out = append(out, l.Elements...)
		out = append(out, r.Elements...)
		return object.NewArray(out...), nil
	}})
	c.AddMethod("-", &object.BuiltinMethod{Name: "-", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		l, _ := recv.(*object.Array)
		r, ok := arrayCoerce(env, args[0])
		if !ok {
			return nil, errorf("evaluator: Array#-: expected Array, got %T", args[0])
		}
		out := []object.RubyObject{}
	nextElem:
		for _, e := range l.Elements {
			for _, x := range r.Elements {
				if rubyEqual(e, x) {
					continue nextElem
				}
			}
			out = append(out, e)
		}
		return object.NewArray(out...), nil
	}})
	c.AddMethod("<<", &object.BuiltinMethod{Name: "<<", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		l, _ := recv.(*object.Array)
		l.Elements = append(l.Elements, args[0])
		return l, nil
	}})
	c.AddMethod("==", &object.BuiltinMethod{Name: "==", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		return object.BooleanOf(rubyEqual(recv, args[0])), nil
	}})
}
