package evaluator

import (
	"github.com/lczyk/goruby/ast"
	"github.com/lczyk/goruby/object"
)

// blockCallback is the invocation shape callMethodWithBlock uses to
// stay agnostic of whether the "block" came from a literal `{ ... }`
// or from a Proc (e.g. `&:sym` / `&blk`).
type blockCallback func(args []object.RubyObject) (object.RubyObject, error)

// callMethodWithBlock dispatches receiver methods that take a block.
// Used for `obj.method { ... }`, `obj.method(&proc)`, and `obj.method(&:sym)`.
// Falls through to plain callMethod when the receiver / name pair has
// no block-aware implementation.
func callMethodWithBlock(env *object.Environment, recv object.RubyObject, name string, args []object.RubyObject, blk *ast.BlockExpression) (object.RubyObject, error) {
	cb := func(a []object.RubyObject) (object.RubyObject, error) {
		return invokeBlock(env, blk, a)
	}
	return callMethodWithCallback(env, recv, name, args, cb, blk)
}

// callMethodWithProc routes a call whose block came from a `&proc`
// (or `&:sym`) capture rather than a literal block, sharing the same
// dispatch table as the literal-block path.
func callMethodWithProc(env *object.Environment, recv object.RubyObject, name string, args []object.RubyObject, p *object.Proc) (object.RubyObject, error) {
	cb := func(a []object.RubyObject) (object.RubyObject, error) {
		return invokeProc(env, p, a)
	}
	return callMethodWithCallback(env, recv, name, args, cb, nil)
}

// callMethodWithCallback is the shared dispatch path. blk is the
// AST-level BlockExpression when available (some callees that pass the
// block on to user methods need it); cb is the universal invocation
// shim.
func callMethodWithCallback(env *object.Environment, recv object.RubyObject, name string, args []object.RubyObject, cb blockCallback, blk *ast.BlockExpression) (object.RubyObject, error) {
	return callMethodWithBlockImpl(env, recv, name, args, cb, blk)
}

// iterStep runs one iteration step via the block callback. Translates
// nextSignal into a normal value-returning step and breakSignal into a
// stop signal. Bubbles other errors verbatim.
func iterStep(invoke blockCallback, args []object.RubyObject) (val object.RubyObject, stop bool, err error) {
	val, err = invoke(args)
	if err == nil {
		return val, false, nil
	}
	if ns, ok := err.(*nextSignal); ok {
		v := ns.Value
		if v == nil {
			v = object.NIL
		}
		return v, false, nil
	}
	if bs, ok := err.(*breakSignal); ok {
		v := bs.Value
		if v == nil {
			v = object.NIL
		}
		return v, true, nil
	}
	return nil, false, err
}

