package stdlib

import (
	"math"

	"github.com/lczyk/goruby/evaluator/builtinapi"
	"github.com/lczyk/goruby/object"
)

// BootstrapMathModule installs the Math module with the common
// constants and methods used by the corpus / tests.
func BootstrapMathModule(env *object.Environment) *object.Class {
	if existing, ok := env.Get("Math"); ok {
		if c, ok := existing.(*object.Class); ok {
			return c
		}
	}
	m := object.NewClass("Math", nil)
	m.IsModule = true
	m.Constants["PI"] = object.NewFloat(math.Pi)
	m.Constants["E"] = object.NewFloat(math.E)
	for _, fn := range []struct {
		name string
		f    func(float64) float64
	}{
		{"sqrt", math.Sqrt},
		{"sin", math.Sin},
		{"cos", math.Cos},
		{"tan", math.Tan},
		{"log", math.Log},
		{"log2", math.Log2},
		{"log10", math.Log10},
		{"exp", math.Exp},
		{"atan", math.Atan},
		{"floor", math.Floor},
		{"ceil", math.Ceil},
	} {
		fn := fn
		m.ClassMethods[fn.name] = &object.UserMethod{Name: fn.name, Body: builtinapi.NativeFn{Fn: wrapMath1(fn.name, fn.f)}}
	}
	for _, fn := range []struct {
		name string
		f    func(float64, float64) float64
	}{
		{"hypot", math.Hypot},
		{"atan2", math.Atan2},
		{"pow", math.Pow},
	} {
		fn := fn
		m.ClassMethods[fn.name] = &object.UserMethod{Name: fn.name, Body: builtinapi.NativeFn{Fn: wrapMath2(fn.name, fn.f)}}
	}
	env.SetGlobal("Math", m)
	return m
}

func wrapMath1(name string, f func(float64) float64) func(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	return func(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
		if len(args) != 1 {
			return nil, builtinapi.Errorf("evaluator: Math.%s: wrong number of arguments (given %d, expected 1)", name, len(args))
		}
		v, ok := toFloatArg(args[0])
		if !ok {
			return nil, builtinapi.Errorf("evaluator: Math.%s: expected Numeric, got %T", name, args[0])
		}
		return object.NewFloat(f(v)), nil
	}
}

func wrapMath2(name string, f func(float64, float64) float64) func(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	return func(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
		if len(args) != 2 {
			return nil, builtinapi.Errorf("evaluator: Math.%s: wrong number of arguments (given %d, expected 2)", name, len(args))
		}
		a, ok := toFloatArg(args[0])
		if !ok {
			return nil, builtinapi.Errorf("evaluator: Math.%s: expected Numeric arg 1, got %T", name, args[0])
		}
		b, ok := toFloatArg(args[1])
		if !ok {
			return nil, builtinapi.Errorf("evaluator: Math.%s: expected Numeric arg 2, got %T", name, args[1])
		}
		return object.NewFloat(f(a, b)), nil
	}
}

func toFloatArg(o object.RubyObject) (float64, bool) {
	switch v := o.(type) {
	case *object.Integer:
		return float64(v.Value), true
	case *object.Float:
		return v.Value, true
	}
	return 0, false
}
