package evaluator

import (
	"github.com/lczyk/goruby/object"
)

// Block-form Enumerable derivations. Registered on ObjectClass, the
// same place as the no-block materialise-to-Array variants in
// enumerable_dispatch.go; AddMethod replaces the prior entry. Each Fn
// checks for a block payload: when present, the value is derived from
// the receiver's `each` directly (yielding each item through the
// caller's block); when absent, behaviour falls through to the
// materialise-to-Array path that previously stood alone.

func init() {
	c := object.ObjectClass

	// runEach drives the receiver's `each` user-method with a Go
	// callback wrapped in a goBlockMarker. The cb's return value is
	// returned to `each`; returning a *breakSignal lets us short-circuit.
	runEach := func(env *object.Environment, recv object.RubyObject, name string, cb func([]object.RubyObject) (object.RubyObject, error)) (object.RubyObject, error) {
		inst, eachUM, err := enumGate(env, recv, name)
		if err != nil {
			return nil, err
		}
		return invokeMethodOn(env, inst, eachUM, nil, &goBlockMarker{fn: cb})
	}

	// Enumerable derivations apply only to Instances whose class
	// transitively includes Enumerable. For Instances that lack the
	// include but define method_missing, fall through to it so the
	// method-missing proxy idiom keeps working. Builtin types
	// (Array/Hash/Range/...) never reach this Fn -- their own classes
	// register matching method names that win the LookupMethod walk
	// before ObjectClass is consulted.
	register := func(name string, blockForm func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback) (object.RubyObject, error), plainForm func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error)) {
		c.AddMethod(name, &object.BuiltinMethod{
			Name: name,
			Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, blkAny any) (object.RubyObject, error) {
				if inst, ok := recv.(*object.Instance); ok && !instanceIncludesEnumerable(env, inst) {
					if mm, found := dispatchClass(env, inst).LookupMethod("method_missing"); found {
						mmArgs := make([]object.RubyObject, 0, 1+len(args))
						mmArgs = append(mmArgs, env.Symbols().Intern(name))
						mmArgs = append(mmArgs, args...)
						return mm.Call(env, inst, mmArgs, blkAny)
					}
					return raiseBuiltin(env, "NoMethodError", "undefined method `"+name+"' for instance of "+inst.C.Name)
				}
				if bm, ok := blkAny.(*goBlockMarker); ok && bm != nil {
					return blockForm(env, recv, args, bm.fn)
				}
				return plainForm(env, recv, args)
			},
		})
	}

	// map / collect.
	mapPlain := func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
		// No-block form: materialise via each into an Array (matches
		// MRI's enumerator-returning behaviour by collecting yields).
		out := []object.RubyObject{}
		_, err := runEach(env, recv, "map", func(a []object.RubyObject) (object.RubyObject, error) {
			out = append(out, joinYieldArgs(a))
			return object.NIL, nil
		})
		if err != nil {
			return nil, err
		}
		return object.NewArray(out...), nil
	}
	mapBlock := func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback) (object.RubyObject, error) {
		out := []object.RubyObject{}
		_, err := runEach(env, recv, "map", func(a []object.RubyObject) (object.RubyObject, error) {
			v, stop, err := iterStep(invoke, []object.RubyObject{joinYieldArgs(a)})
			if err != nil {
				return nil, err
			}
			if stop {
				return nil, &breakSignal{Value: v}
			}
			out = append(out, v)
			return object.NIL, nil
		})
		if err != nil {
			if bs, ok := err.(*breakSignal); ok {
				return bs.Value, nil
			}
			return nil, err
		}
		return object.NewArray(out...), nil
	}
	register("map", mapBlock, mapPlain)
	c.Methods["collect"] = c.Methods["map"]

	selectPlain := func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
		return nil, errorf("evaluator: select without a block needs an enumerator (unsupported)")
	}
	selectBlock := func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback) (object.RubyObject, error) {
		out := []object.RubyObject{}
		_, err := runEach(env, recv, "select", func(a []object.RubyObject) (object.RubyObject, error) {
			e := joinYieldArgs(a)
			v, stop, err := iterStep(invoke, []object.RubyObject{e})
			if err != nil {
				return nil, err
			}
			if stop {
				return nil, &breakSignal{Value: v}
			}
			if truthy(v) {
				out = append(out, e)
			}
			return object.NIL, nil
		})
		if err != nil {
			if bs, ok := err.(*breakSignal); ok {
				return bs.Value, nil
			}
			return nil, err
		}
		return object.NewArray(out...), nil
	}
	register("select", selectBlock, selectPlain)
	c.Methods["filter"] = c.Methods["select"]
	c.Methods["find_all"] = c.Methods["select"]

	rejectPlain := func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
		return nil, errorf("evaluator: reject without a block needs an enumerator (unsupported)")
	}
	rejectBlock := func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback) (object.RubyObject, error) {
		out := []object.RubyObject{}
		_, err := runEach(env, recv, "reject", func(a []object.RubyObject) (object.RubyObject, error) {
			e := joinYieldArgs(a)
			v, stop, err := iterStep(invoke, []object.RubyObject{e})
			if err != nil {
				return nil, err
			}
			if stop {
				return nil, &breakSignal{Value: v}
			}
			if !truthy(v) {
				out = append(out, e)
			}
			return object.NIL, nil
		})
		if err != nil {
			if bs, ok := err.(*breakSignal); ok {
				return bs.Value, nil
			}
			return nil, err
		}
		return object.NewArray(out...), nil
	}
	register("reject", rejectBlock, rejectPlain)

	// reduce / inject. Plain form: materialise to Array and re-dispatch
	// (so symbol-arg variants like `reduce(:+)` keep working). Block
	// form: accumulate via each.
	reducePlain := func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
		out := []object.RubyObject{}
		_, err := runEach(env, recv, "reduce", func(a []object.RubyObject) (object.RubyObject, error) {
			out = append(out, joinYieldArgs(a))
			return object.NIL, nil
		})
		if err != nil {
			return nil, err
		}
		return callMethod(env, object.NewArray(out...), "reduce", args)
	}
	reduceBlock := func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback) (object.RubyObject, error) {
		var acc object.RubyObject
		hasInit := false
		if len(args) == 1 {
			acc = args[0]
			hasInit = true
		}
		_, err := runEach(env, recv, "reduce", func(a []object.RubyObject) (object.RubyObject, error) {
			e := joinYieldArgs(a)
			if !hasInit {
				acc = e
				hasInit = true
				return object.NIL, nil
			}
			v, stop, err := iterStep(invoke, []object.RubyObject{acc, e})
			if err != nil {
				return nil, err
			}
			if stop {
				return nil, &breakSignal{Value: v}
			}
			acc = v
			return object.NIL, nil
		})
		if err != nil {
			if bs, ok := err.(*breakSignal); ok {
				return bs.Value, nil
			}
			return nil, err
		}
		if !hasInit {
			return object.NIL, nil
		}
		return acc, nil
	}
	register("reduce", reduceBlock, reducePlain)
	c.Methods["inject"] = c.Methods["reduce"]

	findBlock := func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback) (object.RubyObject, error) {
		var hit object.RubyObject = object.NIL
		_, err := runEach(env, recv, "find", func(a []object.RubyObject) (object.RubyObject, error) {
			e := joinYieldArgs(a)
			v, stop, err := iterStep(invoke, []object.RubyObject{e})
			if err != nil {
				return nil, err
			}
			if stop {
				return nil, &breakSignal{Value: v}
			}
			if truthy(v) {
				hit = e
				return nil, &breakSignal{Value: e}
			}
			return object.NIL, nil
		})
		if err != nil {
			if bs, ok := err.(*breakSignal); ok {
				return bs.Value, nil
			}
			return nil, err
		}
		return hit, nil
	}
	findPlain := func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
		return nil, errorf("evaluator: find without a block needs an enumerator (unsupported)")
	}
	register("find", findBlock, findPlain)
	c.Methods["detect"] = c.Methods["find"]
}

