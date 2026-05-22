package evaluator

import (
	"github.com/lczyk/goruby/ast"

	"github.com/lczyk/goruby/object"
)

// Block-aware String methods: each_char, each_byte, each_line, gsub, sub.

func init() {
	c := object.StringClass

	stringRecv := func(env *object.Environment, recv object.RubyObject, name string) (string, error) {
		s, ok := stringText(env, recv)
		if !ok {
			return "", errorf("evaluator: String#%s on non-String %T", name, recv)
		}
		return s, nil
	}

	addBlockOrPlainMethod(c, "each_char",
		func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
			// No-block each_char returns an Enumerator over the chars.
			s, err := stringRecv(env, recv, "each_char")
			if err != nil {
				return nil, err
			}
			chars := []rune(s)
			out := make([]object.RubyObject, len(chars))
			for i, r := range chars {
				out[i] = object.NewString(string(r))
			}
			return &object.Enumerator{Receiver: object.NewArray(out...), Method: "each"}, nil
		},
		func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
			s, err := stringRecv(env, recv, "each_char")
			if err != nil {
				return nil, err
			}
			for _, ch := range []rune(s) {
				_, stop, err := iterStep(invoke, []object.RubyObject{object.NewString(string(ch))})
				if err != nil {
					return nil, err
				}
				if stop {
					return recv, nil
				}
			}
			return recv, nil
		})

	addBlockOrPlainMethod(c, "each_byte",
		func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
			s, err := stringRecv(env, recv, "each_byte")
			if err != nil {
				return nil, err
			}
			out := make([]object.RubyObject, len(s))
			for i := range s {
				out[i] = object.NewInteger(int64(s[i]))
			}
			return &object.Enumerator{Receiver: object.NewArray(out...), Method: "each"}, nil
		},
		func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
			s, err := stringRecv(env, recv, "each_byte")
			if err != nil {
				return nil, err
			}
			for i := 0; i < len(s); i++ {
				_, stop, err := iterStep(invoke, []object.RubyObject{object.NewInteger(int64(s[i]))})
				if err != nil {
					return nil, err
				}
				if stop {
					return recv, nil
				}
			}
			return recv, nil
		})

	addBlockMethod(c, "each_line", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
		s, err := stringRecv(env, recv, "each_line")
		if err != nil {
			return nil, err
		}
		parts := splitLinesKeepNL(s)
		for _, p := range parts {
			_, stop, err := iterStep(invoke, []object.RubyObject{object.NewString(p)})
			if err != nil {
				return nil, err
			}
			if stop {
				return recv, nil
			}
		}
		return recv, nil
	})

	// gsub / sub: plain form handled by string_methods.go's existing
	// registration; block form below.
	addBlockOrPlainMethod(c, "gsub",
		func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
			v, _, err := callStringMethod(env, recv, "gsub", args)
			return v, err
		},
		func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
			s, err := stringRecv(env, recv, "gsub")
			if err != nil {
				return nil, err
			}
			if len(args) != 1 {
				return nil, errorf("evaluator: String#gsub { ... } expects 1 pattern arg, got %d", len(args))
			}
			return stringGsubBlock(env, s, args[0], invoke)
		})

	addBlockOrPlainMethod(c, "sub",
		func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
			v, _, err := callStringMethod(env, recv, "sub", args)
			return v, err
		},
		func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
			s, err := stringRecv(env, recv, "sub")
			if err != nil {
				return nil, err
			}
			if len(args) != 1 {
				return nil, errorf("evaluator: String#sub { ... } expects 1 pattern arg, got %d", len(args))
			}
			return stringSubBlock(env, s, args[0], invoke)
		})
}
