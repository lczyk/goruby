package evaluator

import (
	"strconv"

	"github.com/lczyk/goruby/object"
)

// Builtin Integer methods, registered on object.IntegerClass at init.
// Migrated out of callMethod's hand-rolled switch so dispatch goes
// through Send. Legacy switch arms for the same names still exist as
// dead code (unreachable now that Send catches them first); they get
// removed once every type has migrated.

func init() {
	c := object.IntegerClass
	add := func(name string, fn func(env *object.Environment, r *object.Integer, args []object.RubyObject) (object.RubyObject, error)) {
		c.AddMethod(name, &object.BuiltinMethod{
			Name: name,
			Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
				return fn(env, recv.(*object.Integer), args)
			},
		})
	}

	add("even?", func(env *object.Environment, r *object.Integer, args []object.RubyObject) (object.RubyObject, error) {
		return object.BooleanOf(r.Value%2 == 0), nil
	})
	add("odd?", func(env *object.Environment, r *object.Integer, args []object.RubyObject) (object.RubyObject, error) {
		return object.BooleanOf(r.Value%2 != 0), nil
	})
	add("zero?", func(env *object.Environment, r *object.Integer, args []object.RubyObject) (object.RubyObject, error) {
		return object.BooleanOf(r.Value == 0), nil
	})
	add("negative?", func(env *object.Environment, r *object.Integer, args []object.RubyObject) (object.RubyObject, error) {
		return object.BooleanOf(r.Value < 0), nil
	})
	add("positive?", func(env *object.Environment, r *object.Integer, args []object.RubyObject) (object.RubyObject, error) {
		return object.BooleanOf(r.Value > 0), nil
	})
	add("succ", func(env *object.Environment, r *object.Integer, args []object.RubyObject) (object.RubyObject, error) {
		return object.NewInteger(r.Value + 1), nil
	})
	add("next", func(env *object.Environment, r *object.Integer, args []object.RubyObject) (object.RubyObject, error) {
		return object.NewInteger(r.Value + 1), nil
	})
	add("pred", func(env *object.Environment, r *object.Integer, args []object.RubyObject) (object.RubyObject, error) {
		return object.NewInteger(r.Value - 1), nil
	})
	add("to_i", func(env *object.Environment, r *object.Integer, args []object.RubyObject) (object.RubyObject, error) {
		return r, nil
	})
	add("to_int", func(env *object.Environment, r *object.Integer, args []object.RubyObject) (object.RubyObject, error) {
		return r, nil
	})
	add("to_f", func(env *object.Environment, r *object.Integer, args []object.RubyObject) (object.RubyObject, error) {
		return object.NewFloat(float64(r.Value)), nil
	})
	add("abs", func(env *object.Environment, r *object.Integer, args []object.RubyObject) (object.RubyObject, error) {
		if r.Value < 0 {
			return object.NewInteger(-r.Value), nil
		}
		return r, nil
	})
	add("chr", func(env *object.Environment, r *object.Integer, args []object.RubyObject) (object.RubyObject, error) {
		return object.NewString(string(rune(r.Value))), nil
	})
	add("to_s", func(env *object.Environment, r *object.Integer, args []object.RubyObject) (object.RubyObject, error) {
		base := 10
		if len(args) == 1 {
			b, ok := args[0].(*object.Integer)
			if !ok {
				return nil, errorf("evaluator: Integer#to_s base must be Integer")
			}
			base = int(b.Value)
			if base < 2 || base > 36 {
				return nil, errorf("evaluator: ArgumentError: invalid radix %d", base)
			}
		}
		return object.NewString(strconv.FormatInt(r.Value, base)), nil
	})
}
