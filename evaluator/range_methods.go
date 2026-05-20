package evaluator

import (
	"github.com/lczyk/goruby/object"
)

func init() {
	c := object.RangeClass
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
		lo, hi, ok := rangeIntegerBounds(r)
		if !ok {
			return nil, errorf("evaluator: Range#cover? needs Integer bounds")
		}
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
	add("include?", cover)
	add("cover?", cover)
}
