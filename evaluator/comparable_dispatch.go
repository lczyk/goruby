package evaluator

import (
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
				return nil, errorf("evaluator: NoMethodError: undefined method `%s' for instance of %T", name, recv)
			}
			// Object#== defaults to pointer identity when no <=> is
			// defined, matching MRI's `Object#==` (equivalent to
			// `equal?`). Other comparisons require <=>.
			if name == "==" {
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
			return nil, errorf("evaluator: NoMethodError: undefined method `between?' for instance of %T", recv)
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
			return nil, errorf("evaluator: NoMethodError: undefined method `clamp' for instance of %T", recv)
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
	inst, ok := recv.(*object.Instance)
	if !ok {
		return 0, errorf("evaluator: NoMethodError: undefined method `%s' for %T", name, recv)
	}
	m, ok := dispatchClass(env, inst).LookupSpaceship()
	if !ok {
		return 0, errorf("evaluator: NoMethodError: undefined method `%s' for instance of %s", name, inst.C.Name)
	}
	um, ok := m.(*object.UserMethod)
	if !ok {
		return 0, errorf("evaluator: NoMethodError: undefined method `%s' for instance of %s", name, inst.C.Name)
	}
	v, err := invokeMethodOn(env, inst, um, []object.RubyObject{other}, nil)
	if err != nil {
		return 0, err
	}
	i, ok := v.(*object.Integer)
	if !ok {
		return 0, errorf("evaluator: <=> returned %T, expected Integer", v)
	}
	return i.Value, nil
}
