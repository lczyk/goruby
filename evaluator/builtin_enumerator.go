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
			// Without a block, return a fresh Enumerator over [elem, idx]
			// pairs. Callers chain .to_a / .map etc on the result.
			enum, ok := recv.(*object.Enumerator)
			if !ok {
				return nil, errorf("evaluator: Enumerator#with_index on non-Enumerator %T", recv)
			}
			arr, ok := enum.Receiver.(*object.Array)
			if !ok {
				return nil, errorf("evaluator: Enumerator#with_index: receiver %T not iterable", enum.Receiver)
			}
			start := int64(0)
			if len(args) >= 1 {
				if s, ok := args[0].(*object.Integer); ok {
					start = s.Value
				}
			}
			pairs := make([]object.RubyObject, len(arr.Elements))
			for i, e := range arr.Elements {
				pairs[i] = object.NewArray(e, object.NewInteger(int64(i)+start))
			}
			return &object.Enumerator{Receiver: object.NewArray(pairs...), Method: "each"}, nil
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
	// Note: Enumerator.new { |y| ... } is handled in the generic
	// Class#new builtin (class_dispatch.go) where the block payload
	// is delivered correctly w/ the caller's self preserved.
	addBlockMethod(c, "select", enumeratorSelect)
	c.Methods["filter"] = c.Methods["select"]
	addBlockMethod(c, "reject", enumeratorReject)
	addBlockMethod(c, "reduce", enumeratorReduce)
	c.Methods["inject"] = c.Methods["reduce"]
	c.AddMethod("count", &object.BuiltinMethod{
		Name: "count",
		Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
			if e, ok := recv.(*object.Enumerator); ok {
				if arr, ok := e.Receiver.(*object.Array); ok {
					return object.NewInteger(int64(len(arr.Elements))), nil
				}
			}
			return object.NewInteger(0), nil
		},
	})
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
	// Enumerator#first(n=1) -- take first n elements from the buffered
	// receiver array. With no arg, returns the single first element.
	c.AddMethod("first", &object.BuiltinMethod{
		Name: "first",
		Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
			e, ok := recv.(*object.Enumerator)
			if !ok {
				return object.NIL, nil
			}
			arr, ok := e.Receiver.(*object.Array)
			if !ok {
				return object.NIL, nil
			}
			if len(args) == 0 {
				if len(arr.Elements) == 0 {
					return object.NIL, nil
				}
				return arr.Elements[0], nil
			}
			n, ok := args[0].(*object.Integer)
			if !ok {
				return nil, errorf("evaluator: Enumerator#first count must be Integer")
			}
			k := int(n.Value)
			if k < 0 {
				return nil, errorf("evaluator: negative array size")
			}
			if k > len(arr.Elements) {
				k = len(arr.Elements)
			}
			out := make([]object.RubyObject, k)
			copy(out, arr.Elements[:k])
			return object.NewArray(out...), nil
		},
	})
	c.AddMethod("take", c.Methods["first"])
	env.SetGlobal("Enumerator", c)
	return c
}

// enumeratorClassNew implements `Enumerator.new { |yielder| ... }`.
// The real mri Enumerator is lazy -- block runs only when the
// resulting Enumerator is iterated, and the yielder buffers one
// value at a time. We fake the API with eager evaluation: run the
// block immediately, give it a yielder that appends to a buffer,
// then wrap the collected values as an Enumerator over an Array.
// Good enough for the common shape `Enumerator.new { |y| something.each { |v| y.yield v } }`.
func enumeratorClassNew(c *object.Class) func(*object.Environment, []object.RubyObject) (object.RubyObject, error) {
	return func(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
		blkAny := env.CurrentBlock
		if blkAny == nil {
			return &object.Enumerator{Receiver: object.NewArray(), Method: "each"}, nil
		}
		collected := []object.RubyObject{}
		// Yielder is a Proc that appends each call's first arg to
		// the collected slice. mri's Yielder responds to both <<
		// and yield -- but the bouncy idiom passes the yielder as
		// a block (&yielder) so it gets invoked by .call. our
		// goBlockMarker captures the closure.
		yielder := procFromGoBlock(&goBlockMarker{fn: func(a []object.RubyObject) (object.RubyObject, error) {
			if len(a) == 1 {
				collected = append(collected, a[0])
			} else if len(a) > 1 {
				collected = append(collected, object.NewArray(a...))
			}
			return object.NIL, nil
		}})
		// Run the user's block with yielder as its single argument.
		if be, ok := blkAny.(*ast.BlockExpression); ok && be != nil {
			if _, err := invokeBlock(env, be, []object.RubyObject{yielder}); err != nil {
				return nil, err
			}
		} else if bm, ok := blkAny.(*goBlockMarker); ok && bm != nil {
			if _, err := bm.fn([]object.RubyObject{yielder}); err != nil {
				return nil, err
			}
		}
		return &object.Enumerator{Receiver: object.NewArray(collected...), Method: "each"}, nil
	}
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

// enumeratorReduce implements Enumerator#reduce(init=nil) { |acc, x| ... }.
// The block receives the running accumulator and each element of the
// underlying Receiver array. With no init arg, the first element seeds.
func enumeratorReduce(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
	enum, ok := recv.(*object.Enumerator)
	if !ok {
		return nil, errorf("evaluator: Enumerator#reduce on non-Enumerator %T", recv)
	}
	arr, ok := enum.Receiver.(*object.Array)
	if !ok {
		return nil, errorf("evaluator: Enumerator#reduce: receiver %T not iterable", enum.Receiver)
	}
	var acc object.RubyObject
	start := 0
	if len(args) >= 1 {
		acc = args[0]
	} else {
		if len(arr.Elements) == 0 {
			return object.NIL, nil
		}
		acc = arr.Elements[0]
		start = 1
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
