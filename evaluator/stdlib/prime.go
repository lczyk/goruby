package stdlib

import (
	"math/big"

	"github.com/lczyk/goruby/evaluator/builtinapi"
	"github.com/lczyk/goruby/object"
)

// BootstrapPrimeModule installs a minimal stdlib `Prime` module.
// MRI's Prime is a full enumerator-backed module; we cover the two
// class methods alice exercises directly:
//
//   - Prime.prime_division(n) -> Array<[prime, exponent]>
//   - Prime.int_from_prime_division(factors) -> Integer (recompose)
//
// Implementation is trial division; fine for the small-integer
// territory esolang interpreters move through. Bignum inputs aren't
// rejected -- the algorithm just gets slow.
//
// require 'prime' still routes through Kernel#require's stdlib
// allowlist; this bootstrap runs on first eval so the constant is
// available regardless of whether `require` was called.
func BootstrapPrimeModule(env *object.Environment) *object.Class {
	if existing, ok := env.Get("Prime"); ok {
		if c, ok := existing.(*object.Class); ok {
			return c
		}
	}
	c := object.NewClass("Prime", nil)
	c.IsModule = true
	c.ClassMethods["prime_division"] = &object.UserMethod{
		Name: "prime_division",
		Body: builtinapi.NativeFn{Fn: primeDivision},
	}
	c.ClassMethods["int_from_prime_division"] = &object.UserMethod{
		Name: "int_from_prime_division",
		Body: builtinapi.NativeFn{Fn: primeIntFromPrimeDivision},
	}
	env.SetGlobal("Prime", c)
	return c
}

// primeDivision factors n into [prime, exponent] pairs. Matches MRI:
// negative n raises ZeroDivisionError on n==0 and otherwise produces
// [[-1,1], ...] for n<0 (sign carried as a -1 factor).
func primeDivision(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	if len(args) != 1 {
		return nil, builtinapi.Errorf("evaluator: Prime.prime_division: wrong number of arguments (given %d, expected 1)", len(args))
	}
	intArg, ok := args[0].(*object.Integer)
	if !ok {
		return nil, builtinapi.Errorf("evaluator: Prime.prime_division: expected Integer, got %T", args[0])
	}
	n := intArg.ToBig()
	if n.Sign() == 0 {
		return builtinapi.RaiseBuiltin(env, "ZeroDivisionError", "divided by 0")
	}
	var factors []object.RubyObject
	work := new(big.Int).Set(n)
	if work.Sign() < 0 {
		work.Neg(work)
		factors = append(factors, pairOf(-1, 1))
	}
	exp := int64(0)
	two := big.NewInt(2)
	for new(big.Int).Mod(work, two).Sign() == 0 {
		work.Quo(work, two)
		exp++
	}
	if exp > 0 {
		factors = append(factors, pairOfBig(big.NewInt(2), exp))
	}
	div := big.NewInt(3)
	sq := new(big.Int)
	for sq.Mul(div, div); sq.Cmp(work) <= 0; sq.Mul(div, div) {
		exp = 0
		rem := new(big.Int)
		for rem.Mod(work, div); rem.Sign() == 0; rem.Mod(work, div) {
			work.Quo(work, div)
			exp++
		}
		if exp > 0 {
			factors = append(factors, pairOfBig(new(big.Int).Set(div), exp))
		}
		div.Add(div, two)
	}
	if work.Cmp(big.NewInt(1)) > 0 {
		factors = append(factors, pairOfBig(new(big.Int).Set(work), 1))
	}
	return object.NewArray(factors...), nil
}

// primeIntFromPrimeDivision recomposes an Integer from
// [[prime, exponent], ...] pairs.
func primeIntFromPrimeDivision(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	if len(args) != 1 {
		return nil, builtinapi.Errorf("evaluator: Prime.int_from_prime_division: wrong number of arguments (given %d, expected 1)", len(args))
	}
	arr, ok := args[0].(*object.Array)
	if !ok {
		return nil, builtinapi.Errorf("evaluator: Prime.int_from_prime_division: expected Array, got %T", args[0])
	}
	out := big.NewInt(1)
	for i, e := range arr.Elements {
		pair, ok := e.(*object.Array)
		if !ok || len(pair.Elements) != 2 {
			return nil, builtinapi.Errorf("evaluator: Prime.int_from_prime_division: element %d not a [prime, exponent] pair", i)
		}
		p, okP := pair.Elements[0].(*object.Integer)
		expI, okE := pair.Elements[1].(*object.Integer)
		if !okP || !okE {
			return nil, builtinapi.Errorf("evaluator: Prime.int_from_prime_division: pair %d must be Integers", i)
		}
		if expI.Value < 0 {
			return nil, builtinapi.Errorf("evaluator: Prime.int_from_prime_division: negative exponent")
		}
		out.Mul(out, new(big.Int).Exp(p.ToBig(), big.NewInt(expI.Value), nil))
	}
	return object.NewBigInteger(out), nil
}

func pairOf(prime, exp int64) *object.Array {
	return object.NewArray(object.NewInteger(prime), object.NewInteger(exp))
}

func pairOfBig(prime *big.Int, exp int64) *object.Array {
	return object.NewArray(object.NewBigInteger(prime), object.NewInteger(exp))
}