func callMethodWithBlockImpl(env *object.Environment, recv object.RubyObject, name string, args []object.RubyObject, invoke blockCallback, blk *ast.BlockExpression) (object.RubyObject, error) {
	// Universal block-taking methods (apply to any receiver).
	switch name {
	case "then", "yield_self":
		_ = args
		return invoke([]object.RubyObject{recv})
	case "tap":
		if _, err := invoke([]object.RubyObject{recv}); err != nil {
			return nil, err
		}
		return recv, nil
	case "send", "__send__", "public_send":
		if len(args) < 1 {
			return nil, errorf("evaluator: send needs a method name")
		}
		mname, ok := symbolOrString(env, args[0])
		if !ok {
			return nil, errorf("evaluator: send: method name must be Symbol or String")
		}
		return callMethodWithBlockImpl(env, recv, mname, args[1:], invoke, blk)
	}
	if cls, ok := recv.(*object.Class); ok {
		if cls.Name == "Proc" && name == "new" {
			return procFromBlock(env, blk), nil
		}
		if cls.Name == "Hash" && name == "new" {
			h := object.NewHash()
			h.DefaultBlock = procFromBlock(env, blk)
			return h, nil
		}
		if cls.Name == "Array" && name == "new" {
			if len(args) != 1 {
				return nil, errorf("evaluator: Array.new { ... } expects 1 size arg")
			}
			n, ok := args[0].(*object.Integer)
			if !ok {
				return nil, errorf("evaluator: Array.new size must be Integer")
			}
			if n.Value < 0 {
				return nil, errorf("evaluator: ArgumentError: negative array size")
			}
			out := make([]object.RubyObject, 0, n.Value)
			for i := int64(0); i < n.Value; i++ {
				v, stop, err := iterStep(invoke, []object.RubyObject{object.NewInteger(i)})
				if err != nil {
					return nil, err
				}
				if stop {
					return object.NewArray(out...), nil
				}
				out = append(out, v)
			}
			return object.NewArray(out...), nil
		}
		// User-class .new { ... }: instantiate, then invoke
		// initialize (when defined) with the block bound for `&blk`
		// capture and `block_given?` / `yield`.
		if name == "new" {
			inst := object.NewInstance(cls)
			if m, found := cls.LookupMethod("initialize"); found {
				if um, ok := m.(*object.UserMethod); ok {
					if _, err := invokeMethodOnWithBlock(env, inst, um, args, blk); err != nil {
						return nil, err
					}
				}
			}
			return inst, nil
		}
		if m, found := cls.LookupClassMethod(name); found {
			if um, ok := m.(*object.UserMethod); ok {
				return invokeMethodOnWithBlock(env, cls, um, args, blk)
			}
		}
	}
	if inst, ok := recv.(*object.Instance); ok {
		if m, found := dispatchClass(env, inst).LookupMethod(name); found {
			if um, ok := m.(*object.UserMethod); ok {
				return invokeMethodOnWithBlock(env, inst, um, args, blk)
			}
		}
		if v, handled, err := callEnumerableBlock(env, inst, name, args, invoke); handled {
			return v, err
		}
		// method_missing with block.
		if mm, found := dispatchClass(env, inst).LookupMethod("method_missing"); found {
			if um, ok := mm.(*object.UserMethod); ok {
				mmArgs := make([]object.RubyObject, 0, 1+len(args))
				mmArgs = append(mmArgs, env.Symbols().Intern(name))
				mmArgs = append(mmArgs, args...)
				return invokeMethodOnWithBlock(env, inst, um, mmArgs, blk)
			}
		}
	}
	if r, ok := recv.(*object.Range); ok {
		switch name {
		case "each":
			elems, err := rangeToSlice(env, r)
			if err != nil {
				return nil, err
			}
			for _, e := range elems {
				_, stop, err := iterStep(invoke, []object.RubyObject{e})
				if err != nil {
					return nil, err
				}
				if stop {
					return r, nil
				}
			}
			return r, nil
		case "map", "collect":
			elems, err := rangeToSlice(env, r)
			if err != nil {
				return nil, err
			}
			out := []object.RubyObject{}
			for _, e := range elems {
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
		case "select", "filter", "reject", "find", "detect", "all?", "any?", "none?", "count", "group_by", "partition", "min_by", "max_by", "take_while", "drop_while", "each_slice", "each_cons", "reduce", "inject", "chunk_while", "slice_when", "each_with_object", "each_with_index", "sort", "sort_by":
			elems, err := rangeToSlice(env, r)
			if err != nil {
				return nil, err
			}
			return callMethodWithBlockImpl(env, object.NewArray(elems...), name, args, invoke, blk)
		}
	}
	if s, ok := stringText(env, recv); ok {
		switch name {
		case "each_char":
			for _, ch := range []rune(s) {
				_, stop, err := iterStep(invoke, []object.RubyObject{object.NewString(string(ch))})
				if err != nil {
					return nil, err
				}
				if stop {
					return recv, nil
				}
			}
			return recv, nil
		case "each_byte":
			for i := 0; i < len(s); i++ {
				_, stop, err := iterStep(invoke, []object.RubyObject{object.NewInteger(int64(s[i]))})
				if err != nil {
					return nil, err
				}
				if stop {
					return recv, nil
				}
			}
			return recv, nil
		case "each_line":
			parts := splitLinesKeepNL(s)
			for _, p := range parts {
				_, stop, err := iterStep(invoke, []object.RubyObject{object.NewString(p)})
				if err != nil {
					return nil, err
				}
				if stop {
					return recv, nil
				}
			}
			return recv, nil
		case "gsub":
			if len(args) != 1 {
				return nil, errorf("evaluator: String#gsub { ... } expects 1 pattern arg, got %d", len(args))
			}
			return stringGsubBlock(env, s, args[0], invoke)
		case "sub":
			if len(args) != 1 {
				return nil, errorf("evaluator: String#sub { ... } expects 1 pattern arg, got %d", len(args))
			}
			return stringSubBlock(env, s, args[0], invoke)
		}
	}
	if h, ok := recv.(*object.Hash); ok {
		switch name {
		case "each", "each_pair":
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
		case "each_key":
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
		case "each_value":
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
		case "map", "collect":
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
		case "select", "filter":
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
		case "reject":
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
		case "find", "detect":
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
		case "any?":
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
		case "all?":
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
		case "count":
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
		case "transform_values":
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
		case "sum":
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
		case "min_by", "max_by":
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
				if (name == "min_by" && c < 0) || (name == "max_by" && c > 0) {
					bestKey = k
					bestVal = e
				}
			}
			if bestKey == nil {
				return object.NIL, nil
			}
			return object.NewArray(bestVal.Key, bestVal.Value), nil
		case "merge":
			// Block form: yields (key, v_self, v_other) on conflict to
			// pick the merged value.
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
		case "transform_keys":
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
		case "sort_by":
			type keyed struct {
				key object.RubyObject
				ent object.HashEntry
			}
			keyed_entries := make([]keyed, 0, len(h.Entries))
			for _, e := range h.Entries {
				v, stop, err := iterStep(invoke, []object.RubyObject{e.Key, e.Value})
				if err != nil {
					return nil, err
				}
				if stop {
					return v, nil
				}
				keyed_entries = append(keyed_entries, keyed{key: v, ent: e})
			}
			sortStable(len(keyed_entries), func(i, j int) bool {
				c, _ := compareObjects(keyed_entries[i].key, keyed_entries[j].key)
				return c < 0
			}, func(i, j int) {
				keyed_entries[i], keyed_entries[j] = keyed_entries[j], keyed_entries[i]
			})
			out := make([]object.RubyObject, len(keyed_entries))
			for i, ke := range keyed_entries {
				out[i] = object.NewArray(ke.ent.Key, ke.ent.Value)
			}
			return object.NewArray(out...), nil
		}
	}
	if f, ok := recv.(*object.Float); ok && name == "step" {
		if len(args) < 1 || len(args) > 2 {
			return nil, errorf("evaluator: Float#step expects 1..2 args, got %d", len(args))
		}
		limit, err := toFloatValue(args[0])
		if err != nil {
			return nil, err
		}
		step := 1.0
		if len(args) == 2 {
			s, err := toFloatValue(args[1])
			if err != nil {
				return nil, err
			}
			step = s
		}
		if step == 0 {
			return nil, errorf("evaluator: ArgumentError: step can't be 0")
		}
		cond := func(v float64) bool { return v <= limit }
		if step < 0 {
			cond = func(v float64) bool { return v >= limit }
		}
		for v := f.Value; cond(v); v += step {
			_, stop, err := iterStep(invoke, []object.RubyObject{object.NewFloat(v)})
			if err != nil {
				return nil, err
			}
			if stop {
				return f, nil
			}
		}
		return f, nil
	}
	if i, ok := recv.(*object.Integer); ok {
		switch name {
		case "times":
			for k := int64(0); k < i.Value; k++ {
				_, stop, err := iterStep(invoke, []object.RubyObject{object.NewInteger(k)})
				if err != nil {
					return nil, err
				}
				if stop {
					return i, nil
				}
			}
			return i, nil
		case "upto":
			if len(args) != 1 {
				return nil, errorf("evaluator: Integer#upto expects 1 arg, got %d", len(args))
			}
			to, ok := args[0].(*object.Integer)
			if !ok {
				return nil, errorf("evaluator: Integer#upto needs Integer arg")
			}
			for k := i.Value; k <= to.Value; k++ {
				_, stop, err := iterStep(invoke, []object.RubyObject{object.NewInteger(k)})
				if err != nil {
					return nil, err
				}
				if stop {
					return i, nil
				}
			}
			return i, nil
		case "downto":
			if len(args) != 1 {
				return nil, errorf("evaluator: Integer#downto expects 1 arg, got %d", len(args))
			}
			to, ok := args[0].(*object.Integer)
			if !ok {
				return nil, errorf("evaluator: Integer#downto needs Integer arg")
			}
			for k := i.Value; k >= to.Value; k-- {
				_, stop, err := iterStep(invoke, []object.RubyObject{object.NewInteger(k)})
				if err != nil {
					return nil, err
				}
				if stop {
					return i, nil
				}
			}
			return i, nil
		case "step":
			if len(args) < 1 || len(args) > 2 {
				return nil, errorf("evaluator: Integer#step expects 1..2 args, got %d", len(args))
			}
			limit, ok := args[0].(*object.Integer)
			if !ok {
				return nil, errorf("evaluator: Integer#step needs Integer limit")
			}
			step := int64(1)
			if len(args) == 2 {
				sa, ok := args[1].(*object.Integer)
				if !ok {
					return nil, errorf("evaluator: Integer#step needs Integer step")
				}
				step = sa.Value
			}
			if step == 0 {
				return nil, errorf("evaluator: ArgumentError: step can't be 0")
			}
			cond := func(k int64) bool { return k <= limit.Value }
			if step < 0 {
				cond = func(k int64) bool { return k >= limit.Value }
			}
			for k := i.Value; cond(k); k += step {
				_, stop, err := iterStep(invoke, []object.RubyObject{object.NewInteger(k)})
				if err != nil {
					return nil, err
				}
				if stop {
					return i, nil
				}
			}
			return i, nil
		}
	}
	if arr, ok := recv.(*object.Array); ok {
		switch name {
		case "map", "collect":
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
		case "flat_map", "collect_concat":
			out := []object.RubyObject{}
			for _, e := range arr.Elements {
				v, stop, err := iterStep(invoke, []object.RubyObject{e})
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
		case "select", "filter":
			out := make([]object.RubyObject, 0, len(arr.Elements))
			for _, e := range arr.Elements {
				v, stop, err := iterStep(invoke, []object.RubyObject{e})
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
			for _, e := range arr.Elements {
				v, stop, err := iterStep(invoke, []object.RubyObject{e})
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
		case "each":
			for _, e := range arr.Elements {
				_, stop, err := iterStep(invoke, []object.RubyObject{e})
				if err != nil {
					return nil, err
				}
				if stop {
					return arr, nil
				}
			}
			return arr, nil
		case "each_with_index":
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
		case "zip":
			// Block form yields each tuple and returns nil.
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
		case "each_with_object":
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
		case "each_slice":
			if len(args) != 1 {
				return nil, errorf("evaluator: each_slice expects 1 arg")
			}
			n, ok := args[0].(*object.Integer)
			if !ok || n.Value <= 0 {
				return nil, errorf("evaluator: each_slice needs positive Integer")
			}
			step := int(n.Value)
			for i := 0; i < len(arr.Elements); i += step {
				end := i + step
				if end > len(arr.Elements) {
					end = len(arr.Elements)
				}
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
		case "each_cons":
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
		case "reduce", "inject":
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
		case "count":
			n := 0
			for _, e := range arr.Elements {
				v, stop, err := iterStep(invoke, []object.RubyObject{e})
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
		case "sort":
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
		case "sort_by":
			keys := make([]object.RubyObject, len(arr.Elements))
			vals := make([]object.RubyObject, len(arr.Elements))
			copy(vals, arr.Elements)
			for i, e := range arr.Elements {
				v, stop, err := iterStep(invoke, []object.RubyObject{e})
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
		case "find", "detect":
			for _, e := range arr.Elements {
				v, stop, err := iterStep(invoke, []object.RubyObject{e})
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
		case "group_by":
			groups := []object.HashEntry{}
			for _, e := range arr.Elements {
				k, stop, err := iterStep(invoke, []object.RubyObject{e})
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
		case "partition":
			truthyOut := []object.RubyObject{}
			falsyOut := []object.RubyObject{}
			for _, e := range arr.Elements {
				v, stop, err := iterStep(invoke, []object.RubyObject{e})
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
		case "min_by":
			var bestVal object.RubyObject
			var bestKey object.RubyObject
			for _, e := range arr.Elements {
				k, stop, err := iterStep(invoke, []object.RubyObject{e})
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
		case "minmax_by":
			var loVal, hiVal object.RubyObject
			var loKey, hiKey object.RubyObject
			for _, e := range arr.Elements {
				k, stop, err := iterStep(invoke, []object.RubyObject{e})
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
		case "max_by":
			var bestVal object.RubyObject
			var bestKey object.RubyObject
			for _, e := range arr.Elements {
				k, stop, err := iterStep(invoke, []object.RubyObject{e})
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
		case "take_while":
			out := []object.RubyObject{}
			for _, e := range arr.Elements {
				v, stop, err := iterStep(invoke, []object.RubyObject{e})
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
		case "drop_while":
			started := false
			out := []object.RubyObject{}
			for _, e := range arr.Elements {
				if started {
					out = append(out, e)
					continue
				}
				v, stop, err := iterStep(invoke, []object.RubyObject{e})
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
		case "chunk_while":
			// Yields adjacent pairs (prev, cur); starts a new chunk
			// when the block returns falsy.
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
		case "slice_when":
			// Inverse of chunk_while: split when the block returns truthy.
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
		case "any?":
			for _, e := range arr.Elements {
				v, stop, err := iterStep(invoke, []object.RubyObject{e})
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
		case "all?":
			for _, e := range arr.Elements {
				v, stop, err := iterStep(invoke, []object.RubyObject{e})
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
		case "none?":
			for _, e := range arr.Elements {
				v, stop, err := iterStep(invoke, []object.RubyObject{e})
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
		}
	}
	return nil, errorf("evaluator: %T#%s with block not yet supported", recv, name)
}

// invokeBlock evaluates blk with the given positional args bound to its
// parameters. Uses an enclosed env so block-local writes don't leak,
// while still seeing the caller's locals (matching ruby block scoping).
func invokeBlock(env *object.Environment, blk *ast.BlockExpression, args []object.RubyObject) (object.RubyObject, error) {
	inner := object.NewEnclosedEnvironment(env)
	// Auto-splat: ruby blocks destructure a single Array arg when the
	// block declares multiple positional params -- the idiom that lets
	// `{|k, v| ...}` work for `each_with_index` / `Hash#each` / etc.
	if len(args) == 1 && len(blk.Parameters) > 1 {
		if arr, ok := args[0].(*object.Array); ok {
			args = arr.Elements
		}
	}
	if len(blk.Parameters) == 0 {
		// Bind numbered block params (`_1`, `_2`, ...) -- ruby 2.7+ --
		// and the anonymous `it` (ruby 3.4+) when the block declares
		// no explicit params. Cheap and side-effect-free if the block
		// doesn't use them.
		if len(args) > 0 {
			inner.Set("it", args[0])
		}
		for i, a := range args {
			inner.Set(numberedParamName(i+1), a)
		}
	} else {
		if err := bindParams(inner, blk.Parameters, args); err != nil {
			return nil, err
		}
	}
	return evalBlockStatement(inner, blk.Body)
}

func numberedParamName(i int) string {
	switch i {
	case 1:
		return "_1"
	case 2:
		return "_2"
	case 3:
		return "_3"
	case 4:
		return "_4"
	case 5:
		return "_5"
	case 6:
		return "_6"
	case 7:
		return "_7"
	case 8:
		return "_8"
	case 9:
		return "_9"
	}
	return ""
}

func evalYield(env *object.Environment, n *ast.YieldExpression) (object.RubyObject, error) {
	blkAny := env.EnclosingBlock()
	if blkAny == nil {
		return nil, errorf("evaluator: LocalJumpError: no block given (yield)")
	}
	args, err := evalExpressions(env, n.Arguments)
	if err != nil {
		return nil, err
	}
	switch blk := blkAny.(type) {
	case *ast.BlockExpression:
		return invokeBlock(env, blk, args)
	case *goBlockMarker:
		return blk.fn(args)
	}
	return nil, errorf("evaluator: unexpected block payload %T", blkAny)
}
