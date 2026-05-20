package evaluator

import (
	"strconv"
	"strings"

	"github.com/lczyk/goruby/ast"
	"github.com/lczyk/goruby/object"
)

// callKernel dispatches Kernel-level (implicit-self) calls. Until a full
// method registry exists, the literals corpus only needs `puts` and `p`.
// User-defined top-level methods take precedence over kernel builtins
// (matching MRI's `Object#method` resolution).
func callKernel(env *object.Environment, name string, args []object.RubyObject) (object.RubyObject, error) {
	if m, ok := env.GetMethod(name); ok {
		if um, ok := m.(*object.UserMethod); ok {
			return callUserMethod(env, um, args)
		}
	}
	switch name {
	case "puts":
		return kernelPuts(env, args)
	case "p", "pp":
		return kernelP(env, args)
	case "print":
		return kernelPrint(env, args)
	case "printf":
		return kernelPrintf(env, args)
	case "sprintf", "format":
		return kernelSprintf(env, args)
	case "raise":
		return kernelRaise(env, args)
	case "Integer":
		return kernelInteger(env, args)
	case "Float":
		return kernelFloat(env, args)
	case "String":
		return kernelString(env, args)
	case "Array":
		return kernelArray(args)
	case "gets":
		return stdinGets(env, args)
	case "lambda":
		return nil, errorf("evaluator: Kernel#lambda without block not supported; use ->( ){ ... }")
	}
	return nil, errorf("evaluator: NoMethodError: undefined method `%s' for main:Object", name)
}

// kernelLoop implements Kernel#loop { ... } -- runs the block forever
// until `break` short-circuits it. Returns the break value (or nil).
func kernelLoop(env *object.Environment, blk *ast.BlockExpression) (object.RubyObject, error) {
	for {
		v, stop, err := iterStep(func(a []object.RubyObject) (object.RubyObject, error) {
			return invokeBlock(env, blk, a)
		}, nil)
		if err != nil {
			return nil, err
		}
		if stop {
			return v, nil
		}
	}
}

// kernelPuts implements Kernel#puts: writes each argument followed by a
// newline (unless the value already ends in one). With zero args, writes
// a single newline. Arrays are unrolled element-by-element.
func kernelPuts(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	w := env.Stdout()
	if len(args) == 0 {
		_, _ = w.Write([]byte{'\n'})
		return object.NIL, nil
	}
	for _, a := range args {
		if arr, ok := a.(*object.Array); ok {
			if _, err := kernelPuts(env, arr.Elements); err != nil {
				return nil, err
			}
			continue
		}
		s := putsString(env, a)
		_, _ = w.Write([]byte(s))
		if len(s) == 0 || s[len(s)-1] != '\n' {
			_, _ = w.Write([]byte{'\n'})
		}
	}
	return object.NIL, nil
}

// kernelInteger implements Kernel#Integer(v, base=10) for the common
// String / Integer / Float inputs.
func kernelInteger(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	if len(args) < 1 || len(args) > 2 {
		return nil, errorf("evaluator: Integer: wrong number of arguments (%d)", len(args))
	}
	base := 10
	if len(args) == 2 {
		bi, ok := args[1].(*object.Integer)
		if !ok {
			return nil, errorf("evaluator: Integer: base must be Integer")
		}
		base = int(bi.Value)
	}
	switch v := args[0].(type) {
	case *object.Integer:
		return v, nil
	case *object.Float:
		return object.NewInteger(int64(v.Value)), nil
	case *object.String:
		s := strings.TrimSpace(string(v.Buf))
		s = strings.ReplaceAll(s, "_", "")
		// Accept the standard ruby 0x/0o/0b/0d prefixes when base is
		// auto-detected (i.e. caller didn't specify and string carries
		// one).
		if len(args) == 1 {
			if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
				s = s[2:]
				base = 16
			} else if strings.HasPrefix(s, "0b") || strings.HasPrefix(s, "0B") {
				s = s[2:]
				base = 2
			} else if strings.HasPrefix(s, "0o") || strings.HasPrefix(s, "0O") {
				s = s[2:]
				base = 8
			}
		}
		n, err := strconv.ParseInt(s, base, 64)
		if err != nil {
			return raiseBuiltin(env, "ArgumentError", "invalid value for Integer(): \""+string(v.Buf)+"\"")
		}
		return object.NewInteger(n), nil
	}
	return nil, errorf("evaluator: Integer: can't convert %T", args[0])
}

