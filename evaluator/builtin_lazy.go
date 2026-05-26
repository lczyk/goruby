package evaluator

import (
	"math"

	"github.com/lczyk/goruby/ast"
	"github.com/lczyk/goruby/object"
)

// bootstrapLazyClass wires the LazyEnumerator dispatch class. Covers
// the map / select / reject / take / take_while / drop / drop_while /
// filter_map chain ops plus the forcers first / to_a / force / each.
//
// Lazy semantics: each chain op returns a fresh LazyEnumerator
// extending the original with one more LazyOp. Forcing drives the
// pipeline -- pulls one value at a time from the source, threads it
// through every op, emits on success / skips on filter / stops on
// take limit reached.
func bootstrapLazyClass(env *object.Environment) *object.Class {
	c := object.LazyClass
	if _, ok := env.Get("Enumerator::Lazy"); ok {
		return c
	}

	// Chain op: append one LazyOp and return a new LazyEnumerator
	// sharing the source. The source Next closure is mutable but
	// callers consume a Lazy at most once, matching MRI's
	// effectively-one-shot semantics for non-rewound Lazy.
	chain := func(kind string) func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, blk *ast.BlockExpression) (object.RubyObject, error) {
		return func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, blk *ast.BlockExpression) (object.RubyObject, error) {
			lz, ok := recv.(*object.LazyEnumerator)
			if !ok {
				return nil, errorf("evaluator: Lazy#%s on non-Lazy %T", kind, recv)
			}
			op := object.LazyOp{Kind: kind, Fn: invoke, Block: blk}
			return &object.LazyEnumerator{Next: lz.Next, Ops: append(append([]object.LazyOp{}, lz.Ops...), op)}, nil
		}
	}
	addBlockMethod(c, "map", chain("map"))
	c.Methods["collect"] = c.Methods["map"]
	addBlockMethod(c, "select", chain("select"))
	c.Methods["filter"] = c.Methods["select"]
	c.Methods["find_all"] = c.Methods["select"]
	addBlockMethod(c, "reject", chain("reject"))
	addBlockMethod(c, "take_while", chain("take_while"))
	addBlockMethod(c, "drop_while", chain("drop_while"))
	addBlockMethod(c, "filter_map", chain("filter_map"))
	addBlockMethod(c, "flat_map", chain("flat_map"))
	c.Methods["collect_concat"] = c.Methods["flat_map"]

	// take(n) / drop(n): arg-only ops, no block.
	addLazyCountOp := func(kind string) {
		c.AddMethod(kind, &object.BuiltinMethod{
			Name: kind,
			Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, _ any) (object.RubyObject, error) {
				lz, ok := recv.(*object.LazyEnumerator)
				if !ok {
					return nil, errorf("evaluator: Lazy#%s on non-Lazy %T", kind, recv)
				}
				if len(args) != 1 {
					return raiseBuiltin(env, "ArgumentError", "expected 1 arg")
				}
				n, ok := args[0].(*object.Integer)
				if !ok {
					return nil, errorf("evaluator: Lazy#%s needs Integer", kind)
				}
				op := object.LazyOp{Kind: kind, N: n.Value}
				return &object.LazyEnumerator{Next: lz.Next, Ops: append(append([]object.LazyOp{}, lz.Ops...), op)}, nil
			},
		})
	}
	addLazyCountOp("take")
	addLazyCountOp("drop")

	// Forcers.
	c.AddMethod("first", &object.BuiltinMethod{
		Name: "first",
		Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, _ any) (object.RubyObject, error) {
			lz, ok := recv.(*object.LazyEnumerator)
			if !ok {
				return nil, errorf("evaluator: Lazy#first on non-Lazy %T", recv)
			}
			n := int64(-1)
			if len(args) == 1 {
				ni, ok := args[0].(*object.Integer)
				if !ok {
					return nil, errorf("evaluator: Lazy#first needs Integer")
				}
				n = ni.Value
			}
			out, err := forceLazy(env, lz, n)
			if err != nil {
				return nil, err
			}
			if n < 0 {
				if len(out) == 0 {
					return object.NIL, nil
				}
				return out[0], nil
			}
			return object.NewArray(out...), nil
		},
	})
	toAArr := func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, _ any) (object.RubyObject, error) {
		lz, ok := recv.(*object.LazyEnumerator)
		if !ok {
			return nil, errorf("evaluator: Lazy#to_a on non-Lazy %T", recv)
		}
		out, err := forceLazy(env, lz, -1)
		if err != nil {
			return nil, err
		}
		return object.NewArray(out...), nil
	}
	c.AddMethod("to_a", &object.BuiltinMethod{Name: "to_a", Fn: toAArr})
	c.AddMethod("force", &object.BuiltinMethod{Name: "force", Fn: toAArr})

	addBlockMethod(c, "each", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
		lz, ok := recv.(*object.LazyEnumerator)
		if !ok {
			return nil, errorf("evaluator: Lazy#each on non-Lazy %T", recv)
		}
		out, err := forceLazy(env, lz, -1)
		if err != nil {
			return nil, err
		}
		for _, v := range out {
			_, stop, err := yieldOne(invoke, v)
			if err != nil {
				return nil, err
			}
			if stop {
				return recv, nil
			}
		}
		return recv, nil
	})

	env.SetGlobal("Enumerator::Lazy", c)
	return c
}

