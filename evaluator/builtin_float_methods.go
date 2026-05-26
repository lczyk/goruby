package evaluator

import (
	"math"

	"github.com/lczyk/goruby/evaluator/stdlib"
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
	add("integer?", func(env *object.Environment, r *object.Float, args []object.RubyObject) (object.RubyObject, error) {
		return object.FALSE, nil
	})
	add("real?", func(env *object.Environment, r *object.Float, args []object.RubyObject) (object.RubyObject, error) {
		return object.TRUE, nil
	})
	add("finite?", func(env *object.Environment, r *object.Float, args []object.RubyObject) (object.RubyObject, error) {
		return object.BooleanOf(!math.IsInf(r.Value, 0) && !math.IsNaN(r.Value)), nil
	})
	add("infinite?", func(env *object.Environment, r *object.Float, args []object.RubyObject) (object.RubyObject, error) {
		if math.IsInf(r.Value, 1) {
			return object.NewInteger(1), nil
		}
		if math.IsInf(r.Value, -1) {
			return object.NewInteger(-1), nil
		}
		return object.NIL, nil
	})
	add("nan?", func(env *object.Environment, r *object.Float, args []object.RubyObject) (object.RubyObject, error) {
		return object.BooleanOf(math.IsNaN(r.Value)), nil
	})
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
	add("nonzero?", func(env *object.Environment, r *object.Float, args []object.RubyObject) (object.RubyObject, error) {
		if r.Value == 0 {
			return object.NIL, nil
		}
		return r, nil
	})
	add("to_r", func(env *object.Environment, r *object.Float, args []object.RubyObject) (object.RubyObject, error) {
		return stdlib.FloatToRational(env, r.Value)
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
		return floorCeilFloat(r.Value, args, floorFloat)
	})
	add("ceil", func(env *object.Environment, r *object.Float, args []object.RubyObject) (object.RubyObject, error) {
		return floorCeilFloat(r.Value, args, ceilFloat)
	})
	add("clamp", func(env *object.Environment, r *object.Float, args []object.RubyObject) (object.RubyObject, error) {
		if len(args) == 1 {
			rng, ok := args[0].(*object.Range)
			if !ok {
				return nil, errorf("evaluator: Float#clamp needs Range or 2 args")
			}
			lo, _ := toFloatValue(rng.Begin)
			hi, _ := toFloatValue(rng.End)
			if rng.Exclusive {
				hi -= 1e-12
			}
			v := r.Value
			if v < lo {
				v = lo
			}
			if v > hi {
				v = hi
			}
			return object.NewFloat(v), nil
		}
		if len(args) != 2 {
			return nil, errorf("evaluator: Float#clamp expects 1..2 args")
		}
		lo, err := toFloatValue(args[0])
		if err != nil {
			return nil, err
		}
		hi, err := toFloatValue(args[1])
		if err != nil {
			return nil, err
		}
		v := r.Value
		if v < lo {
			v = lo
		}
		if v > hi {
			v = hi
		}
		return object.NewFloat(v), nil
	})
	add("round", func(env *object.Environment, r *object.Float, args []object.RubyObject) (object.RubyObject, error) {
		return floorCeilFloat(r.Value, args, roundFloat)
	})
	// Operator methods: dispatchable equivalents of the infix operators.
	registerNumOp := func(name string) {
		add(name, func(env *object.Environment, r *object.Float, args []object.RubyObject) (object.RubyObject, error) {
			if len(args) != 1 {
				return nil, errorf("evaluator: Float#%s expects 1 arg, got %d", name, len(args))
			}
			v, handled, err := numericInfix(env, name, r, args[0])
			if err != nil {
				return nil, err
			}
			if !handled {
				if name == "==" {
					return object.FALSE, nil
				}
				if name == "<=>" {
					return object.NIL, nil
				}
				other := "Object"
				if cls := classOfRaw(env, args[0]); cls != nil {
					other = cls.Name
				}
				return raiseBuiltin(env, "ArgumentError", "comparison of Float with "+other+" failed")
			}
			return v, nil
		})
	}
	for _, op := range []string{"+", "-", "*", "/", "%", "**", "<", "<=", ">", ">=", "<=>", "=="} {
		registerNumOp(op)
	}
}

// floorCeilFloat applies fn (floorFloat / ceilFloat / roundFloat) to
// v at the optional digit-precision arg. No arg -> Integer; arg == 0
// -> Integer (MRI matches floor(0) shape); arg > 0 -> Float rounded
// to that many decimal places. Negative-digits form (round to 10^-n)
// not implemented yet.
func floorCeilFloat(v float64, args []object.RubyObject, fn func(float64) float64) (object.RubyObject, error) {
	if len(args) == 0 {
		return object.NewInteger(int64(fn(v))), nil
	}
	digits, ok := args[0].(*object.Integer)
	if !ok {
		return nil, errorf("evaluator: Float digits must be Integer")
	}
	if digits.Value == 0 {
		return object.NewInteger(int64(fn(v))), nil
	}
	if digits.Value < 0 {
		return nil, errorf("evaluator: Float ceil/floor/round with negative digits not yet supported")
	}
	p := 1.0
	for i := int64(0); i < digits.Value; i++ {
		p *= 10
	}
	return object.NewFloat(fn(v*p) / p), nil
}
