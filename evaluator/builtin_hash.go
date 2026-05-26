package evaluator

import (
	"github.com/lczyk/goruby/object"
)

func callHashMethod(env *object.Environment, r *object.Hash, name string, args []object.RubyObject) (object.RubyObject, error) {
	switch name {
	case "default":
		if r.Default != nil {
			return r.Default, nil
		}
		return object.NIL, nil
	case "default=":
		if len(args) != 1 {
			return nil, errorf("evaluator: Hash#default= expects 1 arg, got %d", len(args))
		}
		r.Default = args[0]
		r.DefaultBlock = nil
		return args[0], nil
	case "default_proc=":
		// MRI: takes a Proc / lambda (or nil to clear) invoked as
		// block.call(hash, missing_key) when a lookup misses. We
		// reuse DefaultBlock for both Hash.new { } and the explicit
		// default_proc= path; setting it via this method also clears
		// the value-default Default field so the proc takes precedence.
		if len(args) != 1 {
			return nil, errorf("evaluator: Hash#default_proc= expects 1 arg, got %d", len(args))
		}
		if _, ok := args[0].(*object.Nil); ok {
			r.DefaultBlock = nil
			return args[0], nil
		}
		if _, ok := args[0].(*object.Proc); !ok {
			return nil, errorf("evaluator: Hash#default_proc= expects Proc, got %T", args[0])
		}
		r.DefaultBlock = args[0]
		r.Default = nil
		return args[0], nil
	case "default_proc":
		if r.DefaultBlock != nil {
			if p, ok := r.DefaultBlock.(*object.Proc); ok {
				return p, nil
			}
		}
		return object.NIL, nil
	case "length", "size":
		return object.NewInteger(int64(len(r.Entries))), nil
	case "keys":
		ks := make([]object.RubyObject, 0, len(r.Entries))
		for _, e := range r.Entries {
			ks = append(ks, e.Key)
		}
		return object.NewArray(ks...), nil
	case "values":
		vs := make([]object.RubyObject, 0, len(r.Entries))
		for _, e := range r.Entries {
			vs = append(vs, e.Value)
		}
		return object.NewArray(vs...), nil
	case "values_at":
		// Hash#values_at(*keys) -- one slot per key, nil for misses.
		// MRI also honours the Hash#default / DefaultBlock on misses;
		// rake/backtrace.rb only feeds keys it knows are present,
		// keeping the simple impl honest there.
		out := make([]object.RubyObject, 0, len(args))
		for _, k := range args {
			var hit object.RubyObject = object.NIL
			for _, e := range r.Entries {
				if rubyEqualDispatch(env, e.Key, k) {
					hit = e.Value
					break
				}
			}
			out = append(out, hit)
		}
		return object.NewArray(out...), nil
	case "merge":
		out := make([]object.HashEntry, len(r.Entries))
		copy(out, r.Entries)
		for _, a := range args {
			other, ok := a.(*object.Hash)
			if !ok {
				return nil, errorf("evaluator: Hash#merge needs Hash args")
			}
			for _, e := range other.Entries {
				replaced := false
				for i := range out {
					if rubyEqualDispatch(env, out[i].Key, e.Key) {
						out[i].Value = e.Value
						replaced = true
						break
					}
				}
				if !replaced {
					out = append(out, e)
				}
			}
		}
		merged := object.NewHash(out...)
		// MRI Hash#merge preserves the receiver's default value /
		// default block on the result. Mirror that so block-default
		// hashes built up via `.merge({...})` still trigger their
		// default block on missing keys.
		merged.Default = r.Default
		merged.DefaultBlock = r.DefaultBlock
		return merged, nil
	case "delete":
		if len(args) != 1 {
			return nil, errorf("evaluator: Hash#delete expects 1 arg, got %d", len(args))
		}
		for i, e := range r.Entries {
			if rubyEqualDispatch(env, e.Key, args[0]) {
				r.Entries = append(r.Entries[:i], r.Entries[i+1:]...)
				return e.Value, nil
			}
		}
		return object.NIL, nil
	case "store":
		if len(args) != 2 {
			return nil, errorf("evaluator: Hash#store expects 2 args, got %d", len(args))
		}
		for i := range r.Entries {
			if rubyEqualDispatch(env, r.Entries[i].Key, args[0]) {
				r.Entries[i].Value = args[1]
				return args[1], nil
			}
		}
		r.Entries = append(r.Entries, object.HashEntry{Key: args[0], Value: args[1]})
		return args[1], nil
	case "to_a":
		out := make([]object.RubyObject, len(r.Entries))
		for i, e := range r.Entries {
			out[i] = object.NewArray(e.Key, e.Value)
		}
		return object.NewArray(out...), nil
	case "empty?":
		return object.BooleanOf(len(r.Entries) == 0), nil
	case "any?":
		return object.BooleanOf(len(r.Entries) > 0), nil
	case "fetch":
		if len(args) < 1 || len(args) > 2 {
			return nil, errorf("evaluator: Hash#fetch expects 1..2 args, got %d", len(args))
		}
		for _, e := range r.Entries {
			if rubyEqualDispatch(env, e.Key, args[0]) {
				return e.Value, nil
			}
		}
		if len(args) == 2 {
			return args[1], nil
		}
		return raiseBuiltin(env, "KeyError", "key not found")
	case "dig":
		var cur object.RubyObject = r
		for _, k := range args {
			switch x := cur.(type) {
			case *object.Hash:
				var found object.RubyObject = object.NIL
				for _, e := range x.Entries {
					if rubyEqualDispatch(env, e.Key, k) {
						found = e.Value
						break
					}
				}
				cur = found
			case *object.Array:
				ki, ok := k.(*object.Integer)
				if !ok {
					return nil, errorf("evaluator: Array#dig needs Integer key")
				}
				idx := int(ki.Value)
				if idx < 0 {
					idx += len(x.Elements)
				}
				if idx < 0 || idx >= len(x.Elements) {
					return object.NIL, nil
				}
				cur = x.Elements[idx]
			default:
				return object.NIL, nil
			}
			if _, isNil := cur.(*object.Nil); isNil {
				return object.NIL, nil
			}
		}
		return cur, nil
	case "sort":
		// Sort by key ascending; returns Array of [k, v] pairs.
		out := make([]object.HashEntry, len(r.Entries))
		copy(out, r.Entries)
		sortStable(len(out), func(i, j int) bool {
			c, _ := compareObjectsEnv(env, out[i].Key, out[j].Key)
			return c < 0
		}, func(i, j int) {
			out[i], out[j] = out[j], out[i]
		})
		pairs := make([]object.RubyObject, len(out))
		for i, e := range out {
			pairs[i] = object.NewArray(e.Key, e.Value)
		}
		return object.NewArray(pairs...), nil
	case "min", "max":
		// Hash#min / #max return [k, v] of min/max key.
		if len(r.Entries) == 0 {
			return object.NIL, nil
		}
		best := r.Entries[0]
		for _, e := range r.Entries[1:] {
			c, ok := compareObjectsEnv(env, e.Key, best.Key)
			if !ok {
				return nil, errorf("evaluator: Hash#%s comparison failed", name)
			}
			if (name == "min" && c < 0) || (name == "max" && c > 0) {
				best = e
			}
		}
		return object.NewArray(best.Key, best.Value), nil
	case "invert":
		out := make([]object.HashEntry, len(r.Entries))
		for i, e := range r.Entries {
			out[i] = object.HashEntry{Key: e.Value, Value: e.Key}
		}
		return object.NewHash(out...), nil
	case "except":
		out := []object.HashEntry{}
	nextEntry:
		for _, e := range r.Entries {
			for _, k := range args {
				if rubyEqualDispatch(env, e.Key, k) {
					continue nextEntry
				}
			}
			out = append(out, e)
		}
		return object.NewHash(out...), nil
	case "has_key?", "key?", "include?", "member?":
		if len(args) != 1 {
			return nil, errorf("evaluator: wrong number of arguments to Hash#%s (given %d, expected 1)", name, len(args))
		}
		for _, e := range r.Entries {
			if rubyEqualDispatch(env, e.Key, args[0]) {
				return object.TRUE, nil
			}
		}
		return object.FALSE, nil
	case "has_value?", "value?":
		if len(args) != 1 {
			return nil, errorf("evaluator: wrong number of arguments to Hash#%s (given %d, expected 1)", name, len(args))
		}
		for _, e := range r.Entries {
			if rubyEqualDispatch(env, e.Value, args[0]) {
				return object.TRUE, nil
			}
		}
		return object.FALSE, nil
	}
	return raiseBuiltin(env, "NoMethodError", "undefined method `"+name+"' for Hash")
}
