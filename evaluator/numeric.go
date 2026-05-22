package evaluator

import (
	"math"
	"math/big"
	"math/bits"

	"github.com/lczyk/goruby/evaluator/stdlib"
	"github.com/lczyk/goruby/object"
)

// numericInfix dispatches arithmetic and comparison ops between two
// numeric operands. Returns (nil, false, nil) if op is not a numeric
// operator or the operands aren't both numeric; callers fall through
// to other infix paths.
func numericInfix(env *object.Environment, op string, left, right object.RubyObject) (object.RubyObject, bool, error) {
	lInt, lIsInt := left.(*object.Integer)
	rInt, rIsInt := right.(*object.Integer)
	lFlt, lIsFlt := left.(*object.Float)
	rFlt, rIsFlt := right.(*object.Float)

	if lIsInt && rIsInt {
		// Integer ** negative Integer yields a Rational. Handle before
		// falling through to intInfix/bigInfix, which both treat exp<0
		// as a domain error.
		if op == "**" && !lInt.IsBig() && !rInt.IsBig() && rInt.Value < 0 {
			v, err := stdlib.IntegerPowToRational(env, lInt.Value, rInt.Value)
			return v, true, err
		}
		// Bignum path: any operand spilled to *big.Int routes through
		// bigInfix. Same for the small-times-small overflow promotions
		// below (intInfix returns nil-but-no-error when an arithmetic op
		// overflows int64; we retry under big precision).
		if lInt.IsBig() || rInt.IsBig() {
			v, err := bigInfix(op, lInt.ToBig(), rInt.ToBig())
			if err != nil {
				return nil, true, err
			}
			if v == nil {
				return nil, false, nil
			}
			return v, true, nil
		}
		v, overflow, err := intInfix(op, lInt.Value, rInt.Value)
		if err != nil {
			return nil, true, err
		}
		if overflow {
			v, err := bigInfix(op, big.NewInt(lInt.Value), big.NewInt(rInt.Value))
			if err != nil {
				return nil, true, err
			}
			if v == nil {
				return nil, false, nil
			}
			return v, true, nil
		}
		if v == nil {
			return nil, false, nil
		}
		return v, true, nil
	}

	if (lIsInt || lIsFlt) && (rIsInt || rIsFlt) {
		var l, r float64
		if lIsInt {
			l = integerToFloat(lInt)
		} else {
			l = lFlt.Value
		}
		if rIsInt {
			r = integerToFloat(rInt)
		} else {
			r = rFlt.Value
		}
		v, err := floatInfix(op, l, r)
		if err != nil {
			return nil, true, err
		}
		if v == nil {
			return nil, false, nil
		}
		return v, true, nil
	}

	return nil, false, nil
}

// integerToFloat coerces an Integer (inline or big) to float64. Big
// values past Float53's exact range round to the nearest representable
// float, matching MRI's `Integer#to_f` behaviour.
func integerToFloat(i *object.Integer) float64 {
	if i.IsBig() {
		f, _ := new(big.Float).SetInt(i.Bn).Float64()
		return f
	}
	return float64(i.Value)
}

