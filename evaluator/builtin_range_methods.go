package evaluator

import (
	"github.com/lczyk/goruby/object"
)

func init() {
	c := object.RangeClass
	// Range.new(begin, end[, exclusive]) -- without it Class.new
	// returns a bare Instance and downstream method calls
	// (Range#to_a etc) crash on the recv.(*object.Range) cast.
	if c.ClassMethods == nil {
		c.ClassMethods = map[string]object.RubyMethod{}
	}
	c.ClassMethods["new"] = &object.BuiltinMethod{Name: "new", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		if len(args) < 2 || len(args) > 3 {
			return nil, errorf("evaluator: Range.new: expected 2..3 args, got %d", len(args))
		}
		exclusive := false
		if len(args) == 3 {
			if b, ok := args[2].(*object.Boolean); ok {
				exclusive = b.Value
			}
		}
		return object.NewRange(args[0], args[1], exclusive), nil
	}}
	add := func(name string, fn func(env *object.Environment, r *object.Range, args []object.RubyObject) (object.RubyObject, error)) {
		c.AddMethod(name, &object.BuiltinMethod{
			Name: name,
			Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
				return fn(env, recv.(*object.Range), args)
			},
		})
	}

	add("to_a", func(env *object.Environment, r *object.Range, args []object.RubyObject) (object.RubyObject, error) {
		elems, err := rangeToSlice(env, r)
		if err != nil {
			return nil, err
		}
		return object.NewArray(elems...), nil
	})
	add("lazy", func(env *object.Environment, r *object.Range, args []object.RubyObject) (object.RubyObject, error) {
		return lazyFromRange(r)
	})
	sizeFn := func(env *object.Environment, r *object.Range, args []object.RubyObject) (object.RubyObject, error) {
		lo, hi, ok := rangeIntegerBounds(r)
		if !ok {
			return nil, errorf("evaluator: Range#size needs Integer bounds")
		}
		n := hi - lo
		if !r.Exclusive {
			n++
		}
		if n < 0 {
			n = 0
		}
		return object.NewInteger(n), nil
	}
	add("size", sizeFn)
	add("count", sizeFn)
	add("length", sizeFn)
	add("first", func(env *object.Environment, r *object.Range, args []object.RubyObject) (object.RubyObject, error) {
		return r.Begin, nil
	})
	add("last", func(env *object.Environment, r *object.Range, args []object.RubyObject) (object.RubyObject, error) {
		return r.End, nil
	})
	add("begin", func(env *object.Environment, r *object.Range, args []object.RubyObject) (object.RubyObject, error) {
		return r.Begin, nil
	})
	add("end", func(env *object.Environment, r *object.Range, args []object.RubyObject) (object.RubyObject, error) {
		return r.End, nil
	})
	add("exclude_end?", func(env *object.Environment, r *object.Range, args []object.RubyObject) (object.RubyObject, error) {
		return object.BooleanOf(r.Exclusive), nil
	})
	add("min", func(env *object.Environment, r *object.Range, args []object.RubyObject) (object.RubyObject, error) {
		return r.Begin, nil
	})
	add("max", func(env *object.Environment, r *object.Range, args []object.RubyObject) (object.RubyObject, error) {
		i, ok := r.End.(*object.Integer)
		if !ok {
			return r.End, nil
		}
		if r.Exclusive {
			return object.NewInteger(i.Value - 1), nil
		}
		return r.End, nil
	})
	add("sum", func(env *object.Environment, r *object.Range, args []object.RubyObject) (object.RubyObject, error) {
		lo, hi, ok := rangeIntegerBounds(r)
		if !ok {
			return nil, errorf("evaluator: Range#sum needs Integer bounds")
		}
		end := hi
		if !r.Exclusive {
			end = hi + 1
		}
		n := end - lo
		if n <= 0 {
			return object.NewInteger(0), nil
		}
		s := n * (lo + (end - 1)) / 2
		return object.NewInteger(s), nil
	})
	// Range delegates each of these to its materialised Array via
	// callMethod; the Array adapter (or eventual inlined Array body)
	// handles them.
	viaArray := func(env *object.Environment, r *object.Range, args []object.RubyObject, name string) (object.RubyObject, error) {
		elems, err := rangeToSlice(env, r)
		if err != nil {
			return nil, err
		}
		return callMethod(env, object.NewArray(elems...), name, args)
	}
	for _, name := range []string{
		"reduce", "inject", "min_by", "max_by", "sort", "sort_by",
		"tally", "uniq", "each_slice", "each_cons", "each_with_index",
		"zip", "take", "drop", "join", "flatten",
	} {
		n := name
		add(n, func(env *object.Environment, r *object.Range, args []object.RubyObject) (object.RubyObject, error) {
			return viaArray(env, r, args, n)
		})
	}
	cover := func(env *object.Environment, r *object.Range, args []object.RubyObject) (object.RubyObject, error) {
		if len(args) != 1 {
			return nil, errorf("evaluator: wrong number of arguments to Range#cover?")
		}
		// Integer-bound fast path.
		if lo, hi, ok := rangeIntegerBounds(r); ok {
			vi, ok := args[0].(*object.Integer)
			if !ok {
				return object.FALSE, nil
			}
			if vi.Value < lo {
				return object.FALSE, nil
			}
			if r.Exclusive {
				return object.BooleanOf(vi.Value < hi), nil
			}
			return object.BooleanOf(vi.Value <= hi), nil
		}
		// String-bound: lex compare lo <= val <(=) hi. Args must be
		// real Strings (Symbols are not auto-coerced per MRI).
		if _, isStr := r.Begin.(*object.String); isStr {
			lo, _ := stringText(env, r.Begin)
			hi, hok := stringText(env, r.End)
			if !hok {
				return object.FALSE, nil
			}
			if _, isArgStr := args[0].(*object.String); !isArgStr {
				return object.FALSE, nil
			}
			v, _ := stringText(env, args[0])
			if v < lo {
				return object.FALSE, nil
			}
			if r.Exclusive {
				return object.BooleanOf(v < hi), nil
			}
			return object.BooleanOf(v <= hi), nil
		}
		// Float-bound: numeric compare via toFloat coercion. Integers
		// promote to Float for the comparison.
		if _, isFloat := r.Begin.(*object.Float); isFloat {
			loF, _ := r.Begin.(*object.Float)
			hiF, hok := r.End.(*object.Float)
			if !hok {
				return object.FALSE, nil
			}
			var v float64
			switch a := args[0].(type) {
			case *object.Float:
				v = a.Value
			case *object.Integer:
				v = float64(a.Value)
			default:
				return object.FALSE, nil
			}
			if v < loF.Value {
				return object.FALSE, nil
			}
			if r.Exclusive {
				return object.BooleanOf(v < hiF.Value), nil
			}
			return object.BooleanOf(v <= hiF.Value), nil
		}
		return object.FALSE, nil
	}
	add("include?", cover)
	add("cover?", cover)
}
