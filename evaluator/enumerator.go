package evaluator

import (
	"sort"

	"github.com/lczyk/goruby/ast"
	"github.com/lczyk/goruby/object"
)

// bootstrapEnumeratorClass wires the Enumerator dispatch class. Covers
// the with_index / map / to_a / each shapes used by the corpus -- in
// particular the `sort_by.with_index { ... }` pattern alice's
// `stable_sort` reopening uses.
func bootstrapEnumeratorClass(env *object.Environment) *object.Class {
	c := object.EnumeratorClass
	if _, ok := env.Get("Enumerator"); ok {
		return c
	}
	addBlockOrPlainMethod(c, "with_index",
		func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
			return nil, errorf("evaluator: Enumerator#with_index without a block not supported")
		},
		enumeratorWithIndex,
	)
	addBlockOrPlainMethod(c, "each",
		func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
			// each w/out block: return receiver (the array we were
			// derived from) -- close enough for code that just wants
			// to chain. real mri returns the Enumerator itself.
			if e, ok := recv.(*object.Enumerator); ok {
				return e.Receiver, nil
			}
			return recv, nil
		},
		enumeratorEach,
	)
	addBlockMethod(c, "map", enumeratorMap)
	addBlockMethod(c, "select", enumeratorSelect)
	c.Methods["filter"] = c.Methods["select"]
	addBlockMethod(c, "reject", enumeratorReject)
	c.AddMethod("to_a", &object.BuiltinMethod{
		Name: "to_a",
		Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
			if e, ok := recv.(*object.Enumerator); ok {
				if arr, ok := e.Receiver.(*object.Array); ok {
					out := make([]object.RubyObject, len(arr.Elements))
					copy(out, arr.Elements)
					return object.NewArray(out...), nil
				}
			}
			return recv, nil
		},
	})
	env.SetGlobal("Enumerator", c)
	return c
}

