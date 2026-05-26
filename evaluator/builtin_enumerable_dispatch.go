package evaluator

import (
	"github.com/lczyk/goruby/object"
)

// Enumerable derivations on ObjectClass. When the receiver is an
// Instance whose class includes Enumerable and defines `each`, the
// standard Enumerable methods are derived from each. Falls through
// to NoMethodError otherwise -- builtin types (Array, Hash, Range)
// define their own implementations on their respective classes, so
// those wins via LookupMethod's chain walk before reaching Object.

func init() {
	c := object.ObjectClass

	add := func(name string, fn func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error)) {
		c.AddMethod(name, &object.BuiltinMethod{
			Name: name,
			Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
				return fn(env, recv, args)
			},
		})
	}

	// Methods that materialise the iteration to an Array, then dispatch
	// the request to Array.
	materialiseToArray := func(name string) func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
		return func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
			inst, eachUM, err := enumGate(env, recv, name)
			if err != nil {
				return nil, err
			}
			out := []object.RubyObject{}
			_, err = invokeMethodOn(env, inst, eachUM, nil, &goBlockMarker{fn: func(a []object.RubyObject) (object.RubyObject, error) {
				out = append(out, joinYieldArgs(a))
				return object.NIL, nil
			}})
			if err != nil {
				return nil, err
			}
			return callMethod(env, object.NewArray(out...), name, args)
		}
	}
	for _, n := range []string{"sort", "uniq", "reverse", "tally", "sum", "join", "flatten", "zip", "take", "drop", "reduce", "inject"} {
		add(n, materialiseToArray(n))
	}

	add("to_a", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
		return enumToArray(env, recv, "to_a")
	})
	add("entries", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
		return enumToArray(env, recv, "entries")
	})

	add("count", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
		inst, eachUM, err := enumGate(env, recv, "count")
		if err != nil {
			return nil, err
		}
		n := 0
		_, err = invokeMethodOn(env, inst, eachUM, nil, &goBlockMarker{fn: func(a []object.RubyObject) (object.RubyObject, error) {
			n++
			return object.NIL, nil
		}})
		if err != nil {
			return nil, err
		}
		return object.NewInteger(int64(n)), nil
	})

	add("include?", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
		inst, eachUM, err := enumGate(env, recv, "include?")
		if err != nil {
			return nil, err
		}
		if len(args) != 1 {
			return nil, errorf("evaluator: include? expects 1 arg")
		}
		found := false
		_, err = invokeMethodOn(env, inst, eachUM, nil, &goBlockMarker{fn: func(a []object.RubyObject) (object.RubyObject, error) {
			if rubyEqual(joinYieldArgs(a), args[0]) {
				found = true
			}
			return object.NIL, nil
		}})
		if err != nil {
			return nil, err
		}
		return object.BooleanOf(found), nil
	})

	add("first", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
		inst, eachUM, err := enumGate(env, recv, "first")
		if err != nil {
			return nil, err
		}
		var first object.RubyObject = object.NIL
		got := false
		_, err = invokeMethodOn(env, inst, eachUM, nil, &goBlockMarker{fn: func(a []object.RubyObject) (object.RubyObject, error) {
			if got {
				return object.NIL, &breakSignal{Value: first}
			}
			first = joinYieldArgs(a)
			got = true
			return object.NIL, nil
		}})
		if err != nil {
			return nil, err
		}
		return first, nil
	})

	minmax := func(want int) func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
		name := "min"
		if want > 0 {
			name = "max"
		}
		return func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
			inst, eachUM, err := enumGate(env, recv, name)
			if err != nil {
				return nil, err
			}
			var best object.RubyObject
			_, err = invokeMethodOn(env, inst, eachUM, nil, &goBlockMarker{fn: func(a []object.RubyObject) (object.RubyObject, error) {
				v := joinYieldArgs(a)
				if best == nil {
					best = v
					return object.NIL, nil
				}
				c, ok := compareObjectsEnv(env, v, best)
				if !ok {
					return nil, errorf("evaluator: comparison failed in %s", name)
				}
				if (want < 0 && c < 0) || (want > 0 && c > 0) {
					best = v
				}
				return object.NIL, nil
			}})
			if err != nil {
				return nil, err
			}
			if best == nil {
				return object.NIL, nil
			}
			return best, nil
		}
	}
	add("min", minmax(-1))
	add("max", minmax(1))
}

// enumGate returns the receiver as an Instance plus its `each`
// UserMethod, provided the class includes Enumerable and defines
// `each`. Returns a NoMethodError otherwise.
func enumGate(env *object.Environment, recv object.RubyObject, name string) (*object.Instance, *object.UserMethod, error) {
	raise := func(target string) error {
		_, e := raiseBuiltin(env, "NoMethodError", "undefined method `"+name+"' for "+target)
		return e
	}
	inst, ok := recv.(*object.Instance)
	if !ok {
		cls := classOfRaw(env, recv)
		target := "Object"
		if cls != nil {
			target = cls.Name
		}
		return nil, nil, raise(target)
	}
	if !instanceIncludesEnumerable(env, inst) {
		return nil, nil, raise("instance of " + inst.C.Name)
	}
	m, ok := dispatchClass(env, inst).LookupMethod("each")
	if !ok {
		return nil, nil, raise("instance of " + inst.C.Name)
	}
	um, ok := m.(*object.UserMethod)
	if !ok {
		return nil, nil, raise("instance of " + inst.C.Name)
	}
	return inst, um, nil
}

func enumToArray(env *object.Environment, recv object.RubyObject, name string) (object.RubyObject, error) {
	inst, eachUM, err := enumGate(env, recv, name)
	if err != nil {
		return nil, err
	}
	out := []object.RubyObject{}
	_, err = invokeMethodOn(env, inst, eachUM, nil, &goBlockMarker{fn: func(a []object.RubyObject) (object.RubyObject, error) {
		out = append(out, joinYieldArgs(a))
		return object.NIL, nil
	}})
	if err != nil {
		return nil, err
	}
	return object.NewArray(out...), nil
}
