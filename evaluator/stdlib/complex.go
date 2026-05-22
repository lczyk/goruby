package stdlib

import (
	"math"

	"github.com/lczyk/goruby/evaluator/builtinapi"
	"github.com/lczyk/goruby/object"
)

// BootstrapComplexClass installs ruby's Complex class. Instances carry
// real and imaginary parts as float64 on @real / @imag ivars; class
// methods deliver arithmetic + inspect + abs. Construction goes through
// Kernel#Complex(re, im) or Complex.new(re, im).
func BootstrapComplexClass(env *object.Environment) *object.Class {
	if existing, ok := env.Get("Complex"); ok {
		if c, ok := existing.(*object.Class); ok {
			return c
		}
	}
	c := object.NewClass("Complex", object.NumericClass)
	c.ClassMethods["new"] = &object.UserMethod{Name: "new", Body: builtinapi.NativeFn{Fn: complexNew(c)}}
	c.Methods["real"] = &object.BuiltinMethod{Name: "real", Fn: complexReal}
	c.Methods["imaginary"] = &object.BuiltinMethod{Name: "imaginary", Fn: complexImag}
	c.Methods["imag"] = c.Methods["imaginary"]
	c.Methods["+"] = &object.BuiltinMethod{Name: "+", Fn: complexBinop(c, '+')}
	c.Methods["-"] = &object.BuiltinMethod{Name: "-", Fn: complexBinop(c, '-')}
	c.Methods["*"] = &object.BuiltinMethod{Name: "*", Fn: complexBinop(c, '*')}
	c.Methods["abs"] = &object.BuiltinMethod{Name: "abs", Fn: complexAbs}
	c.Methods["to_s"] = &object.BuiltinMethod{Name: "to_s", Fn: complexToS}
	c.Methods["inspect"] = &object.BuiltinMethod{Name: "inspect", Fn: complexInspect}
	env.SetGlobal("Complex", c)
	return c
}

func complexNew(c *object.Class) func(*object.Environment, []object.RubyObject) (object.RubyObject, error) {
	return func(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
		re, im := 0.0, 0.0
		if len(args) >= 1 {
			f, ok := numericToFloat(args[0])
			if !ok {
				return nil, builtinapi.Errorf("evaluator: Complex.new: expected Numeric for real part, got %T", args[0])
			}
			re = f
		}
		if len(args) >= 2 {
			f, ok := numericToFloat(args[1])
			if !ok {
				return nil, builtinapi.Errorf("evaluator: Complex.new: expected Numeric for imag part, got %T", args[1])
			}
			im = f
		}
		return &object.Instance{
			C: c,
			Ivars: map[string]object.RubyObject{
				"@real": object.NewFloat(re),
				"@imag": object.NewFloat(im),
			},
		}, nil
	}
}

func numericToFloat(o object.RubyObject) (float64, bool) {
	switch v := o.(type) {
	case *object.Integer:
		return float64(v.Value), true
	case *object.Float:
		return v.Value, true
	}
	return 0, false
}

func complexParts(recv object.RubyObject) (re, im float64, ok bool) {
	inst, ok := recv.(*object.Instance)
	if !ok {
		return 0, 0, false
	}
	if f, ok := inst.Ivars["@real"].(*object.Float); ok {
		re = f.Value
	}
	if f, ok := inst.Ivars["@imag"].(*object.Float); ok {
		im = f.Value
	}
	return re, im, true
}

func complexReal(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
	re, _, ok := complexParts(recv)
	if !ok {
		return nil, builtinapi.Errorf("evaluator: Complex#real on non-Complex %T", recv)
	}
	return numericFloatOrInt(re), nil
}

func complexImag(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
	_, im, ok := complexParts(recv)
	if !ok {
		return nil, builtinapi.Errorf("evaluator: Complex#imag on non-Complex %T", recv)
	}
	return numericFloatOrInt(im), nil
}

// numericFloatOrInt collapses a whole-number float back to Integer so
// Complex(1,2).real prints as 1, not 1.0 -- MRI keeps integer-valued
// parts as Integer.
func numericFloatOrInt(f float64) object.RubyObject {
	if math.Trunc(f) == f && !math.IsInf(f, 0) {
		return object.NewInteger(int64(f))
	}
	return object.NewFloat(f)
}

func complexBinop(c *object.Class, op byte) func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
	return func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		if len(args) != 1 {
			return nil, builtinapi.Errorf("evaluator: Complex#%c needs 1 arg", op)
		}
		ar, ai, ok := complexParts(recv)
		if !ok {
			return nil, builtinapi.Errorf("evaluator: Complex#%c on non-Complex %T", op, recv)
		}
		var br, bi float64
		switch o := args[0].(type) {
		case *object.Instance:
			if o.C == c {
				br, bi, _ = complexParts(o)
			} else {
				return nil, builtinapi.Errorf("evaluator: Complex#%c: incompatible operand %T", op, args[0])
			}
		default:
			if f, ok := numericToFloat(o); ok {
				br = f
			} else {
				return nil, builtinapi.Errorf("evaluator: Complex#%c: incompatible operand %T", op, args[0])
			}
		}
		var rr, ri float64
		switch op {
		case '+':
			rr, ri = ar+br, ai+bi
		case '-':
			rr, ri = ar-br, ai-bi
		case '*':
			rr, ri = ar*br-ai*bi, ar*bi+ai*br
		}
		return &object.Instance{
			C: c,
			Ivars: map[string]object.RubyObject{
				"@real": object.NewFloat(rr),
				"@imag": object.NewFloat(ri),
			},
		}, nil
	}
}

func complexAbs(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
	re, im, ok := complexParts(recv)
	if !ok {
		return nil, builtinapi.Errorf("evaluator: Complex#abs on non-Complex %T", recv)
	}
	return object.NewFloat(math.Hypot(re, im)), nil
}

// complexFormat returns "a+bi" / "a-bi" with parts rendered as ruby
// would: integer-valued floats as Integers, otherwise as Floats.
// parens=true wraps when the real part is non-zero negative (MRI's
// inspect always parenthesises; to_s never does).
func complexFormat(re, im float64, parens bool) string {
	rs := numericFloatOrInt(re).Inspect()
	ims := numericFloatOrInt(math.Abs(im)).Inspect()
	sign := "+"
	if im < 0 || (im == 0 && math.Signbit(im)) {
		sign = "-"
	}
	body := rs + sign + ims + "i"
	if parens {
		return "(" + body + ")"
	}
	return body
}

func complexToS(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
	re, im, ok := complexParts(recv)
	if !ok {
		return nil, builtinapi.Errorf("evaluator: Complex#to_s on non-Complex %T", recv)
	}
	return object.NewString(complexFormat(re, im, false)), nil
}

func complexInspect(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
	re, im, ok := complexParts(recv)
	if !ok {
		return nil, builtinapi.Errorf("evaluator: Complex#inspect on non-Complex %T", recv)
	}
	return object.NewString(complexFormat(re, im, true)), nil
}

// KernelComplex implements Kernel#Complex(real, imag=0) construction.
func KernelComplex(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	c, _ := env.Get("Complex")
	cls, ok := c.(*object.Class)
	if !ok {
		return nil, builtinapi.Errorf("evaluator: Complex class missing")
	}
	return complexNew(cls)(env, args)
}