// forceLazy drives a LazyEnumerator: pulls values from its source,
// threads each through every op in order. If limit >= 0, stops after
// emitting that many values. -1 means run until source exhausted (or
// a take op caps it).
//
// Stateful op behaviour:
//   - take(n): emit at most n total, then signal stop.
//   - drop(n): skip the first n that reach this op, then pass.
//   - take_while: pass until block returns false; then stop.
//   - drop_while: skip while block returns true; switch to pass thereafter.
//
// All state is driver-local so a re-force walks the same chain afresh
// (matching MRI's pipeline-rerun semantics).
func forceLazy(env *object.Environment, lz *object.LazyEnumerator, limit int64) ([]object.RubyObject, error) {
	out := []object.RubyObject{}
	// Per-op state slots, indexed by op position.
	takeRemaining := make([]int64, len(lz.Ops))
	dropRemaining := make([]int64, len(lz.Ops))
	dropWhileDone := make([]bool, len(lz.Ops))
	takeWhileStopped := make([]bool, len(lz.Ops))
	for i, op := range lz.Ops {
		switch op.Kind {
		case "take":
			takeRemaining[i] = op.N
		case "drop":
			dropRemaining[i] = op.N
		}
	}

source:
	for {
		v, more := lz.Next()
		if !more {
			break
		}
		// Thread v through every op. `skip` short-circuits the
		// remaining pipeline for this value (filter rejected, drop
		// consumed, etc.). `stopAll` ends the whole force.
		skip := false
		stopAll := false
		flatVals := []object.RubyObject{v}
		// flatVals lets flat_map / filter_map fan out the value.
		// Start with the single source element; ops update it.
		for opIdx, op := range lz.Ops {
			if skip || stopAll {
				break
			}
			switch op.Kind {
			case "map":
				cb := op.Fn.(blockCallback)
				newVals := make([]object.RubyObject, 0, len(flatVals))
				for _, fv := range flatVals {
					nv, stop, err := yieldOne(cb, fv)
					if err != nil {
						return nil, err
					}
					if stop {
						stopAll = true
						break
					}
					newVals = append(newVals, nv)
				}
				flatVals = newVals
			case "select":
				cb := op.Fn.(blockCallback)
				newVals := make([]object.RubyObject, 0, len(flatVals))
				for _, fv := range flatVals {
					nv, stop, err := yieldOne(cb, fv)
					if err != nil {
						return nil, err
					}
					if stop {
						stopAll = true
						break
					}
					if truthy(nv) {
						newVals = append(newVals, fv)
					}
				}
				flatVals = newVals
			case "reject":
				cb := op.Fn.(blockCallback)
				newVals := make([]object.RubyObject, 0, len(flatVals))
				for _, fv := range flatVals {
					nv, stop, err := yieldOne(cb, fv)
					if err != nil {
						return nil, err
					}
					if stop {
						stopAll = true
						break
					}
					if !truthy(nv) {
						newVals = append(newVals, fv)
					}
				}
				flatVals = newVals
			case "filter_map":
				cb := op.Fn.(blockCallback)
				newVals := make([]object.RubyObject, 0, len(flatVals))
				for _, fv := range flatVals {
					nv, stop, err := yieldOne(cb, fv)
					if err != nil {
						return nil, err
					}
					if stop {
						stopAll = true
						break
					}
					if truthy(nv) {
						newVals = append(newVals, nv)
					}
				}
				flatVals = newVals
			case "flat_map":
				cb := op.Fn.(blockCallback)
				newVals := make([]object.RubyObject, 0, len(flatVals))
				for _, fv := range flatVals {
					nv, stop, err := yieldOne(cb, fv)
					if err != nil {
						return nil, err
					}
					if stop {
						stopAll = true
						break
					}
					if arr, ok := nv.(*object.Array); ok {
						newVals = append(newVals, arr.Elements...)
					} else {
						newVals = append(newVals, nv)
					}
				}
				flatVals = newVals
			case "take_while":
				cb := op.Fn.(blockCallback)
				if takeWhileStopped[opIdx] {
					stopAll = true
					break
				}
				newVals := make([]object.RubyObject, 0, len(flatVals))
				for _, fv := range flatVals {
					nv, stop, err := yieldOne(cb, fv)
					if err != nil {
						return nil, err
					}
					if stop {
						stopAll = true
						break
					}
					if !truthy(nv) {
						takeWhileStopped[opIdx] = true
						stopAll = true
						break
					}
					newVals = append(newVals, fv)
				}
				if !stopAll {
					flatVals = newVals
				}
			case "drop_while":
				cb := op.Fn.(blockCallback)
				if dropWhileDone[opIdx] {
					// pass through unchanged
					break
				}
				newVals := make([]object.RubyObject, 0, len(flatVals))
				for _, fv := range flatVals {
					if dropWhileDone[opIdx] {
						newVals = append(newVals, fv)
						continue
					}
					nv, stop, err := yieldOne(cb, fv)
					if err != nil {
						return nil, err
					}
					if stop {
						stopAll = true
						break
					}
					if !truthy(nv) {
						dropWhileDone[opIdx] = true
						newVals = append(newVals, fv)
					}
				}
				flatVals = newVals
			case "take":
				if takeRemaining[opIdx] <= 0 {
					stopAll = true
					break
				}
				// Cap emission for this stage to remaining.
				if int64(len(flatVals)) > takeRemaining[opIdx] {
					flatVals = flatVals[:takeRemaining[opIdx]]
				}
				takeRemaining[opIdx] -= int64(len(flatVals))
			case "drop":
				if dropRemaining[opIdx] >= int64(len(flatVals)) {
					dropRemaining[opIdx] -= int64(len(flatVals))
					skip = true
					break
				}
				flatVals = flatVals[dropRemaining[opIdx]:]
				dropRemaining[opIdx] = 0
			default:
				return nil, errorf("evaluator: Lazy op %q not implemented", op.Kind)
			}
			if len(flatVals) == 0 && !stopAll {
				skip = true
			}
		}
		if stopAll {
			break source
		}
		if skip {
			continue
		}
		for _, fv := range flatVals {
			out = append(out, fv)
			if limit >= 0 && int64(len(out)) >= limit {
				break source
			}
		}
	}
	return out, nil
}

