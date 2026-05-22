package evaluator

import (
	"github.com/lczyk/goruby/ast"

	"github.com/lczyk/goruby/object"
)

// Block-aware Range methods. For methods covered by Array (select,
// reject, count, find, etc.), the implementation materialises the
// range to an Array and re-dispatches through the same block-aware
// path on ArrayClass.

func init() {
	c := object.RangeClass

	asRange := func(recv object.RubyObject, name string) (*object.Range, error) {
		r, ok := recv.(*object.Range)
		if !ok {
			return nil, errorf("evaluator: Range#%s on non-Range %T", name, recv)
		}
		return r, nil
	}

	addBlockMethod(c, "each", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
		r, err := asRange(recv, "each")
		if err != nil {
			return nil, err
		}
		if lo, hi, ok := rangeIntegerBounds(r); ok {
			end := hi
			if !r.Exclusive {
				end++
			}
			for k := lo; k < end; k++ {
				_, stop, err := yieldOne(invoke, object.NewInteger(k))
				if err != nil {
					return nil, err
				}
				if stop {
					return r, nil
				}
			}
			return r, nil
		}
		elems, err := rangeToSlice(env, r)
		if err != nil {
			return nil, err
		}
		for _, e := range elems {
			_, stop, err := yieldOne(invoke, e)
			if err != nil {
				return nil, err
			}
			if stop {
				return r, nil
			}
		}
		return r, nil
	})

	addBlockMethod(c, "reverse_each", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
		r, err := asRange(recv, "reverse_each")
		if err != nil {
			return nil, err
		}
		if lo, hi, ok := rangeIntegerBounds(r); ok {
			end := hi
			if !r.Exclusive {
				end++
			}
			for k := end - 1; k >= lo; k-- {
				_, stop, err := yieldOne(invoke, object.NewInteger(k))
				if err != nil {
					return nil, err
				}
				if stop {
					return r, nil
				}
			}
			return r, nil
		}
		elems, err := rangeToSlice(env, r)
		if err != nil {
			return nil, err
		}
		for i := len(elems) - 1; i >= 0; i-- {
			_, stop, err := yieldOne(invoke, elems[i])
			if err != nil {
				return nil, err
			}
			if stop {
				return r, nil
			}
		}
		return r, nil
	})

	mapFn := func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
		r, err := asRange(recv, "map")
		if err != nil {
			return nil, err
		}
		if lo, hi, ok := rangeIntegerBounds(r); ok {
			end := hi
			if !r.Exclusive {
				end++
			}
			out := make([]object.RubyObject, 0, end-lo)
			for k := lo; k < end; k++ {
				v, stop, err := yieldOne(invoke, object.NewInteger(k))
				if err != nil {
					return nil, err
				}
				if stop {
					return v, nil
				}
				out = append(out, v)
			}
			return object.NewArray(out...), nil
		}
		elems, err := rangeToSlice(env, r)
		if err != nil {
			return nil, err
		}
		out := make([]object.RubyObject, 0, len(elems))
		for _, e := range elems {
			v, stop, err := yieldOne(invoke, e)
			if err != nil {
				return nil, err
			}
			if stop {
				return v, nil
			}
			out = append(out, v)
		}
		return object.NewArray(out...), nil
	}
	addBlockMethod(c, "map", mapFn)
	c.Methods["collect"] = c.Methods["map"]

	// Methods delegated to Array via materialisation. Re-dispatch
	// callMethodWithBlock so the Array-class block-aware methods run.
	delegateNames := []string{
		"select", "filter", "find_all", "reject",
		"find", "detect",
		"all?", "any?", "none?", "count",
		"group_by", "partition",
		"min_by", "max_by",
		"take_while", "drop_while",
		"each_slice", "each_cons",
		"reduce", "inject",
		"each_with_object", "each_with_index",
		"sort", "sort_by",
		"chunk_while", "slice_when",
	}
	for _, name := range delegateNames {
		n := name
		addBlockOrPlainMethod(c, n,
			func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
				r, err := asRange(recv, n)
				if err != nil {
					return nil, err
				}
				elems, err := rangeToSlice(env, r)
				if err != nil {
					return nil, err
				}
				return callMethod(env, object.NewArray(elems...), n, args)
			},
			func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, blk *ast.BlockExpression) (object.RubyObject, error) {
				r, err := asRange(recv, n)
				if err != nil {
					return nil, err
				}
				elems, err := rangeToSlice(env, r)
				if err != nil {
					return nil, err
				}
				arr := object.NewArray(elems...)
				marker := &goBlockMarker{fn: invoke, blk: blk}
				return dispatchWithBlock(env, arr, n, args, marker)
			})
	}
}