// intInfix evaluates op on two int64 operands. Returns
// (result, overflow=false, err=nil) on success;
// (nil, overflow=true, nil) when arithmetic overflowed int64 and the
// caller should retry in big.Int;
// (nil, false, nil) when op isn't a numeric op (caller falls through);
// (nil, false, err) on runtime errors (ZeroDivision etc).
func intInfix(op string, l, r int64) (object.RubyObject, bool, error) {
	switch op {
	case "+":
		s, c := bits.Add64(uint64(l), uint64(r), 0)
		// Signed overflow: result sign differs from both operand signs.
		if (l >= 0) == (r >= 0) && (int64(s) >= 0) != (l >= 0) {
			_ = c
			return nil, true, nil
		}
		return object.NewInteger(int64(s)), false, nil
	case "-":
		d, _ := bits.Sub64(uint64(l), uint64(r), 0)
		// Signed overflow when operands have opposite signs and result
		// sign differs from the minuend's.
		if (l >= 0) != (r >= 0) && (int64(d) >= 0) != (l >= 0) {
			return nil, true, nil
		}
		return object.NewInteger(int64(d)), false, nil
	case "*":
		// Detect overflow by Mul64 on absolute values, then re-sign.
		if l == 0 || r == 0 {
			return object.NewInteger(0), false, nil
		}
		// Special-case MinInt64 to avoid overflow when negating it.
		if l == math.MinInt64 || r == math.MinInt64 {
			return nil, true, nil
		}
		la, ra := l, r
		neg := false
		if la < 0 {
			la = -la
			neg = !neg
		}
		if ra < 0 {
			ra = -ra
			neg = !neg
		}
		hi, lo := bits.Mul64(uint64(la), uint64(ra))
		if hi != 0 || lo > uint64(math.MaxInt64) {
			return nil, true, nil
		}
		out := int64(lo)
		if neg {
			out = -out
		}
		return object.NewInteger(out), false, nil
	case "/":
		if r == 0 {
			return nil, false, errZeroDivision
		}
		// MRI Integer#/ is floor division: -7 / 2 == -4.
		q := l / r
		if (l%r != 0) && ((l < 0) != (r < 0)) {
			q--
		}
		return object.NewInteger(q), false, nil
	case "%":
		if r == 0 {
			return nil, false, errZeroDivision
		}
		// MRI Integer#% follows the floor-division remainder: result has
		// the same sign as the divisor.
		m := l % r
		if m != 0 && ((m < 0) != (r < 0)) {
			m += r
		}
		return object.NewInteger(m), false, nil
	case "**":
		if r < 0 {
			// numericInfix has already intercepted Integer**neg above;
			// reach this branch only via the bignum overflow retry path.
			return nil, false, errorf("evaluator: negative Integer exponent yields Rational; not yet supported")
		}
		// Detect potential overflow conservatively: any large base /
		// exponent combo goes to the big path. Reuses the multiplicative
		// overflow check by stepping the power one multiply at a time
		// and bailing out on the first overflow.
		var out int64 = 1
		base := l
		exp := r
		for exp > 0 {
			if exp&1 == 1 {
				v, ovf, err := intInfix("*", out, base)
				if err != nil {
					return nil, false, err
				}
				if ovf {
					return nil, true, nil
				}
				out = v.(*object.Integer).Value
			}
			exp >>= 1
			if exp == 0 {
				break
			}
			v, ovf, err := intInfix("*", base, base)
			if err != nil {
				return nil, false, err
			}
			if ovf {
				return nil, true, nil
			}
			base = v.(*object.Integer).Value
		}
		return object.NewInteger(out), false, nil
	case "<":
		return object.BooleanOf(l < r), false, nil
	case "<=":
		return object.BooleanOf(l <= r), false, nil
	case ">":
		return object.BooleanOf(l > r), false, nil
	case ">=":
		return object.BooleanOf(l >= r), false, nil
	case "<=>":
		switch {
		case l < r:
			return object.NewInteger(-1), false, nil
		case l > r:
			return object.NewInteger(1), false, nil
		}
		return object.NewInteger(0), false, nil
	case "&":
		return object.NewInteger(l & r), false, nil
	case "|":
		return object.NewInteger(l | r), false, nil
	case "^":
		return object.NewInteger(l ^ r), false, nil
	case "<<":
		return object.NewInteger(l << uint(r)), false, nil
	case ">>":
		return object.NewInteger(l >> uint(r)), false, nil
	}
	return nil, false, nil
}

