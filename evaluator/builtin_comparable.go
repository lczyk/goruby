package evaluator

import (
	"fmt"

	"github.com/lczyk/goruby/object"
)

// Comparable derivations on ObjectClass. When a user class defines
// `<=>`, the standard Comparable methods (<, <=, >, >=, ==, between?,
// clamp) are derived automatically, matching MRI's `include
// Comparable` behaviour without requiring the explicit include. Only
// Instance receivers participate; built-in types either define these
// directly (e.g. IntegerClass#<) or have their own infix path.

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

	cmp := func(name string) func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
		return func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
			if len(args) != 1 {
				return raiseBuiltin(env, "ArgumentError", fmt.Sprintf("wrong number of arguments (given %d, expected 1)", len(args)))
			}
			// Object#== defaults to pointer identity when no <=> is
			// defined, matching MRI's `Object#==` (equivalent to
			// `equal?`). Other comparisons require <=>.
			if name == "==" {
				// Identity short-circuit: equal objects are always ==
				// regardless of what <=> returns. MRI guarantees this
				// for Object#== fallback even when Comparable is
				// included. Rake::LATE is a Singleton whose <=> always
				// returns 1; without this, late == late would be false.
				if recv == args[0] {
					return object.TRUE, nil
				}
				if inst, ok := recv.(*object.Instance); ok {
					if _, has := dispatchClass(env, inst).LookupSpaceship(); !has {
						return object.BooleanOf(recv == args[0]), nil
					}
				} else {
					return object.BooleanOf(recv == args[0]), nil
				}
			}
			c, err := spaceshipCompare(env, recv, args[0], name)
			if err != nil {
				return nil, err
			}
			switch name {
			case "<":
				return object.BooleanOf(c < 0), nil
			case "<=":
				return object.BooleanOf(c <= 0), nil
			case ">":
				return object.BooleanOf(c > 0), nil
			case ">=":
				return object.BooleanOf(c >= 0), nil
			case "==":
				return object.BooleanOf(c == 0), nil
			}
			return nil, errorf("evaluator: internal: unknown comparison %q", name)
		}
	}
	for _, n := range []string{"<", "<=", ">", ">=", "=="} {
		add(n, cmp(n))
	}

	add("between?", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
		if len(args) != 2 {
			return raiseBuiltin(env, "ArgumentError", fmt.Sprintf("wrong number of arguments (given %d, expected 2)", len(args)))
		}
		lo, err := spaceshipCompare(env, recv, args[0], "between?")
		if err != nil {
			return nil, err
		}
		hi, err := spaceshipCompare(env, recv, args[1], "between?")
		if err != nil {
			return nil, err
		}
		return object.BooleanOf(lo >= 0 && hi <= 0), nil
	})

	add("clamp", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
		if len(args) != 2 {
			return raiseBuiltin(env, "ArgumentError", fmt.Sprintf("wrong number of arguments (given %d, expected 2)", len(args)))
		}
		loCmp, err := spaceshipCompare(env, recv, args[0], "clamp")
		if err != nil {
			return nil, err
		}
		if loCmp < 0 {
			return args[0], nil
		}
		hiCmp, err := spaceshipCompare(env, recv, args[1], "clamp")
		if err != nil {
			return nil, err
		}
		if hiCmp > 0 {
			return args[1], nil
		}
		return recv, nil
	})
}

// spaceshipCompare invokes the receiver's <=> against other, returning
// the resulting integer. Errors with NoMethodError if recv is not an
// Instance or its class doesn't define <=>. Uses the per-class
// spaceship cache to skip the LookupMethod walk on repeated calls --
// the common case in any sort / min / max / comparison loop.
func spaceshipCompare(env *object.Environment, recv, other object.RubyObject, name string) (int64, error) {
	raise := func(target string) error {
		_, e := raiseBuiltin(env, "NoMethodError", "undefined method `"+name+"' for "+target)
		return e
	}
	inst, ok := recv.(*object.Instance)
	if !ok {
		// Non-Instance receiver: dispatch <=> via callMethod so
		// String / Integer / Float / Symbol can participate too
		// (their <=> lives on the respective builtin class).
		v, err := callMethod(env, recv, "<=>", []object.RubyObject{other})
		if err != nil {
			return 0, err
		}
		if i, ok := v.(*object.Integer); ok {
			return i.Value, nil
		}
		cls := classOfRaw(env, recv)
		target := "Object"
		if cls != nil {
			target = cls.Name
		}
		return 0, raise(target)
	}
	m, ok := dispatchClass(env, inst).LookupSpaceship()
	if !ok {
		return 0, raise("instance of " + inst.C.Name)
	}
	var v object.RubyObject
	var err error
	switch mm := m.(type) {
	case *object.UserMethod:
		v, err = invokeMethodOn(env, inst, mm, []object.RubyObject{other}, nil)
	case *object.BuiltinMethod:
		// Builtin <=> on Go-shipped classes (Time, etc.) dispatches
		// directly through Fn -- callers don't need to thread through
		// invokeMethodOn's frame/env machinery.
		v, err = mm.Fn(env, inst, []object.RubyObject{other}, nil)
	default:
		return 0, raise("instance of " + inst.C.Name)
	}
	if err != nil {
		return 0, err
	}
	if _, isNil := v.(*object.Nil); isNil {
		// MRI: <=> returning nil means "incomparable"; the calling
		// op (>, <, etc.) raises ArgumentError. Translate to a
		// rescuable Ruby error.
		recvName := "Object"
		if cls := classOfRaw(env, recv); cls != nil {
			recvName = cls.Name
		}
		otherName := "Object"
		if cls := classOfRaw(env, other); cls != nil {
			otherName = cls.Name
		}
		_, e := raiseBuiltin(env, "ArgumentError", "comparison of "+recvName+" with "+otherName+" failed")
		return 0, e
	}
	i, ok := v.(*object.Integer)
	if !ok {
		return 0, errorf("evaluator: <=> returned %T, expected Integer", v)
	}
	return i.Value, nil
}
