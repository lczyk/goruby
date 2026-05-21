package evaluator

import (
	"github.com/lczyk/goruby/ast"

	"github.com/lczyk/goruby/object"
)

// Block-aware Array methods, registered on ArrayClass so block dispatch
// goes through the standard class chain (object.Send) instead of a
// type-switch. The no-block forms of these names are implemented in
// callArrayMethod (array_impl.go); the dispatcher in block.go picks
// the block form when a block is present, the plain form otherwise.

func init() {
	c := object.ArrayClass

	asArray := func(recv object.RubyObject, name string) (*object.Array, error) {
		a, ok := recv.(*object.Array)
		if !ok {
			return nil, errorf("evaluator: Array#%s on non-Array %T", name, recv)
		}
		return a, nil
	}

	plain := func(name string) func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
		return func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
			a, err := asArray(recv, name)
			if err != nil {
				return nil, err
			}
			return callArrayMethod(env, a, name, args)
		}
	}

	addBlockOrPlainMethod(c, "map", nil, func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
		arr, err := asArray(recv, "map")
		if err != nil {
			return nil, err
		}
		out := make([]object.RubyObject, 0, len(arr.Elements))
		for _, e := range arr.Elements {
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
	})
	c.Methods["collect"] = c.Methods["map"]

	addBlockMethod(c, "flat_map", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
		arr, err := asArray(recv, "flat_map")
		if err != nil {
			return nil, err
		}
		out := []object.RubyObject{}
		for _, e := range arr.Elements {
			v, stop, err := yieldOne(invoke, e)
			if err != nil {
				return nil, err
			}
			if stop {
				return v, nil
			}
			if a, ok := v.(*object.Array); ok {
				out = append(out, a.Elements...)
			} else {
				out = append(out, v)
			}
		}
		return object.NewArray(out...), nil
	})
	c.Methods["collect_concat"] = c.Methods["flat_map"]

	selectFn := func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
		arr, err := asArray(recv, "select")
		if err != nil {
			return nil, err
		}
		out := make([]object.RubyObject, 0, len(arr.Elements))
		for _, e := range arr.Elements {
			v, stop, err := yieldOne(invoke, e)
			if err != nil {
				return nil, err
			}
			if stop {
				return v, nil
			}
			if truthy(v) {
				out = append(out, e)
			}
		}
		return object.NewArray(out...), nil
	}
	addBlockOrPlainMethod(c, "select", nil, selectFn)
	c.Methods["filter"] = c.Methods["select"]
	c.Methods["find_all"] = c.Methods["select"]

	// In-place variants: select!/filter!/keep_if rewrite the receiver to
	// keep only block-truthy elements. select! / filter! return nil when
	// no element was removed (matches MRI); keep_if always returns self.
	inplaceFilter := func(keepTruthy, returnNilIfUnchanged bool, name string) func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
		return func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
			arr, err := asArray(recv, name)
			if err != nil {
				return nil, err
			}
			out := arr.Elements[:0]
			changed := false
			for _, e := range arr.Elements {
				v, stop, err := yieldOne(invoke, e)
				if err != nil {
					return nil, err
				}
				if stop {
					return v, nil
				}
				keep := truthy(v) == keepTruthy
				if keep {
					out = append(out, e)
				} else {
					changed = true
				}
			}
			arr.Elements = out
			if returnNilIfUnchanged && !changed {
				return object.NIL, nil
			}
			return arr, nil
		}
	}
	addBlockOrPlainMethod(c, "select!", nil, inplaceFilter(true, true, "select!"))
	c.Methods["filter!"] = c.Methods["select!"]
	addBlockOrPlainMethod(c, "keep_if", nil, inplaceFilter(true, false, "keep_if"))
	addBlockOrPlainMethod(c, "reject!", nil, inplaceFilter(false, true, "reject!"))
	addBlockOrPlainMethod(c, "delete_if", nil, inplaceFilter(false, false, "delete_if"))

	addBlockOrPlainMethod(c, "reject", nil, func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
		arr, err := asArray(recv, "reject")
		if err != nil {
			return nil, err
		}
		out := make([]object.RubyObject, 0, len(arr.Elements))
		for _, e := range arr.Elements {
			v, stop, err := yieldOne(invoke, e)
			if err != nil {
				return nil, err
			}
			if stop {
				return v, nil
			}
			if !truthy(v) {
				out = append(out, e)
			}
		}
		return object.NewArray(out...), nil
	})

	addBlockMethod(c, "each", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
		arr, err := asArray(recv, "each")
		if err != nil {
			return nil, err
		}
		for _, e := range arr.Elements {
			_, stop, err := yieldOne(invoke, e)
			if err != nil {
				return nil, err
			}
			if stop {
				return arr, nil
			}
		}
		return arr, nil
	})

	addBlockOrPlainMethod(c, "each_with_index", plain("each_with_index"),
		func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
			arr, err := asArray(recv, "each_with_index")
			if err != nil {
				return nil, err
			}
			for i, e := range arr.Elements {
				_, stop, err := iterStep(invoke, []object.RubyObject{e, object.NewInteger(int64(i))})
				if err != nil {
					return nil, err
				}
				if stop {
					return arr, nil
				}
			}
			return arr, nil
		})

	addBlockOrPlainMethod(c, "zip", plain("zip"),
		func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
			arr, err := asArray(recv, "zip")
			if err != nil {
				return nil, err
			}
			others := make([]*object.Array, 0, len(args))
			for _, a := range args {
				oa, ok := a.(*object.Array)
				if !ok {
					return nil, errorf("evaluator: Array#zip needs Array args")
				}
				others = append(others, oa)
			}
			for i, e := range arr.Elements {
				tuple := make([]object.RubyObject, 1+len(others))
				tuple[0] = e
				for j, o := range others {
					if i < len(o.Elements) {
						tuple[j+1] = o.Elements[i]
					} else {
						tuple[j+1] = object.NIL
					}
				}
				_, stop, err := iterStep(invoke, []object.RubyObject{object.NewArray(tuple...)})
				if err != nil {
					return nil, err
				}
				if stop {
					return object.NIL, nil
				}
			}
			return object.NIL, nil
		})

	addBlockMethod(c, "each_with_object", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
		arr, err := asArray(recv, "each_with_object")
		if err != nil {
			return nil, err
		}
		if len(args) != 1 {
			return nil, errorf("evaluator: each_with_object expects 1 arg, got %d", len(args))
		}
		memo := args[0]
		for _, e := range arr.Elements {
			_, stop, err := iterStep(invoke, []object.RubyObject{e, memo})
			if err != nil {
				return nil, err
			}
			if stop {
				return memo, nil
			}
		}
		return memo, nil
	})

	addBlockOrPlainMethod(c, "each_slice", plain("each_slice"),
		func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
			arr, err := asArray(recv, "each_slice")
			if err != nil {
				return nil, err
			}
			if len(args) != 1 {
				return nil, errorf("evaluator: each_slice expects 1 arg")
			}
			n, ok := args[0].(*object.Integer)
			if !ok || n.Value <= 0 {
				return nil, errorf("evaluator: each_slice needs positive Integer")
			}
			step := int(n.Value)
			for i := 0; i < len(arr.Elements); i += step {
				end := min(i+step, len(arr.Elements))
				slice := make([]object.RubyObject, end-i)
				copy(slice, arr.Elements[i:end])
				_, stop, err := iterStep(invoke, []object.RubyObject{object.NewArray(slice...)})
				if err != nil {
					return nil, err
				}
				if stop {
					return arr, nil
				}
			}
			return arr, nil
		})

	addBlockOrPlainMethod(c, "each_cons", plain("each_cons"),
		func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
			arr, err := asArray(recv, "each_cons")
			if err != nil {
				return nil, err
			}
			if len(args) != 1 {
				return nil, errorf("evaluator: each_cons expects 1 arg")
			}
			n, ok := args[0].(*object.Integer)
			if !ok || n.Value <= 0 {
				return nil, errorf("evaluator: each_cons needs positive Integer")
			}
			w := int(n.Value)
			for i := 0; i+w <= len(arr.Elements); i++ {
				slice := make([]object.RubyObject, w)
				copy(slice, arr.Elements[i:i+w])
				_, stop, err := iterStep(invoke, []object.RubyObject{object.NewArray(slice...)})
				if err != nil {
					return nil, err
				}
				if stop {
					return arr, nil
				}
			}
			return object.NIL, nil
		})

	addBlockOrPlainMethod(c, "reduce", plain("reduce"),
		func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
			arr, err := asArray(recv, "reduce")
			if err != nil {
				return nil, err
			}
			var acc object.RubyObject
			start := 0
			switch len(args) {
			case 0:
				if len(arr.Elements) == 0 {
					return object.NIL, nil
				}
				acc = arr.Elements[0]
				start = 1
			case 1:
				acc = args[0]
			default:
				return nil, errorf("evaluator: Array#reduce: wrong number of arguments (%d)", len(args))
			}
			for i := start; i < len(arr.Elements); i++ {
				v, stop, err := iterStep(invoke, []object.RubyObject{acc, arr.Elements[i]})
				if err != nil {
					return nil, err
				}
				if stop {
					return v, nil
				}
				acc = v
			}
			return acc, nil
		})
	c.Methods["inject"] = c.Methods["reduce"]

	addBlockOrPlainMethod(c, "count", plain("count"),
		func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
			arr, err := asArray(recv, "count")
			if err != nil {
				return nil, err
			}
			n := 0
			for _, e := range arr.Elements {
				v, stop, err := yieldOne(invoke, e)
				if err != nil {
					return nil, err
				}
				if stop {
					return v, nil
				}
				if truthy(v) {
					n++
				}
			}
			return object.NewInteger(int64(n)), nil
		})

	addBlockOrPlainMethod(c, "sort", plain("sort"),
		func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
			arr, err := asArray(recv, "sort")
			if err != nil {
				return nil, err
			}
			out := make([]object.RubyObject, len(arr.Elements))
			copy(out, arr.Elements)
			var sortErr error
			sortStable(len(out), func(i, j int) bool {
				if sortErr != nil {
					return false
				}
				v, _, err := iterStep(invoke, []object.RubyObject{out[i], out[j]})
				if err != nil {
					sortErr = err
					return false
				}
				if c, ok := v.(*object.Integer); ok {
					return c.Value < 0
				}
				sortErr = errorf("evaluator: sort block must return Integer, got %T", v)
				return false
			}, func(i, j int) {
				out[i], out[j] = out[j], out[i]
			})
			if sortErr != nil {
				return nil, sortErr
			}
			return object.NewArray(out...), nil
		})

	addBlockMethod(c, "sort_by", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
		arr, err := asArray(recv, "sort_by")
		if err != nil {
			return nil, err
		}
		keys := make([]object.RubyObject, len(arr.Elements))
		vals := make([]object.RubyObject, len(arr.Elements))
		copy(vals, arr.Elements)
		for i, e := range arr.Elements {
			v, stop, err := yieldOne(invoke, e)
			if err != nil {
				return nil, err
			}
			if stop {
				return v, nil
			}
			keys[i] = v
		}
		sortStable(len(vals), func(i, j int) bool {
			c, _ := compareObjects(keys[i], keys[j])
			return c < 0
		}, func(i, j int) {
			vals[i], vals[j] = vals[j], vals[i]
			keys[i], keys[j] = keys[j], keys[i]
		})
		return object.NewArray(vals...), nil
	})

	addBlockMethod(c, "find", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
		arr, err := asArray(recv, "find")
		if err != nil {
			return nil, err
		}
		for _, e := range arr.Elements {
			v, stop, err := yieldOne(invoke, e)
			if err != nil {
				return nil, err
			}
			if stop {
				return v, nil
			}
			if truthy(v) {
				return e, nil
			}
		}
		return object.NIL, nil
	})
	c.Methods["detect"] = c.Methods["find"]

	addBlockMethod(c, "group_by", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
		arr, err := asArray(recv, "group_by")
		if err != nil {
			return nil, err
		}
		groups := []object.HashEntry{}
		for _, e := range arr.Elements {
			k, stop, err := yieldOne(invoke, e)
			if err != nil {
				return nil, err
			}
			if stop {
				return k, nil
			}
			placed := false
			for i := range groups {
				if rubyEqual(groups[i].Key, k) {
					bucket := groups[i].Value.(*object.Array)
					bucket.Elements = append(bucket.Elements, e)
					placed = true
					break
				}
			}
			if !placed {
				groups = append(groups, object.HashEntry{Key: k, Value: object.NewArray(e)})
			}
		}
		return object.NewHash(groups...), nil
	})

	addBlockMethod(c, "partition", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
		arr, err := asArray(recv, "partition")
		if err != nil {
			return nil, err
		}
		truthyOut := []object.RubyObject{}
		falsyOut := []object.RubyObject{}
		for _, e := range arr.Elements {
			v, stop, err := yieldOne(invoke, e)
			if err != nil {
				return nil, err
			}
			if stop {
				return v, nil
			}
			if truthy(v) {
				truthyOut = append(truthyOut, e)
			} else {
				falsyOut = append(falsyOut, e)
			}
		}
		return object.NewArray(object.NewArray(truthyOut...), object.NewArray(falsyOut...)), nil
	})

	addBlockOrPlainMethod(c, "min_by", plain("min"),
		func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
			arr, err := asArray(recv, "min_by")
			if err != nil {
				return nil, err
			}
			var bestVal object.RubyObject
			var bestKey object.RubyObject
			for _, e := range arr.Elements {
				k, stop, err := yieldOne(invoke, e)
				if err != nil {
					return nil, err
				}
				if stop {
					return k, nil
				}
				if bestKey == nil {
					bestKey = k
					bestVal = e
					continue
				}
				c, ok := compareObjectsEnv(env, k, bestKey)
				if !ok {
					return nil, errorf("evaluator: min_by comparison failed")
				}
				if c < 0 {
					bestKey = k
					bestVal = e
				}
			}
			if bestVal == nil {
				return object.NIL, nil
			}
			return bestVal, nil
		})

	addBlockMethod(c, "minmax_by", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
		arr, err := asArray(recv, "minmax_by")
		if err != nil {
			return nil, err
		}
		var loVal, hiVal object.RubyObject
		var loKey, hiKey object.RubyObject
		for _, e := range arr.Elements {
			k, stop, err := yieldOne(invoke, e)
			if err != nil {
				return nil, err
			}
			if stop {
				return k, nil
			}
			if loKey == nil {
				loKey, loVal = k, e
				hiKey, hiVal = k, e
				continue
			}
			if c, ok := compareObjectsEnv(env, k, loKey); ok && c < 0 {
				loKey, loVal = k, e
			}
			if c, ok := compareObjectsEnv(env, k, hiKey); ok && c > 0 {
				hiKey, hiVal = k, e
			}
		}
		if loVal == nil {
			return object.NewArray(object.NIL, object.NIL), nil
		}
		return object.NewArray(loVal, hiVal), nil
	})

	addBlockOrPlainMethod(c, "max_by", plain("max"),
		func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
			arr, err := asArray(recv, "max_by")
			if err != nil {
				return nil, err
			}
			var bestVal object.RubyObject
			var bestKey object.RubyObject
			for _, e := range arr.Elements {
				k, stop, err := yieldOne(invoke, e)
				if err != nil {
					return nil, err
				}
				if stop {
					return k, nil
				}
				if bestKey == nil {
					bestKey = k
					bestVal = e
					continue
				}
				c, ok := compareObjectsEnv(env, k, bestKey)
				if !ok {
					return nil, errorf("evaluator: max_by comparison failed")
				}
				if c > 0 {
					bestKey = k
					bestVal = e
				}
			}
			if bestVal == nil {
				return object.NIL, nil
			}
			return bestVal, nil
		})

	addBlockMethod(c, "take_while", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
		arr, err := asArray(recv, "take_while")
		if err != nil {
			return nil, err
		}
		out := []object.RubyObject{}
		for _, e := range arr.Elements {
			v, stop, err := yieldOne(invoke, e)
			if err != nil {
				return nil, err
			}
			if stop {
				return v, nil
			}
			if !truthy(v) {
				break
			}
			out = append(out, e)
		}
		return object.NewArray(out...), nil
	})

	addBlockMethod(c, "drop_while", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
		arr, err := asArray(recv, "drop_while")
		if err != nil {
			return nil, err
		}
		started := false
		out := []object.RubyObject{}
		for _, e := range arr.Elements {
			if started {
				out = append(out, e)
				continue
			}
			v, stop, err := yieldOne(invoke, e)
			if err != nil {
				return nil, err
			}
			if stop {
				return v, nil
			}
			if !truthy(v) {
				started = true
				out = append(out, e)
			}
		}
		return object.NewArray(out...), nil
	})

	addBlockMethod(c, "chunk_while", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
		arr, err := asArray(recv, "chunk_while")
		if err != nil {
			return nil, err
		}
		if len(arr.Elements) == 0 {
			return object.NewArray(), nil
		}
		out := []object.RubyObject{}
		current := []object.RubyObject{arr.Elements[0]}
		for i := 1; i < len(arr.Elements); i++ {
			prev := arr.Elements[i-1]
			cur := arr.Elements[i]
			v, stop, err := iterStep(invoke, []object.RubyObject{prev, cur})
			if err != nil {
				return nil, err
			}
			if stop {
				return v, nil
			}
			if truthy(v) {
				current = append(current, cur)
				continue
			}
			out = append(out, object.NewArray(current...))
			current = []object.RubyObject{cur}
		}
		out = append(out, object.NewArray(current...))
		return object.NewArray(out...), nil
	})

	addBlockMethod(c, "slice_when", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
		arr, err := asArray(recv, "slice_when")
		if err != nil {
			return nil, err
		}
		if len(arr.Elements) == 0 {
			return object.NewArray(), nil
		}
		out := []object.RubyObject{}
		current := []object.RubyObject{arr.Elements[0]}
		for i := 1; i < len(arr.Elements); i++ {
			prev := arr.Elements[i-1]
			cur := arr.Elements[i]
			v, stop, err := iterStep(invoke, []object.RubyObject{prev, cur})
			if err != nil {
				return nil, err
			}
			if stop {
				return v, nil
			}
			if truthy(v) {
				out = append(out, object.NewArray(current...))
				current = []object.RubyObject{cur}
				continue
			}
			current = append(current, cur)
		}
		out = append(out, object.NewArray(current...))
		return object.NewArray(out...), nil
	})

	addBlockOrPlainMethod(c, "any?", plain("any?"),
		func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
			arr, err := asArray(recv, "any?")
			if err != nil {
				return nil, err
			}
			for _, e := range arr.Elements {
				v, stop, err := yieldOne(invoke, e)
				if err != nil {
					return nil, err
				}
				if stop {
					return v, nil
				}
				if truthy(v) {
					return object.TRUE, nil
				}
			}
			return object.FALSE, nil
		})

	addBlockOrPlainMethod(c, "all?", plain("all?"),
		func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
			arr, err := asArray(recv, "all?")
			if err != nil {
				return nil, err
			}
			for _, e := range arr.Elements {
				v, stop, err := yieldOne(invoke, e)
				if err != nil {
					return nil, err
				}
				if stop {
					return v, nil
				}
				if !truthy(v) {
					return object.FALSE, nil
				}
			}
			return object.TRUE, nil
		})

	addBlockOrPlainMethod(c, "none?", plain("none?"),
		func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
			arr, err := asArray(recv, "none?")
			if err != nil {
				return nil, err
			}
			for _, e := range arr.Elements {
				v, stop, err := yieldOne(invoke, e)
				if err != nil {
					return nil, err
				}
				if stop {
					return v, nil
				}
				if truthy(v) {
					return object.FALSE, nil
				}
			}
			return object.TRUE, nil
		})
}