// lazyFromArray builds a LazyEnumerator that pulls elements from a
// snapshot of arr's elements. Snapshotting protects against the
// caller mutating arr mid-iteration (matches MRI's Array#lazy).
func lazyFromArray(arr *object.Array) *object.LazyEnumerator {
	snap := make([]object.RubyObject, len(arr.Elements))
	copy(snap, arr.Elements)
	idx := 0
	return &object.LazyEnumerator{
		Next: func() (object.RubyObject, bool) {
			if idx >= len(snap) {
				return nil, false
			}
			v := snap[idx]
			idx++
			return v, true
		},
	}
}

// lazyFromRange builds a LazyEnumerator over an Integer-bounded
// Range. End may be Float::INFINITY (math.Inf(1)) for an unbounded
// upper bound -- the Next closure then never reports exhausted, and
// callers must terminate the chain with a take / take_while / first.
func lazyFromRange(rng *object.Range) (*object.LazyEnumerator, error) {
	bi, ok := rng.Begin.(*object.Integer)
	if !ok {
		return nil, errorf("evaluator: Lazy: Range#lazy needs Integer begin, got %T", rng.Begin)
	}
	cur := bi.Value
	infinite := false
	end := int64(0)
	switch e := rng.End.(type) {
	case *object.Integer:
		end = e.Value
	case *object.Float:
		if math.IsInf(e.Value, 1) {
			infinite = true
		} else {
			end = int64(e.Value)
		}
	case *object.Nil:
		infinite = true
	default:
		return nil, errorf("evaluator: Lazy: Range#lazy needs Integer end, got %T", rng.End)
	}
	exclusive := rng.Exclusive
	return &object.LazyEnumerator{
		Next: func() (object.RubyObject, bool) {
			if !infinite {
				if exclusive && cur >= end {
					return nil, false
				}
				if !exclusive && cur > end {
					return nil, false
				}
			}
			v := object.NewInteger(cur)
			cur++
			return v, true
		},
	}, nil
}