// enumeratorWithIndex implements `enum.with_index(start=0) { |elem, idx| ... }`.
// Behaviour depends on the underlying method that produced the
// Enumerator. For sort_by, the block result is the sort key. For
// map, the block return values build the result array. For each,
// the block is invoked for side effects and the receiver returns.
func enumeratorWithIndex(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
	enum, ok := recv.(*object.Enumerator)
	if !ok {
		return nil, errorf("evaluator: Enumerator#with_index on non-Enumerator receiver %T", recv)
	}
	arr, ok := enum.Receiver.(*object.Array)
	if !ok {
		return nil, errorf("evaluator: Enumerator#with_index: receiver %T not iterable", enum.Receiver)
	}
	start := int64(0)
	if len(args) >= 1 {
		s, ok := args[0].(*object.Integer)
		if !ok {
			return nil, errorf("evaluator: Enumerator#with_index: start must be Integer, got %T", args[0])
		}
		start = s.Value
	}
	switch enum.Method {
	case "sort_by":
		// Compute sort key for each (elem, idx). Pair w/ original elem
		// + original index for tie-stable ordering (matches stable sort).
		type keyed struct {
			key  object.RubyObject
			elem object.RubyObject
			ord  int
		}
		keyed_s := make([]keyed, len(arr.Elements))
		for i, e := range arr.Elements {
			v, stop, err := iterStep(invoke, []object.RubyObject{e, object.NewInteger(int64(i) + start)})
			if err != nil {
				return nil, err
			}
			if stop {
				return v, nil
			}
			keyed_s[i] = keyed{key: v, elem: e, ord: i}
		}
		sort.SliceStable(keyed_s, func(i, j int) bool {
			c, _ := compareObjectsEnv(env, keyed_s[i].key, keyed_s[j].key)
			return c < 0
		})
		out := make([]object.RubyObject, len(keyed_s))
		for i, k := range keyed_s {
			out[i] = k.elem
		}
		return object.NewArray(out...), nil
	case "map", "collect":
		out := make([]object.RubyObject, 0, len(arr.Elements))
		for i, e := range arr.Elements {
			v, stop, err := iterStep(invoke, []object.RubyObject{e, object.NewInteger(int64(i) + start)})
			if err != nil {
				return nil, err
			}
			if stop {
				return v, nil
			}
			out = append(out, v)
		}
		return object.NewArray(out...), nil
	case "each", "":
		for i, e := range arr.Elements {
			_, stop, err := iterStep(invoke, []object.RubyObject{e, object.NewInteger(int64(i) + start)})
			if err != nil {
				return nil, err
			}
			if stop {
				return enum.Receiver, nil
			}
		}
		return enum.Receiver, nil
	case "select", "filter":
		out := make([]object.RubyObject, 0, len(arr.Elements))
		for i, e := range arr.Elements {
			v, stop, err := iterStep(invoke, []object.RubyObject{e, object.NewInteger(int64(i) + start)})
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
	case "reject":
		out := make([]object.RubyObject, 0, len(arr.Elements))
		for i, e := range arr.Elements {
			v, stop, err := iterStep(invoke, []object.RubyObject{e, object.NewInteger(int64(i) + start)})
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
	}
	return nil, errorf("evaluator: Enumerator#with_index not supported for method %q", enum.Method)
}

// enumeratorEach implements Enumerator#each { block }. Currently
// dispatches the underlying method's plain each over the receiver
// array, ignoring the original method's transformation semantics --
// good enough for "convert enum back to iterable".
func enumeratorEach(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
	enum, ok := recv.(*object.Enumerator)
	if !ok {
		return nil, errorf("evaluator: Enumerator#each on non-Enumerator receiver %T", recv)
	}
	arr, ok := enum.Receiver.(*object.Array)
	if !ok {
		return nil, errorf("evaluator: Enumerator#each: receiver %T not iterable", enum.Receiver)
	}
	for _, e := range arr.Elements {
		_, stop, err := iterStep(invoke, []object.RubyObject{e})
		if err != nil {
			return nil, err
		}
		if stop {
			break
		}
	}
	return enum.Receiver, nil
}

// enumeratorSelect implements Enumerator#select { block } -- keep
// elements where block returns truthy.
func enumeratorSelect(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
	return enumFilter(env, recv, invoke, true)
}

// enumeratorReject is the inverse of select.
func enumeratorReject(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
	return enumFilter(env, recv, invoke, false)
}

func enumFilter(env *object.Environment, recv object.RubyObject, invoke blockCallback, keepTruthy bool) (object.RubyObject, error) {
	enum, ok := recv.(*object.Enumerator)
	if !ok {
		return nil, errorf("evaluator: Enumerator filter on non-Enumerator receiver %T", recv)
	}
	arr, ok := enum.Receiver.(*object.Array)
	if !ok {
		return nil, errorf("evaluator: Enumerator filter: receiver %T not iterable", enum.Receiver)
	}
	out := make([]object.RubyObject, 0, len(arr.Elements))
	for _, e := range arr.Elements {
		v, stop, err := iterStep(invoke, []object.RubyObject{e})
		if err != nil {
			return nil, err
		}
		if stop {
			return v, nil
		}
		if truthy(v) == keepTruthy {
			out = append(out, e)
		}
	}
	return object.NewArray(out...), nil
}

// enumeratorMap implements Enumerator#map { block }.
func enumeratorMap(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
	enum, ok := recv.(*object.Enumerator)
	if !ok {
		return nil, errorf("evaluator: Enumerator#map on non-Enumerator receiver %T", recv)
	}
	arr, ok := enum.Receiver.(*object.Array)
	if !ok {
		return nil, errorf("evaluator: Enumerator#map: receiver %T not iterable", enum.Receiver)
	}
	out := make([]object.RubyObject, 0, len(arr.Elements))
	for _, e := range arr.Elements {
		v, stop, err := iterStep(invoke, []object.RubyObject{e})
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
