package evaluator

import (
	"github.com/lczyk/goruby/object"
)

// instanceIncludesEnumerable reports whether inst's class transitively
// includes the Enumerable module (or `each` is the only thing defined
// and the user clearly meant it -- but we stick to explicit include
// for now).
func instanceIncludesEnumerable(env *object.Environment, inst *object.Instance) bool {
	enum, ok := env.Get("Enumerable")
	if !ok {
		return false
	}
	mod, ok := enum.(*object.Class)
	if !ok {
		return false
	}
	return inst.C.IsAncestor(mod)
}

// callEnumerableBlock handles the block-taking Enumerable derivations
// for Instances whose class includes Enumerable. Falls through when
// the class doesn't include Enumerable or the method isn't covered.
func callEnumerableBlock(env *object.Environment, inst *object.Instance, name string, args []object.RubyObject, invoke blockCallback) (object.RubyObject, bool, error) {
	if !instanceIncludesEnumerable(env, inst) {
		return nil, false, nil
	}
	eachMethod, ok := dispatchClass(env, inst).LookupMethod("each")
	if !ok {
		return nil, false, nil
	}
	eachUM, ok := eachMethod.(*object.UserMethod)
	if !ok {
		return nil, false, nil
	}

	runEach := func(cb func([]object.RubyObject) (object.RubyObject, error)) (object.RubyObject, error) {
		return invokeMethodOn(env, inst, eachUM, nil, &goBlockMarker{fn: cb})
	}

	switch name {
	case "map", "collect":
		out := []object.RubyObject{}
		_, err := runEach(func(a []object.RubyObject) (object.RubyObject, error) {
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
				return bs.Value, true, nil
			}
			return nil, true, err
		}
		return object.NewArray(out...), true, nil
	case "select", "filter":
		out := []object.RubyObject{}
		_, err := runEach(func(a []object.RubyObject) (object.RubyObject, error) {
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
				return bs.Value, true, nil
			}
			return nil, true, err
		}
		return object.NewArray(out...), true, nil
	case "reject":
		out := []object.RubyObject{}
		_, err := runEach(func(a []object.RubyObject) (object.RubyObject, error) {
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
				return bs.Value, true, nil
			}
			return nil, true, err
		}
		return object.NewArray(out...), true, nil
	case "reduce", "inject":
		var acc object.RubyObject
		hasInit := false
		if len(args) == 1 {
			acc = args[0]
			hasInit = true
		}
		_, err := runEach(func(a []object.RubyObject) (object.RubyObject, error) {
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
				return bs.Value, true, nil
			}
			return nil, true, err
		}
		if !hasInit {
			return object.NIL, true, nil
		}
		return acc, true, nil
	case "find", "detect":
		var hit object.RubyObject = object.NIL
		_, err := runEach(func(a []object.RubyObject) (object.RubyObject, error) {
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
				return bs.Value, true, nil
			}
			return nil, true, err
		}
		return hit, true, nil
	}
	return nil, false, nil
}

// joinYieldArgs collapses a yield's args into a single value: empty
// yields -> nil, single -> the value, multiple -> an Array.
func joinYieldArgs(a []object.RubyObject) object.RubyObject {
	switch len(a) {
	case 0:
		return object.NIL
	case 1:
		return a[0]
	}
	return object.NewArray(a...)
}