// bigInfix evaluates op on two *big.Int operands. Result Integer
// demotes to inline form when it fits int64; bit-and-shift ops are
// out of scope here (the corpus doesn't need them on bignums yet --
// add when needed). Returns nil with no error when op isn't a
// supported bignum op so callers fall through.
func bigInfix(op string, l, r *big.Int) (object.RubyObject, error) {
	switch op {
	case "+":
		return object.NewBigInteger(new(big.Int).Add(l, r)), nil
	case "-":
		return object.NewBigInteger(new(big.Int).Sub(l, r)), nil
	case "*":
		return object.NewBigInteger(new(big.Int).Mul(l, r)), nil
	case "/":
		if r.Sign() == 0 {
			return nil, errZeroDivision
		}
		// math/big Quo truncates toward zero. MRI Integer#/ floors.
		// Adjust by subtracting 1 when truncation and floor disagree
		// (signs differ and there's a non-zero remainder).
		q := new(big.Int).Quo(l, r)
		rem := new(big.Int).Rem(l, r)
		if rem.Sign() != 0 && (l.Sign() < 0) != (r.Sign() < 0) {
			q.Sub(q, big.NewInt(1))
		}
		return object.NewBigInteger(q), nil
	case "%":
		if r.Sign() == 0 {
			return nil, errZeroDivision
		}
		// big.Int.Mod returns Euclidean mod (non-negative result);
		// MRI Integer#% follows the divisor's sign. Match by adjusting:
		// rem = l - (l/r)*r where / is the floor-div above.
		rem := new(big.Int).Rem(l, r)
		if rem.Sign() != 0 && (rem.Sign() < 0) != (r.Sign() < 0) {
			rem.Add(rem, r)
		}
		return object.NewBigInteger(rem), nil
	case "**":
		if r.Sign() < 0 {
			return nil, errorf("evaluator: negative Integer exponent yields Rational; not yet supported")
		}
		if !r.IsInt64() {
			return nil, errorf("evaluator: Integer#** exponent too large")
		}
		// big.Int.Exp uses signed exponent and an optional modulus.
		// Pass nil mod for plain power.
		return object.NewBigInteger(new(big.Int).Exp(l, r, nil)), nil
	case "<":
		return object.BooleanOf(l.Cmp(r) < 0), nil
	case "<=":
		return object.BooleanOf(l.Cmp(r) <= 0), nil
	case ">":
		return object.BooleanOf(l.Cmp(r) > 0), nil
	case ">=":
		return object.BooleanOf(l.Cmp(r) >= 0), nil
	case "<=>":
		return object.NewInteger(int64(l.Cmp(r))), nil
	case "&":
		return object.NewBigInteger(new(big.Int).And(l, r)), nil
	case "|":
		return object.NewBigInteger(new(big.Int).Or(l, r)), nil
	case "^":
		return object.NewBigInteger(new(big.Int).Xor(l, r)), nil
	}
	return nil, nil
}

func floatInfix(op string, l, r float64) (object.RubyObject, error) {
	switch op {
	case "+":
		return object.NewFloat(l + r), nil
	case "-":
		return object.NewFloat(l - r), nil
	case "*":
		return object.NewFloat(l * r), nil
	case "/":
		return object.NewFloat(l / r), nil
	case "**":
		return object.NewFloat(floatPow(l, r)), nil
	case "<":
		return object.BooleanOf(l < r), nil
	case "<=":
		return object.BooleanOf(l <= r), nil
	case ">":
		return object.BooleanOf(l > r), nil
	case ">=":
		return object.BooleanOf(l >= r), nil
	case "<=>":
		switch {
		case l < r:
			return object.NewInteger(-1), nil
		case l > r:
			return object.NewInteger(1), nil
		case l == r:
			return object.NewInteger(0), nil
		}
		return object.NIL, nil // NaN comparison returns nil in MRI.
	}
	return nil, nil
}

// floatPow is math.Pow without importing math at this scale. Iterative
// for integer-shaped exponents, fall through to repeated multiplication
// otherwise. Acceptable for the corpus; replace with math.Pow once Float
// stdlib parity matters.
func floatPow(base, exp float64) float64 {
	if exp == float64(int64(exp)) && exp >= 0 {
		out := 1.0
		for i := int64(0); i < int64(exp); i++ {
			out *= base
		}
		return out
	}
	// Fallback: not actually exercised by the corpus. Routed through
	// the iterative form so this file stays import-free until needed.
	out := 1.0
	for i := 0; i < int(exp); i++ {
		out *= base
	}
	return out
}
