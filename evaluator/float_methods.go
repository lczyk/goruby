package evaluator

import (
	"github.com/lczyk/goruby/object"
)

func init() {
	c := object.FloatClass
	add := func(name string, fn func(env *object.Environment, r *object.Float, args []object.RubyObject) (object.RubyObject, error)) {
		c.AddMethod(name, &object.BuiltinMethod{
			Name: name,
			Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
				return fn(env, recv.(*object.Float), args)
			},
		})
	}

	add("abs", func(env *object.Environment, r *object.Float, args []object.RubyObject) (object.RubyObject, error) {
		if r.Value < 0 {
			return object.NewFloat(-r.Value), nil
		}
		return r, nil
	})

	toInt := func(env *object.Environment, r *object.Float, args []object.RubyObject) (object.RubyObject, error) {
		return object.NewInteger(int64(r.Value)), nil
	}
	add("to_i", toInt)
	add("to_int", toInt)
	add("truncate", toInt)
	add("to_f", func(env *object.Environment, r *object.Float, args []object.RubyObject) (object.RubyObject, error) {
		return r, nil
	})
	add("to_s", func(env *object.Environment, r *object.Float, args []object.RubyObject) (object.RubyObject, error) {
		return object.NewString(env.Inspect(r)), nil
	})
	add("negative?", func(env *object.Environment, r *object.Float, args []object.RubyObject) (object.RubyObject, error) {
		return object.BooleanOf(r.Value < 0), nil
	})
	add("positive?", func(env *object.Environment, r *object.Float, args []object.RubyObject) (object.RubyObject, error) {
		return object.BooleanOf(r.Value > 0), nil
	})
	add("zero?", func(env *object.Environment, r *object.Float, args []object.RubyObject) (object.RubyObject, error) {
		return object.BooleanOf(r.Value == 0), nil
	})
	add("nan?", func(env *object.Environment, r *object.Float, args []object.RubyObject) (object.RubyObject, error) {
		return object.BooleanOf(r.Value != r.Value), nil
	})
	add("infinite?", func(env *object.Environment, r *object.Float, args []object.RubyObject) (object.RubyObject, error) {
		switch {
		case r.Value > 1e308:
			return object.NewInteger(1), nil
		case r.Value < -1e308:
			return object.NewInteger(-1), nil
		}
		return object.NIL, nil
	})
	add("floor", func(env *object.Environment, r *object.Float, args []object.RubyObject) (object.RubyObject, error) {
		return object.NewInteger(int64(floorFloat(r.Value))), nil
	})
	add("ceil", func(env *object.Environment, r *object.Float, args []object.RubyObject) (object.RubyObject, error) {
		return object.NewInteger(int64(ceilFloat(r.Value))), nil
	})
	add("round", func(env *object.Environment, r *object.Float, args []object.RubyObject) (object.RubyObject, error) {
		if len(args) == 1 {
			digits, ok := args[0].(*object.Integer)
			if !ok {
				return nil, errorf("evaluator: Float#round digits must be Integer")
			}
			p := 1.0
			for i := int64(0); i < digits.Value; i++ {
				p *= 10
			}
			return object.NewFloat(roundFloat(r.Value*p) / p), nil
		}
		return object.NewInteger(int64(roundFloat(r.Value))), nil
	})
}
