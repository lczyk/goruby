package stdlib

import (
	"math"
	"strconv"

	"github.com/lczyk/goruby/evaluator/builtinapi"
	"github.com/lczyk/goruby/object"
)

// mathFrexp aliases math.Frexp -- kept local so the conversion site
// reads as a single decomposition step.
func mathFrexp(v float64) (frac float64, exp int) { return math.Frexp(v) }

// BootstrapRationalClass installs a minimal Rational class. Instances
// carry @num and @den (both Integer; @den always positive, GCD-reduced).
// Construction goes through Kernel#Rational(n, d) or, more commonly in
// the corpus, by raising an Integer to a negative Integer exponent.
func BootstrapRationalClass(env *object.Environment) *object.Class {
	if existing, ok := env.Get("Rational"); ok {
		if c, ok := existing.(*object.Class); ok {
			return c
		}
	}
	c := object.NewClass("Rational", object.NumericClass)
	c.ClassMethods["new"] = &object.UserMethod{Name: "new", Body: builtinapi.NativeFn{Fn: rationalNew(c)}}
	c.Methods["numerator"] = &object.BuiltinMethod{Name: "numerator", Fn: rationalAccessor("@num")}
	c.Methods["denominator"] = &object.BuiltinMethod{Name: "denominator", Fn: rationalAccessor("@den")}
	c.Methods["to_s"] = &object.BuiltinMethod{Name: "to_s", Fn: rationalToS}
	c.Methods["inspect"] = &object.BuiltinMethod{Name: "inspect", Fn: rationalInspect}
	env.SetGlobal("Rational", c)
	return c
}

func rationalNew(c *object.Class) func(*object.Environment, []object.RubyObject) (object.RubyObject, error) {
	return func(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
		num, den := int64(0), int64(1)
		if len(args) >= 1 {
			n, ok := args[0].(*object.Integer)
			if !ok {
				return nil, builtinapi.Errorf("evaluator: Rational.new: numerator must be Integer, got %T", args[0])
			}
			num = n.Value
		}
		if len(args) >= 2 {
			d, ok := args[1].(*object.Integer)
			if !ok {
				return nil, builtinapi.Errorf("evaluator: Rational.new: denominator must be Integer, got %T", args[1])
			}
			den = d.Value
			if den == 0 {
				return nil, builtinapi.Errorf("evaluator: ZeroDivisionError: divided by 0")
			}
		}
		return newRational(c, num, den), nil
	}
}

// newRational builds a reduced Rational instance with the canonical
// sign convention (denominator non-negative).
func newRational(c *object.Class, num, den int64) *object.Instance {
	if den < 0 {
		num, den = -num, -den
	}
	g := gcdInt(absInt(num), den)
	if g > 1 {
		num /= g
		den /= g
	}
	return &object.Instance{
		C: c,
		Ivars: map[string]object.RubyObject{
			"@num": object.NewInteger(num),
			"@den": object.NewInteger(den),
		},
	}
}

func gcdInt(a, b int64) int64 {
	for b != 0 {
		a, b = b, a%b
	}
	if a == 0 {
		return 1
	}
	return a
}

func absInt(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

func rationalParts(recv object.RubyObject) (num, den int64, ok bool) {
	inst, ok := recv.(*object.Instance)
	if !ok {
		return 0, 0, false
	}
	if n, ok := inst.Ivars["@num"].(*object.Integer); ok {
		num = n.Value
	}
	if d, ok := inst.Ivars["@den"].(*object.Integer); ok {
		den = d.Value
	}
	return num, den, true
}

func rationalAccessor(ivar string) func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
	return func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		inst, ok := recv.(*object.Instance)
		if !ok {
			return nil, builtinapi.Errorf("evaluator: Rational accessor on non-Rational %T", recv)
		}
		if v, ok := inst.Ivars[ivar]; ok {
			return v, nil
		}
		return object.NewInteger(0), nil
	}
}

func rationalToS(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
	num, den, ok := rationalParts(recv)
	if !ok {
		return nil, builtinapi.Errorf("evaluator: Rational#to_s on non-Rational %T", recv)
	}
	return object.NewString(strconv.FormatInt(num, 10) + "/" + strconv.FormatInt(den, 10)), nil
}

func rationalInspect(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
	num, den, ok := rationalParts(recv)
	if !ok {
		return nil, builtinapi.Errorf("evaluator: Rational#inspect on non-Rational %T", recv)
	}
	return object.NewString("(" + strconv.FormatInt(num, 10) + "/" + strconv.FormatInt(den, 10) + ")"), nil
}

// FloatToRational converts v into a Rational using the IEEE 754
// binary mantissa decomposition: x = frac * 2^exp where frac is in
// [0.5, 1). Scales frac by 2^53 (full mantissa precision) so the
// fraction is an integer, then reduces by GCD. NaN / Inf raise
// FloatDomainError. Caller must guarantee the Rational class is
// bootstrapped; this looks it up from env.
func FloatToRational(env *object.Environment, v float64) (object.RubyObject, error) {
	if v != v {
		return nil, builtinapi.Errorf("evaluator: FloatDomainError: NaN")
	}
	if v > 1e308 || v < -1e308 {
		return nil, builtinapi.Errorf("evaluator: FloatDomainError: Infinity")
	}
	rcls, _ := env.Get("Rational")
	c, _ := rcls.(*object.Class)
	if c == nil {
		c = BootstrapRationalClass(env)
	}
	if v == 0 {
		return newRational(c, 0, 1), nil
	}
	sign := int64(1)
	if v < 0 {
		sign = -1
		v = -v
	}
	frac, exp := mathFrexp(v)
	// x = frac * 2^exp, frac in [0.5, 1). Scale: num = frac * 2^53;
	// denom = 2^(53 - exp). Both fit in int64 for typical inputs.
	num := int64(frac * (1 << 53))
	shift := int64(53 - exp)
	var denom int64 = 1
	if shift >= 0 {
		if shift >= 63 {
			// Underflow guard -- denom would overflow int64.
			return nil, builtinapi.Errorf("evaluator: Float#to_r value out of representable Rational range")
		}
		denom = int64(1) << shift
	} else {
		// shift negative -> denom = 1, num scaled up.
		s := -shift
		if s >= 63 {
			return nil, builtinapi.Errorf("evaluator: Float#to_r value out of representable Rational range")
		}
		num <<= uint(s)
	}
	return newRational(c, sign*num, denom), nil
}

// IntegerPowToRational handles the negative-exponent case for
// Integer**Integer: base**(-n) becomes Rational(sign, base**n) when
// base is non-zero. Caller already validated exp < 0 and that exp's
// magnitude fits in int64.
func IntegerPowToRational(env *object.Environment, base, exp int64) (object.RubyObject, error) {
	if base == 0 {
		return nil, builtinapi.Errorf("evaluator: ZeroDivisionError: divided by 0")
	}
	rcls, _ := env.Get("Rational")
	c, _ := rcls.(*object.Class)
	if c == nil {
		c = BootstrapRationalClass(env)
	}
	absExp := -exp
	var denom int64 = 1
	b := base
	for i := int64(0); i < absExp; i++ {
		denom *= b
	}
	num := int64(1)
	if denom < 0 {
		num = -1
		denom = -denom
	}
	return newRational(c, num, denom), nil
}
