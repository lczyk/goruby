package evaluator

import (
	"github.com/lczyk/goruby/ast"

	"github.com/lczyk/goruby/object"
)

// Block-aware Float methods: step.

func init() {
	c := object.FloatClass

	addBlockOrPlainMethod(c, "step",
		func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
			f, ok := recv.(*object.Float)
			if !ok {
				return nil, errorf("evaluator: Float#step on non-Float %T", recv)
			}
			if len(args) < 1 || len(args) > 2 {
				return nil, errorf("evaluator: Float#step expects 1..2 args, got %d", len(args))
			}
			limit, err := toFloatValue(args[0])
			if err != nil {
				return nil, err
			}
			step := 1.0
			if len(args) == 2 {
				s, err := toFloatValue(args[1])
				if err != nil {
					return nil, err
				}
				step = s
			}
			if step == 0 {
				return nil, errorf("evaluator: ArgumentError: step can't be 0")
			}
			cond := func(v float64) bool { return v <= limit }
			if step < 0 {
				cond = func(v float64) bool { return v >= limit }
			}
			out := []object.RubyObject{}
			for v := f.Value; cond(v); v += step {
				out = append(out, object.NewFloat(v))
			}
			return object.NewArray(out...), nil
		},
		func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
			f, ok := recv.(*object.Float)
			if !ok {
				return nil, errorf("evaluator: Float#step on non-Float %T", recv)
			}
			if len(args) < 1 || len(args) > 2 {
				return nil, errorf("evaluator: Float#step expects 1..2 args, got %d", len(args))
			}
			limit, err := toFloatValue(args[0])
			if err != nil {
				return nil, err
			}
			step := 1.0
			if len(args) == 2 {
				s, err := toFloatValue(args[1])
				if err != nil {
					return nil, err
				}
				step = s
			}
			if step == 0 {
				return nil, errorf("evaluator: ArgumentError: step can't be 0")
			}
			cond := func(v float64) bool { return v <= limit }
			if step < 0 {
				cond = func(v float64) bool { return v >= limit }
			}
			for v := f.Value; cond(v); v += step {
				_, stop, err := iterStep(invoke, []object.RubyObject{object.NewFloat(v)})
				if err != nil {
					return nil, err
				}
				if stop {
					return f, nil
				}
			}
			return f, nil
		})
}
