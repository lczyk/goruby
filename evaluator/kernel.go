package evaluator

import (
	"github.com/lczyk/goruby/object"
)

// callKernel dispatches Kernel-level (implicit-self) calls. Until a full
// method registry exists, the literals corpus only needs `puts` and `p`.
func callKernel(env *object.Environment, name string, args []object.RubyObject) (object.RubyObject, error) {
	switch name {
	case "puts":
		return kernelPuts(env, args)
	case "p":
		return kernelP(env, args)
	}
	return nil, errorf("evaluator: NoMethodError: undefined method `%s' for main:Object", name)
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
	}
	return env.Inspect(o)
}
