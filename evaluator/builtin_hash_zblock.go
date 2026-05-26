package evaluator

import (
	"github.com/lczyk/goruby/ast"

	"github.com/lczyk/goruby/object"
)

// Block-aware Hash methods, registered on HashClass.

func init() {
	c := object.HashClass

	asHash := func(recv object.RubyObject, name string) (*object.Hash, error) {
		h, ok := recv.(*object.Hash)
		if !ok {
			return nil, errorf("evaluator: Hash#%s on non-Hash %T", name, recv)
		}
		return h, nil
	}
	plain := func(name string) func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
		return func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
			h, err := asHash(recv, name)
			if err != nil {
				return nil, err
			}
			return callHashMethod(env, h, name, args)
		}
	}

	addBlockOrPlainMethod(c, "each", plain("each"),
		func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
			h, err := asHash(recv, "each")
			if err != nil {
				return nil, err
			}
			for _, e := range h.Entries {
				_, stop, err := iterStep(invoke, []object.RubyObject{e.Key, e.Value})
				if err != nil {
					return nil, err
				}
				if stop {
					return h, nil
				}
			}
			return h, nil
		})
	c.Methods["each_pair"] = c.Methods["each"]

	addBlockMethod(c, "each_key", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
		h, err := asHash(recv, "each_key")
		if err != nil {
			return nil, err
		}
		for _, e := range h.Entries {
			_, stop, err := iterStep(invoke, []object.RubyObject{e.Key})
			if err != nil {
				return nil, err
			}
			if stop {
				return h, nil
			}
		}
		return h, nil
	})

	addBlockMethod(c, "each_value", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
		h, err := asHash(recv, "each_value")
		if err != nil {
			return nil, err
		}
		for _, e := range h.Entries {
			_, stop, err := iterStep(invoke, []object.RubyObject{e.Value})
			if err != nil {
				return nil, err
			}
			if stop {
				return h, nil
			}
		}
		return h, nil
	})

	addBlockMethod(c, "map", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
		h, err := asHash(recv, "map")
		if err != nil {
			return nil, err
		}
		out := []object.RubyObject{}
		for _, e := range h.Entries {
			v, stop, err := iterStep(invoke, []object.RubyObject{e.Key, e.Value})
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

	selectFn := func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
		h, err := asHash(recv, "select")
		if err != nil {
			return nil, err
		}
		out := []object.HashEntry{}
		for _, e := range h.Entries {
			v, stop, err := iterStep(invoke, []object.RubyObject{e.Key, e.Value})
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
		return object.NewHash(out...), nil
	}
	addBlockMethod(c, "select", selectFn)
	c.Methods["filter"] = c.Methods["select"]
	c.Methods["find_all"] = c.Methods["select"]

	addBlockMethod(c, "reject", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
		h, err := asHash(recv, "reject")
		if err != nil {
			return nil, err
		}
		out := []object.HashEntry{}
		for _, e := range h.Entries {
			v, stop, err := iterStep(invoke, []object.RubyObject{e.Key, e.Value})
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
		return object.NewHash(out...), nil
	})

	addBlockMethod(c, "find", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
		h, err := asHash(recv, "find")
		if err != nil {
			return nil, err
		}
		for _, e := range h.Entries {
			v, stop, err := iterStep(invoke, []object.RubyObject{e.Key, e.Value})
			if err != nil {
				return nil, err
			}
			if stop {
				return v, nil
			}
			if truthy(v) {
				return object.NewArray(e.Key, e.Value), nil
			}
		}
		return object.NIL, nil
	})
	c.Methods["detect"] = c.Methods["find"]

	addBlockMethod(c, "any?", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
		h, err := asHash(recv, "any?")
		if err != nil {
			return nil, err
		}
		for _, e := range h.Entries {
			v, stop, err := iterStep(invoke, []object.RubyObject{e.Key, e.Value})
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

	addBlockMethod(c, "all?", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
		h, err := asHash(recv, "all?")
		if err != nil {
			return nil, err
		}
		for _, e := range h.Entries {
			v, stop, err := iterStep(invoke, []object.RubyObject{e.Key, e.Value})
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

	addBlockOrPlainMethod(c, "count",
		func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
			h, err := asHash(recv, "count")
			if err != nil {
				return nil, err
			}
			return object.NewInteger(int64(len(h.Entries))), nil
		},
		func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
			h, err := asHash(recv, "count")
			if err != nil {
				return nil, err
			}
			n := 0
			for _, e := range h.Entries {
				v, stop, err := iterStep(invoke, []object.RubyObject{e.Key, e.Value})
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

	addBlockMethod(c, "transform_values", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
		h, err := asHash(recv, "transform_values")
		if err != nil {
			return nil, err
		}
		out := make([]object.HashEntry, 0, len(h.Entries))
		for _, e := range h.Entries {
			v, stop, err := iterStep(invoke, []object.RubyObject{e.Value})
			if err != nil {
				return nil, err
			}
			if stop {
				return v, nil
			}
			out = append(out, object.HashEntry{Key: e.Key, Value: v})
		}
		return object.NewHash(out...), nil
	})

	addBlockMethod(c, "sum", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
		h, err := asHash(recv, "sum")
		if err != nil {
			return nil, err
		}
		var intSum int64
		var floatSum float64
		anyFloat := false
		if len(args) == 1 {
			if i, ok := args[0].(*object.Integer); ok {
				intSum = i.Value
			} else if f, ok := args[0].(*object.Float); ok {
				floatSum = f.Value
				anyFloat = true
			}
		}
		for _, e := range h.Entries {
			v, stop, err := iterStep(invoke, []object.RubyObject{e.Key, e.Value})
			if err != nil {
				return nil, err
			}
			if stop {
				return v, nil
			}
			switch x := v.(type) {
			case *object.Integer:
				if anyFloat {
					floatSum += float64(x.Value)
				} else {
					intSum += x.Value
				}
			case *object.Float:
				if !anyFloat {
					floatSum = float64(intSum)
					anyFloat = true
				}
				floatSum += x.Value
			default:
				return nil, errorf("evaluator: Hash#sum non-numeric: %T", x)
			}
		}
		if anyFloat {
			return object.NewFloat(floatSum), nil
		}
		return object.NewInteger(intSum), nil
	})

	minmaxBy := func(name string, want int) func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
		return func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
			h, err := asHash(recv, name)
			if err != nil {
				return nil, err
			}
			var bestVal object.HashEntry
			var bestKey object.RubyObject
			for _, e := range h.Entries {
				k, stop, err := iterStep(invoke, []object.RubyObject{e.Key, e.Value})
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
					return nil, errorf("evaluator: %s comparison failed", name)
				}
				if (want < 0 && c < 0) || (want > 0 && c > 0) {
					bestKey = k
					bestVal = e
				}
			}
			if bestKey == nil {
				return object.NIL, nil
			}
			return object.NewArray(bestVal.Key, bestVal.Value), nil
		}
	}
	minmaxByPlain := func(name string) func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
		return func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
			h, err := asHash(recv, name)
			if err != nil {
				return nil, err
			}
			pairs := make([]object.RubyObject, 0, len(h.Entries))
			for _, e := range h.Entries {
				pairs = append(pairs, object.NewArray(e.Key, e.Value))
			}
			return &object.Enumerator{Receiver: object.NewArray(pairs...), Method: name}, nil
		}
	}
	addBlockOrPlainMethod(c, "min_by", minmaxByPlain("min_by"), minmaxBy("min_by", -1))
	addBlockOrPlainMethod(c, "max_by", minmaxByPlain("max_by"), minmaxBy("max_by", 1))

	addBlockOrPlainMethod(c, "merge", plain("merge"),
		func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
			h, err := asHash(recv, "merge")
			if err != nil {
				return nil, err
			}
			out := make([]object.HashEntry, len(h.Entries))
			copy(out, h.Entries)
			for _, a := range args {
				other, ok := a.(*object.Hash)
				if !ok {
					return nil, errorf("evaluator: Hash#merge needs Hash args")
				}
				for _, e := range other.Entries {
					replaced := false
					for i := range out {
						if rubyEqual(out[i].Key, e.Key) {
							v, stop, err := iterStep(invoke, []object.RubyObject{e.Key, out[i].Value, e.Value})
							if err != nil {
								return nil, err
							}
							if stop {
								return v, nil
							}
							out[i].Value = v
							replaced = true
							break
						}
					}
					if !replaced {
						out = append(out, e)
					}
				}
			}
			return object.NewHash(out...), nil
		})

	addBlockMethod(c, "transform_keys", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
		h, err := asHash(recv, "transform_keys")
		if err != nil {
			return nil, err
		}
		out := make([]object.HashEntry, 0, len(h.Entries))
		for _, e := range h.Entries {
			k, stop, err := iterStep(invoke, []object.RubyObject{e.Key})
			if err != nil {
				return nil, err
			}
			if stop {
				return k, nil
			}
			out = append(out, object.HashEntry{Key: k, Value: e.Value})
		}
		return object.NewHash(out...), nil
	})

	addBlockOrPlainMethod(c, "sort_by",
		func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
			h, err := asHash(recv, "sort_by")
			if err != nil {
				return nil, err
			}
			pairs := make([]object.RubyObject, 0, len(h.Entries))
			for _, e := range h.Entries {
				pairs = append(pairs, object.NewArray(e.Key, e.Value))
			}
			return &object.Enumerator{Receiver: object.NewArray(pairs...), Method: "sort_by"}, nil
		},
		func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
			h, err := asHash(recv, "sort_by")
			if err != nil {
				return nil, err
			}
			type keyed struct {
				key object.RubyObject
				ent object.HashEntry
			}
			ke := make([]keyed, 0, len(h.Entries))
			for _, e := range h.Entries {
				v, stop, err := iterStep(invoke, []object.RubyObject{e.Key, e.Value})
				if err != nil {
					return nil, err
				}
				if stop {
					return v, nil
				}
				ke = append(ke, keyed{key: v, ent: e})
			}
			sortStable(len(ke), func(i, j int) bool {
				c, _ := compareObjects(ke[i].key, ke[j].key)
				return c < 0
			}, func(i, j int) {
				ke[i], ke[j] = ke[j], ke[i]
			})
			out := make([]object.RubyObject, len(ke))
			for i, k := range ke {
				out[i] = object.NewArray(k.ent.Key, k.ent.Value)
			}
			return object.NewArray(out...), nil
		})

	// Hash Enumerable-shaped delegations that materialise to
	// [[k,v], ...] then dispatch on the resulting Array. Saves
	// re-implementing each one.
	delegateNames := []string{
		"group_by", "partition",
		"flat_map", "collect_concat",
		"take_while", "drop_while",
		"chunk_while", "slice_when",
	}
	for _, name := range delegateNames {
		n := name
		addBlockMethod(c, n, func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, blk *ast.BlockExpression) (object.RubyObject, error) {
			h, err := asHash(recv, n)
			if err != nil {
				return nil, err
			}
			pairs := make([]object.RubyObject, len(h.Entries))
			for i, e := range h.Entries {
				pairs[i] = object.NewArray(e.Key, e.Value)
			}
			arr := object.NewArray(pairs...)
			marker := &goBlockMarker{fn: invoke, blk: blk}
			return dispatchWithBlock(env, arr, n, args, marker)
		})
	}
}
