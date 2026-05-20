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

// callEnumerable handles the common Enumerable methods on an Instance
// whose class includes Enumerable. Returns handled=false otherwise.
// Derives all behaviour from the user-defined `each` method.
func callEnumerable(env *object.Environment, inst *object.Instance, name string, args []object.RubyObject) (object.RubyObject, bool, error) {
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

	// Methods that materialise then dispatch to Array.
	switch name {
	case "sort", "uniq", "reverse", "tally", "sum", "join", "flatten", "zip", "take", "drop", "reduce", "inject":
		out := []object.RubyObject{}
		if _, err := runEach(func(a []object.RubyObject) (object.RubyObject, error) {
			out = append(out, joinYieldArgs(a))
			return object.NIL, nil
		}); err != nil {
			return nil, true, err
		}
		v, err := callMethod(env, object.NewArray(out...), name, args)
		return v, true, err
	}

	switch name {
	case "to_a", "entries":
		out := []object.RubyObject{}
		_, err := runEach(func(a []object.RubyObject) (object.RubyObject, error) {
			out = append(out, joinYieldArgs(a))
			return object.NIL, nil
		})
		if err != nil {
			return nil, true, err
		}
		return object.NewArray(out...), true, nil
	case "count":
		n := 0
		_, err := runEach(func(a []object.RubyObject) (object.RubyObject, error) {
			n++
			return object.NIL, nil
		})
		if err != nil {
			return nil, true, err
		}
		return object.NewInteger(int64(n)), true, nil
	case "include?":
		if len(args) != 1 {
			return nil, true, errorf("evaluator: include? expects 1 arg")
		}
		found := false
		_, err := runEach(func(a []object.RubyObject) (object.RubyObject, error) {
			if rubyEqual(joinYieldArgs(a), args[0]) {
				found = true
			}
			return object.NIL, nil
		})
		if err != nil {
			return nil, true, err
		}
		return object.BooleanOf(found), true, nil
	case "first":
		var first object.RubyObject = object.NIL
		got := false
		_, err := runEach(func(a []object.RubyObject) (object.RubyObject, error) {
			if got {
				return object.NIL, &breakSignal{Value: first}
			}
			first = joinYieldArgs(a)
			got = true
			return object.NIL, nil
		})
		if err != nil {
			return nil, true, err
		}
		return first, true, nil
	case "min":
		var best object.RubyObject
		_, err := runEach(func(a []object.RubyObject) (object.RubyObject, error) {
			v := joinYieldArgs(a)
			if best == nil {
				best = v
				return object.NIL, nil
			}
			c, ok := compareObjectsEnv(env, v, best)
			if !ok {
				return nil, errorf("evaluator: comparison failed in min")
			}
			if c < 0 {
				best = v
			}
			return object.NIL, nil
		})
		if err != nil {
			return nil, true, err
		}
		if best == nil {
			return object.NIL, true, nil
		}
		return best, true, nil
	case "max":
		var best object.RubyObject
		_, err := runEach(func(a []object.RubyObject) (object.RubyObject, error) {
			v := joinYieldArgs(a)
			if best == nil {
				best = v
				return object.NIL, nil
			}
			c, ok := compareObjectsEnv(env, v, best)
			if !ok {
				return nil, errorf("evaluator: comparison failed in max")
			}
			if c > 0 {
				best = v
			}
			return object.NIL, nil
		})
		if err != nil {
			return nil, true, err
		}
		if best == nil {
			return object.NIL, true, nil
		}
		return best, true, nil
	}
	return nil, false, nil
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
