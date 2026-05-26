package evaluator

import (
	"github.com/lczyk/goruby/ast"

	"github.com/lczyk/goruby/object"
)

// Block-aware Integer methods: times, upto, downto, step.

func init() {
	c := object.IntegerClass

	asInt := func(recv object.RubyObject, name string) (*object.Integer, error) {
		i, ok := recv.(*object.Integer)
		if !ok {
			return nil, errorf("evaluator: Integer#%s on non-Integer %T", name, recv)
		}
		return i, nil
	}

	addBlockOrPlainMethod(c, "times",
		func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
			// No-block times returns an Enumerator over [0, n).
			i, err := asInt(recv, "times")
			if err != nil {
				return nil, err
			}
			out := make([]object.RubyObject, 0, i.Value)
			for k := int64(0); k < i.Value; k++ {
				out = append(out, object.NewInteger(k))
			}
			return &object.Enumerator{Receiver: object.NewArray(out...), Method: "each"}, nil
		},
		func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
			i, err := asInt(recv, "times")
			if err != nil {
				return nil, err
			}
			for k := int64(0); k < i.Value; k++ {
				_, stop, err := yieldOne(invoke, object.NewInteger(k))
				if err != nil {
					return nil, err
				}
				if stop {
					return i, nil
				}
			}
			return i, nil
		})

	addBlockOrPlainMethod(c, "upto",
		func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
			i, err := asInt(recv, "upto")
			if err != nil {
				return nil, err
			}
			if len(args) != 1 {
				return nil, errorf("evaluator: Integer#upto expects 1 arg, got %d", len(args))
			}
			to, ok := args[0].(*object.Integer)
			if !ok {
				return nil, errorf("evaluator: Integer#upto needs Integer arg")
			}
			out := make([]object.RubyObject, 0)
			for k := i.Value; k <= to.Value; k++ {
				out = append(out, object.NewInteger(k))
			}
			return &object.Enumerator{Receiver: object.NewArray(out...), Method: "each"}, nil
		},
		func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
			i, err := asInt(recv, "upto")
			if err != nil {
				return nil, err
			}
			if len(args) != 1 {
				return nil, errorf("evaluator: Integer#upto expects 1 arg, got %d", len(args))
			}
			to, ok := args[0].(*object.Integer)
			if !ok {
				return nil, errorf("evaluator: Integer#upto needs Integer arg")
			}
			for k := i.Value; k <= to.Value; k++ {
				_, stop, err := yieldOne(invoke, object.NewInteger(k))
				if err != nil {
					return nil, err
				}
				if stop {
					return i, nil
				}
			}
			return i, nil
		})

	addBlockOrPlainMethod(c, "downto",
		func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
			i, err := asInt(recv, "downto")
			if err != nil {
				return nil, err
			}
			if len(args) != 1 {
				return nil, errorf("evaluator: Integer#downto expects 1 arg, got %d", len(args))
			}
			to, ok := args[0].(*object.Integer)
			if !ok {
				return nil, errorf("evaluator: Integer#downto needs Integer arg")
			}
			out := make([]object.RubyObject, 0)
			for k := i.Value; k >= to.Value; k-- {
				out = append(out, object.NewInteger(k))
			}
			return &object.Enumerator{Receiver: object.NewArray(out...), Method: "each"}, nil
		},
		func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
			i, err := asInt(recv, "downto")
			if err != nil {
				return nil, err
			}
			if len(args) != 1 {
				return nil, errorf("evaluator: Integer#downto expects 1 arg, got %d", len(args))
			}
			to, ok := args[0].(*object.Integer)
			if !ok {
				return nil, errorf("evaluator: Integer#downto needs Integer arg")
			}
			for k := i.Value; k >= to.Value; k-- {
				_, stop, err := yieldOne(invoke, object.NewInteger(k))
				if err != nil {
					return nil, err
				}
				if stop {
					return i, nil
				}
			}
			return i, nil
		})

	addBlockOrPlainMethod(c, "step",
		func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
			i, err := asInt(recv, "step")
			if err != nil {
				return nil, err
			}
			if len(args) < 1 || len(args) > 2 {
				return nil, errorf("evaluator: Integer#step expects 1..2 args, got %d", len(args))
			}
			limit, ok := args[0].(*object.Integer)
			if !ok {
				return nil, errorf("evaluator: Integer#step needs Integer limit")
			}
			step := int64(1)
			if len(args) == 2 {
				sa, ok := args[1].(*object.Integer)
				if !ok {
					return nil, errorf("evaluator: Integer#step needs Integer step")
				}
				step = sa.Value
			}
			if step == 0 {
				return nil, errorf("evaluator: ArgumentError: step can't be 0")
			}
			cond := func(k int64) bool { return k <= limit.Value }
			if step < 0 {
				cond = func(k int64) bool { return k >= limit.Value }
			}
			out := []object.RubyObject{}
			for k := i.Value; cond(k); k += step {
				out = append(out, object.NewInteger(k))
			}
			return object.NewArray(out...), nil
		},
		func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
			i, err := asInt(recv, "step")
			if err != nil {
				return nil, err
			}
			if len(args) < 1 || len(args) > 2 {
				return nil, errorf("evaluator: Integer#step expects 1..2 args, got %d", len(args))
			}
			limit, ok := args[0].(*object.Integer)
			if !ok {
				return nil, errorf("evaluator: Integer#step needs Integer limit")
			}
			step := int64(1)
			if len(args) == 2 {
				sa, ok := args[1].(*object.Integer)
				if !ok {
					return nil, errorf("evaluator: Integer#step needs Integer step")
				}
				step = sa.Value
			}
			if step == 0 {
				return nil, errorf("evaluator: ArgumentError: step can't be 0")
			}
			cond := func(k int64) bool { return k <= limit.Value }
			if step < 0 {
				cond = func(k int64) bool { return k >= limit.Value }
			}
			for k := i.Value; cond(k); k += step {
				_, stop, err := yieldOne(invoke, object.NewInteger(k))
				if err != nil {
					return nil, err
				}
				if stop {
					return i, nil
				}
			}
			return i, nil
		})
}
