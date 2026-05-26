package evaluator

import (
	"strconv"

	"github.com/lczyk/goruby/object"
)

// Builtin Integer methods, registered on object.IntegerClass at init.
// Migrated out of callMethod's hand-rolled switch so dispatch goes
// through Send. Legacy switch arms for the same names still exist as
// dead code (unreachable now that Send catches them first); they get
// removed once every type has migrated.

func init() {
	c := object.IntegerClass
	add := func(name string, fn func(env *object.Environment, r *object.Integer, args []object.RubyObject) (object.RubyObject, error)) {
		c.AddMethod(name, &object.BuiltinMethod{
			Name: name,
			Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
				return fn(env, recv.(*object.Integer), args)
			},
		})
	}

	add("even?", func(env *object.Environment, r *object.Integer, args []object.RubyObject) (object.RubyObject, error) {
		return object.BooleanOf(r.Value%2 == 0), nil
	})
	add("odd?", func(env *object.Environment, r *object.Integer, args []object.RubyObject) (object.RubyObject, error) {
		return object.BooleanOf(r.Value%2 != 0), nil
	})
	add("zero?", func(env *object.Environment, r *object.Integer, args []object.RubyObject) (object.RubyObject, error) {
		return object.BooleanOf(r.Value == 0), nil
	})
	add("nonzero?", func(env *object.Environment, r *object.Integer, args []object.RubyObject) (object.RubyObject, error) {
		if r.Value == 0 {
			return object.NIL, nil
		}
		return r, nil
	})
	add("pow", func(env *object.Environment, r *object.Integer, args []object.RubyObject) (object.RubyObject, error) {
		if len(args) < 1 || len(args) > 2 {
			return raiseBuiltin(env, "ArgumentError", "wrong number of arguments (given 0, expected 1..2)")
		}
		exp, ok := args[0].(*object.Integer)
		if !ok {
			return nil, errorf("evaluator: Integer#pow exponent must be Integer")
		}
		if len(args) == 1 {
			if exp.Value < 0 {
				return nil, errorf("evaluator: Integer#pow with negative exponent yields Rational; not yet supported")
			}
			out := int64(1)
			base := r.Value
			e := exp.Value
			for e > 0 {
				if e&1 == 1 {
					out *= base
				}
				base *= base
				e >>= 1
			}
			return object.NewInteger(out), nil
		}
		// Two-arg form: modular exponentiation. (self ** exp) mod mod
		// without materialising the intermediate huge integer.
		mod, ok := args[1].(*object.Integer)
		if !ok {
			return nil, errorf("evaluator: Integer#pow modulus must be Integer")
		}
		if mod.Value == 0 {
			return raiseBuiltin(env, "ZeroDivisionError", "divided by 0")
		}
		if exp.Value < 0 {
			return nil, errorf("evaluator: Integer#pow with negative exponent + modulus not yet supported")
		}
		m := mod.Value
		result := int64(1) % m
		base := r.Value % m
		if base < 0 {
			base += m
		}
		e := exp.Value
		for e > 0 {
			if e&1 == 1 {
				result = (result * base) % m
			}
			base = (base * base) % m
			e >>= 1
		}
		if result < 0 {
			result += m
		}
		return object.NewInteger(result), nil
	})
	add("negative?", func(env *object.Environment, r *object.Integer, args []object.RubyObject) (object.RubyObject, error) {
		return object.BooleanOf(r.Value < 0), nil
	})
	add("positive?", func(env *object.Environment, r *object.Integer, args []object.RubyObject) (object.RubyObject, error) {
		return object.BooleanOf(r.Value > 0), nil
	})
	add("succ", func(env *object.Environment, r *object.Integer, args []object.RubyObject) (object.RubyObject, error) {
		return object.NewInteger(r.Value + 1), nil
	})
	add("next", func(env *object.Environment, r *object.Integer, args []object.RubyObject) (object.RubyObject, error) {
		return object.NewInteger(r.Value + 1), nil
	})
	add("pred", func(env *object.Environment, r *object.Integer, args []object.RubyObject) (object.RubyObject, error) {
		return object.NewInteger(r.Value - 1), nil
	})
	add("to_i", func(env *object.Environment, r *object.Integer, args []object.RubyObject) (object.RubyObject, error) {
		return r, nil
	})
	add("integer?", func(env *object.Environment, r *object.Integer, args []object.RubyObject) (object.RubyObject, error) {
		return object.TRUE, nil
	})
	add("real?", func(env *object.Environment, r *object.Integer, args []object.RubyObject) (object.RubyObject, error) {
		return object.TRUE, nil
	})
	add("finite?", func(env *object.Environment, r *object.Integer, args []object.RubyObject) (object.RubyObject, error) {
		return object.TRUE, nil
	})
	add("infinite?", func(env *object.Environment, r *object.Integer, args []object.RubyObject) (object.RubyObject, error) {
		return object.NIL, nil
	})
	add("to_int", func(env *object.Environment, r *object.Integer, args []object.RubyObject) (object.RubyObject, error) {
		return r, nil
	})
	add("to_f", func(env *object.Environment, r *object.Integer, args []object.RubyObject) (object.RubyObject, error) {
		return object.NewFloat(float64(r.Value)), nil
	})
	add("abs", func(env *object.Environment, r *object.Integer, args []object.RubyObject) (object.RubyObject, error) {
		if r.Value < 0 {
			return object.NewInteger(-r.Value), nil
		}
		return r, nil
	})
	add("chr", func(env *object.Environment, r *object.Integer, args []object.RubyObject) (object.RubyObject, error) {
		// MRI 2.x: Integer#chr without an encoding arg returns a single
		// byte for values 0..255 (encoded as ASCII-8BIT), raising
		// RangeError outside that range. We don't track string encodings
		// separately yet, but we DO need the raw-byte behaviour: lots of
		// ruby code (stackcats, anything writing binary) relies on
		// `(n % 256).chr` producing exactly one byte. The naive
		// `string(rune(n))` UTF-8-encodes 128..255 as two bytes, breaking
		// that contract.
		if r.IsBig() {
			return raiseBuiltin(env, "RangeError", r.Bn.String()+" out of char range")
		}
		if r.Value < 0 || r.Value > 255 {
			return raiseBuiltin(env, "RangeError", strconv.FormatInt(r.Value, 10)+" out of char range")
		}
		return object.NewStringFromBytes([]byte{byte(r.Value)}), nil
	})
	add("to_s", func(env *object.Environment, r *object.Integer, args []object.RubyObject) (object.RubyObject, error) {
		base := 10
		if len(args) == 1 {
			b, ok := args[0].(*object.Integer)
			if !ok {
				return nil, errorf("evaluator: Integer#to_s base must be Integer")
			}
			base = int(b.Value)
			if base < 2 || base > 36 {
				return nil, errorf("evaluator: ArgumentError: invalid radix %d", base)
			}
		}
		if r.IsBig() {
			return object.NewString(r.Bn.Text(base)), nil
		}
		return object.NewString(strconv.FormatInt(r.Value, base)), nil
	})
	add("to_r", func(env *object.Environment, r *object.Integer, args []object.RubyObject) (object.RubyObject, error) {
		return object.NewFloat(float64(r.Value)), nil
	})
	add("clamp", func(env *object.Environment, r *object.Integer, args []object.RubyObject) (object.RubyObject, error) {
		var lo, hi int64
		switch len(args) {
		case 1:
			rng, ok := args[0].(*object.Range)
			if !ok {
				return nil, errorf("evaluator: Integer#clamp expects Range or 2 ints")
			}
			blo, bhi, ok := rangeIntegerBounds(rng)
			if !ok {
				return nil, errorf("evaluator: Integer#clamp Range needs Integer bounds")
			}
			lo = blo
			hi = bhi
			if rng.Exclusive {
				hi--
			}
		case 2:
			lov, ok1 := args[0].(*object.Integer)
			hiv, ok2 := args[1].(*object.Integer)
			if !ok1 || !ok2 {
				return nil, errorf("evaluator: Integer#clamp needs Integer bounds")
			}
			lo = lov.Value
			hi = hiv.Value
		default:
			return nil, errorf("evaluator: Integer#clamp expects 1..2 args, got %d", len(args))
		}
		v := r.Value
		switch {
		case v < lo:
			v = lo
		case v > hi:
			v = hi
		}
		return object.NewInteger(v), nil
	})
	add("between?", func(env *object.Environment, r *object.Integer, args []object.RubyObject) (object.RubyObject, error) {
		if len(args) != 2 {
			return nil, errorf("evaluator: Integer#between? expects 2 args, got %d", len(args))
		}
		lo, ok1 := args[0].(*object.Integer)
		hi, ok2 := args[1].(*object.Integer)
		if !ok1 || !ok2 {
			return nil, errorf("evaluator: Integer#between? needs Integer bounds")
		}
		return object.BooleanOf(r.Value >= lo.Value && r.Value <= hi.Value), nil
	})
	add("divmod", func(env *object.Environment, r *object.Integer, args []object.RubyObject) (object.RubyObject, error) {
		if len(args) != 1 {
			return nil, errorf("evaluator: Integer#divmod expects 1 arg")
		}
		d, ok := args[0].(*object.Integer)
		if !ok {
			return nil, errorf("evaluator: Integer#divmod needs Integer")
		}
		if d.Value == 0 {
			return raiseBuiltin(env, "ZeroDivisionError", "divided by 0")
		}
		q := r.Value / d.Value
		if r.Value%d.Value != 0 && (r.Value < 0) != (d.Value < 0) {
			q--
		}
		m := r.Value - q*d.Value
		return object.NewArray(object.NewInteger(q), object.NewInteger(m)), nil
	})
	mod := func(env *object.Environment, r *object.Integer, args []object.RubyObject) (object.RubyObject, error) {
		if len(args) != 1 {
			return nil, errorf("evaluator: Integer#modulo expects 1 arg")
		}
		d, ok := args[0].(*object.Integer)
		if !ok {
			return nil, errorf("evaluator: Integer#modulo needs Integer")
		}
		if d.Value == 0 {
			return raiseBuiltin(env, "ZeroDivisionError", "divided by 0")
		}
		m := r.Value % d.Value
		if m != 0 && ((m < 0) != (d.Value < 0)) {
			m += d.Value
		}
		return object.NewInteger(m), nil
	}
	add("modulo", mod)
	add("%", mod)
	add("fdiv", func(env *object.Environment, r *object.Integer, args []object.RubyObject) (object.RubyObject, error) {
		if len(args) != 1 {
			return nil, errorf("evaluator: Integer#fdiv expects 1 arg")
		}
		df, err := toFloatValue(args[0])
		if err != nil {
			return nil, err
		}
		return object.NewFloat(float64(r.Value) / df), nil
	})
	add("remainder", func(env *object.Environment, r *object.Integer, args []object.RubyObject) (object.RubyObject, error) {
		if len(args) != 1 {
			return nil, errorf("evaluator: Integer#remainder expects 1 arg")
		}
		d, ok := args[0].(*object.Integer)
		if !ok {
			return nil, errorf("evaluator: Integer#remainder needs Integer")
		}
		if d.Value == 0 {
			return raiseBuiltin(env, "ZeroDivisionError", "divided by 0")
		}
		return object.NewInteger(r.Value % d.Value), nil
	})
	gcdInt := func(a, b int64) int64 {
		if a < 0 {
			a = -a
		}
		if b < 0 {
			b = -b
		}
		for b != 0 {
			a, b = b, a%b
		}
		return a
	}
	add("gcd", func(env *object.Environment, r *object.Integer, args []object.RubyObject) (object.RubyObject, error) {
		if len(args) != 1 {
			return nil, errorf("evaluator: Integer#gcd expects 1 arg")
		}
		d, ok := args[0].(*object.Integer)
		if !ok {
			return nil, errorf("evaluator: Integer#gcd needs Integer")
		}
		return object.NewInteger(gcdInt(r.Value, d.Value)), nil
	})
	add("lcm", func(env *object.Environment, r *object.Integer, args []object.RubyObject) (object.RubyObject, error) {
		if len(args) != 1 {
			return nil, errorf("evaluator: Integer#lcm expects 1 arg")
		}
		d, ok := args[0].(*object.Integer)
		if !ok {
			return nil, errorf("evaluator: Integer#lcm needs Integer")
		}
		if r.Value == 0 || d.Value == 0 {
			return object.NewInteger(0), nil
		}
		g := gcdInt(r.Value, d.Value)
		a, b := r.Value, d.Value
		if a < 0 {
			a = -a
		}
		if b < 0 {
			b = -b
		}
		return object.NewInteger(a / g * b), nil
	})
	add("bit_length", func(env *object.Environment, r *object.Integer, args []object.RubyObject) (object.RubyObject, error) {
		v := r.Value
		if v < 0 {
			v = ^v
		}
		n := int64(0)
		for v > 0 {
			n++
			v >>= 1
		}
		return object.NewInteger(n), nil
	})
	// Operator methods: +, -, *, /, %, **, <, <=, >, >=, <=>, ==.
	// Mirrors the infix path so `n.send(:<, m)` /
	// `n.__send__(:<, m)` resolves the same way `n < m` does. Minitest's
	// assert_operator depends on this.
	registerNumOp := func(name string) {
		add(name, func(env *object.Environment, r *object.Integer, args []object.RubyObject) (object.RubyObject, error) {
			if len(args) != 1 {
				return nil, errorf("evaluator: Integer#%s expects 1 arg, got %d", name, len(args))
			}
			v, handled, err := numericInfix(env, name, r, args[0])
			if err != nil {
				return nil, err
			}
			if !handled {
				// MRI raises TypeError / ArgumentError for mismatched
				// operand types depending on op. == returns false
				// rather than raising. <=> returns nil. Others raise.
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
				return raiseBuiltin(env, "ArgumentError", "comparison of Integer with "+other+" failed")
			}
			return v, nil
		})
	}
	for _, op := range []string{"+", "-", "*", "/", "%", "**", "<", "<=", ">", ">=", "<=>", "=="} {
		registerNumOp(op)
	}

	add("digits", func(env *object.Environment, r *object.Integer, args []object.RubyObject) (object.RubyObject, error) {
		base := int64(10)
		if len(args) == 1 {
			b, ok := args[0].(*object.Integer)
			if !ok {
				return nil, errorf("evaluator: TypeError: Integer#digits base must be Integer")
			}
			base = b.Value
		}
		if base < 2 {
			return nil, errorf("evaluator: ArgumentError: invalid digits base %d", base)
		}
		if r.Value < 0 {
			return nil, errorf("evaluator: Math::DomainError: out of domain")
		}
		if r.Value == 0 {
			return object.NewArray(object.NewInteger(0)), nil
		}
		out := []object.RubyObject{}
		n := r.Value
		for n > 0 {
			out = append(out, object.NewInteger(n%base))
			n /= base
		}
		return object.NewArray(out...), nil
	})
	// Integer.sqrt(n) class method -- integer square root.
	c.ClassMethods["sqrt"] = &object.BuiltinMethod{Name: "sqrt", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		if len(args) != 1 {
			return nil, errorf("evaluator: Integer.sqrt expects 1 arg, got %d", len(args))
		}
		n, ok := args[0].(*object.Integer)
		if !ok {
			return nil, errorf("evaluator: Integer.sqrt arg must be Integer, got %T", args[0])
		}
		if n.Value < 0 {
			return raiseBuiltin(env, "ArgumentError", "Integer.sqrt(): argument out of domain")
		}
		v := n.Value
		// Newton's method on int64.
		if v == 0 {
			return object.NewInteger(0), nil
		}
		x := v
		y := (x + 1) / 2
		for y < x {
			x = y
			y = (x + v/x) / 2
		}
		return object.NewInteger(x), nil
	}}
}