// kernelFloat implements Kernel#Float(v).
func kernelFloat(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	if len(args) != 1 {
		return nil, errorf("evaluator: Float: wrong number of arguments (%d)", len(args))
	}
	switch v := args[0].(type) {
	case *object.Float:
		return v, nil
	case *object.Integer:
		return object.NewFloat(float64(v.Value)), nil
	case *object.String:
		s := strings.TrimSpace(string(v.Buf))
		s = strings.ReplaceAll(s, "_", "")
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return raiseBuiltin(env, "ArgumentError", "invalid value for Float(): \""+string(v.Buf)+"\"")
		}
		return object.NewFloat(f), nil
	}
	return nil, errorf("evaluator: Float: can't convert %T", args[0])
}

// kernelString implements Kernel#String(v) -- routes through to_s.
func kernelString(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	if len(args) != 1 {
		return nil, errorf("evaluator: String: wrong number of arguments (%d)", len(args))
	}
	return object.NewString(putsString(env, args[0])), nil
}

// kernelArray implements Kernel#Array(v) -- v if it's already an Array,
// [v] otherwise (nil becomes []).
func kernelArray(args []object.RubyObject) (object.RubyObject, error) {
	if len(args) != 1 {
		return nil, errorf("evaluator: Array: wrong number of arguments (%d)", len(args))
	}
	switch v := args[0].(type) {
	case *object.Array:
		return v, nil
	case *object.Nil:
		return object.NewArray(), nil
	}
	return object.NewArray(args[0]), nil
}

// kernelSprintf implements Kernel#sprintf / format: first arg is the
// format string, rest are the values to interpolate.
func kernelSprintf(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	if len(args) == 0 {
		return nil, errorf("evaluator: ArgumentError: sprintf needs a format string")
	}
	fmtStr, ok := stringText(env, args[0])
	if !ok {
		return nil, errorf("evaluator: sprintf: format must be String")
	}
	s, err := sprintfFormat(env, fmtStr, args[1:])
	if err != nil {
		return nil, err
	}
	return object.NewString(s), nil
}

// kernelPrintf implements Kernel#printf: same as sprintf but writes
// to stdout instead of returning a String.
func kernelPrintf(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	v, err := kernelSprintf(env, args)
	if err != nil {
		return nil, err
	}
	s := v.(*object.String)
	_, _ = env.Stdout().Write(s.Buf)
	return object.NIL, nil
}

// kernelPrint implements Kernel#print: writes each argument's to_s
// rendering with no trailing newline and no separator.
func kernelPrint(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	w := env.Stdout()
	for _, a := range args {
		_, _ = w.Write([]byte(putsString(env, a)))
	}
	return object.NIL, nil
}

// kernelP implements Kernel#p: writes each argument's inspect form
// followed by a newline. Returns the single arg, or the args slice
// boxed as an Array for multiple args.
func kernelP(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	w := env.Stdout()
	for _, a := range args {
		_, _ = w.Write([]byte(env.Inspect(a)))
		_, _ = w.Write([]byte{'\n'})
	}
	switch len(args) {
	case 0:
		return object.NIL, nil
	case 1:
		return args[0], nil
	}
	return object.NewArray(args...), nil
}

// putsString returns the Kernel#puts rendering of a value. Differs from
// inspect in two places: nil renders as the empty string (so `puts nil`
// produces just "\n"), and strings render bare (no surrounding quotes).
// For Instances with a user-defined `to_s`, dispatches to it.
func putsString(env *object.Environment, o object.RubyObject) string {
	switch v := o.(type) {
	case *object.Nil:
		return ""
	case *object.String:
		return string(v.Buf)
	case *object.FrozenString:
		return env.Strings().Get(v.ID)
	case *object.Symbol:
		return env.Symbols().Name(v.ID)
	case *object.Instance:
		if m, found := v.C.LookupMethod("to_s"); found {
			if um, ok := m.(*object.UserMethod); ok {
				res, err := invokeMethodOn(env, v, um, nil, nil)
				if err == nil {
					if s, ok := stringText(env, res); ok {
						return s
					}
				}
			}
		}
	}
	return env.Inspect(o)
}
