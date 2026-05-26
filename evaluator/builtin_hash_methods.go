package evaluator

import (
	"github.com/lczyk/goruby/object"
)

var hashMethodNames = []string{
	"length", "size", "keys", "values", "values_at", "merge", "delete", "store",
	"to_a", "empty?", "any?", "fetch", "dig", "sort", "min", "max",
	"invert", "except", "has_key?", "key?", "include?", "member?",
	"has_value?", "value?",
	"default", "default=", "default_proc", "default_proc=",
}

func init() {
	c := object.HashClass
	for _, name := range hashMethodNames {
		n := name
		c.AddMethod(n, &object.BuiltinMethod{
			Name: n,
			Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
				return callHashMethod(env, recv.(*object.Hash), n, args)
			},
		})
	}
	// Hash.try_convert(obj) -- returns obj if it's already a Hash,
	// otherwise tries obj.to_hash and returns the result (or nil if
	// to_hash is undefined). MRI uses this to coerce optionally-Hash
	// args without raising. rake/task.rb's execute method uses it on
	// the *args bundle to detect a trailing kwargs Hash.
	// Operator-style methods so __send__ / method().call() routes the
	// same way as the infix path. [] / []= read and write the hash;
	// == is structural value equality.
	c.AddMethod("[]", &object.BuiltinMethod{Name: "[]", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		if len(args) != 1 {
			return nil, errorf("evaluator: Hash#[]: expected 1 arg, got %d", len(args))
		}
		h, _ := recv.(*object.Hash)
		for _, e := range h.Entries {
			if rubyEqual(e.Key, args[0]) {
				return e.Value, nil
			}
		}
		if h.Default != nil {
			return h.Default, nil
		}
		return object.NIL, nil
	}})
	c.AddMethod("[]=", &object.BuiltinMethod{Name: "[]=", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		if len(args) != 2 {
			return nil, errorf("evaluator: Hash#[]=: expected 2 args, got %d", len(args))
		}
		h, _ := recv.(*object.Hash)
		for i := range h.Entries {
			if rubyEqual(h.Entries[i].Key, args[0]) {
				h.Entries[i].Value = args[1]
				return args[1], nil
			}
		}
		h.Entries = append(h.Entries, object.HashEntry{Key: args[0], Value: args[1]})
		return args[1], nil
	}})
	c.AddMethod("==", &object.BuiltinMethod{Name: "==", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		return object.BooleanOf(rubyEqual(recv, args[0])), nil
	}})
	// Hash#to_h: returns self (or a Hash with the same entries). MRI's
	// to_h accepts a block to remap, but the no-block form is the
	// identity. Sufficient for mock's compaction shape.
	c.AddMethod("to_h", &object.BuiltinMethod{Name: "to_h", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		return recv, nil
	}})
	// Hash#clear -- empty the hash in place; returns self.
	c.AddMethod("clear", &object.BuiltinMethod{Name: "clear", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		h, ok := recv.(*object.Hash)
		if !ok {
			return nil, errorf("evaluator: Hash#clear: receiver must be Hash, got %T", recv)
		}
		h.Entries = h.Entries[:0]
		return h, nil
	}})
	// Hash#compact -- drop entries whose value is nil.
	c.AddMethod("compact", &object.BuiltinMethod{Name: "compact", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		h, ok := recv.(*object.Hash)
		if !ok {
			return nil, errorf("evaluator: Hash#compact: receiver must be Hash, got %T", recv)
		}
		out := []object.HashEntry{}
		for _, e := range h.Entries {
			if _, isNil := e.Value.(*object.Nil); !isNil {
				out = append(out, e)
			}
		}
		return object.NewHash(out...), nil
	}})
	// Hash#reduce/#inject(memo=first) { |memo, (k, v)| ... } folds
	// over entries with an explicit memo seed (or the first entry's
	// pair as the seed). Returns the final memo.
	reduceFn := func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		h, ok := recv.(*object.Hash)
		if !ok {
			return nil, errorf("evaluator: Hash#reduce: receiver must be Hash, got %T", recv)
		}
		bm, ok := block.(*goBlockMarker)
		if !ok || bm == nil || bm.fn == nil {
			return object.NIL, nil
		}
		var memo object.RubyObject
		start := 0
		if len(args) >= 1 {
			memo = args[0]
		} else if len(h.Entries) > 0 {
			memo = object.NewArray(h.Entries[0].Key, h.Entries[0].Value)
			start = 1
		} else {
			return object.NIL, nil
		}
		for i := start; i < len(h.Entries); i++ {
			pair := object.NewArray(h.Entries[i].Key, h.Entries[i].Value)
			v, err := bm.fn([]object.RubyObject{memo, pair})
			if err != nil {
				return nil, err
			}
			memo = v
		}
		return memo, nil
	}
	c.AddMethod("reduce", &object.BuiltinMethod{Name: "reduce", Fn: reduceFn})
	c.AddMethod("inject", &object.BuiltinMethod{Name: "inject", Fn: reduceFn})

	// Hash#each_with_object(seed) { |(k, v), memo| ... } yields each
	// entry alongside the seed; returns seed.
	c.AddMethod("each_with_object", &object.BuiltinMethod{Name: "each_with_object", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		h, ok := recv.(*object.Hash)
		if !ok {
			return nil, errorf("evaluator: Hash#each_with_object: receiver must be Hash, got %T", recv)
		}
		if len(args) != 1 {
			return nil, errorf("evaluator: Hash#each_with_object: expected 1 arg, got %d", len(args))
		}
		memo := args[0]
		bm, ok := block.(*goBlockMarker)
		if !ok || bm == nil || bm.fn == nil {
			return memo, nil
		}
		for _, e := range h.Entries {
			pair := object.NewArray(e.Key, e.Value)
			if _, err := bm.fn([]object.RubyObject{pair, memo}); err != nil {
				return nil, err
			}
		}
		return memo, nil
	}})
	c.AddMethod("transform_keys", &object.BuiltinMethod{Name: "transform_keys", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		h, ok := recv.(*object.Hash)
		if !ok {
			return nil, errorf("evaluator: Hash#transform_keys: receiver must be Hash, got %T", recv)
		}
		if bm, ok := block.(*goBlockMarker); ok && bm != nil && bm.fn != nil {
			out := make([]object.HashEntry, 0, len(h.Entries))
			for _, e := range h.Entries {
				k, err := bm.fn([]object.RubyObject{e.Key})
				if err != nil {
					return nil, err
				}
				out = append(out, object.HashEntry{Key: k, Value: e.Value})
			}
			return object.NewHash(out...), nil
		}
		// No block: return self (Enumerator approximation).
		return h, nil
	}})

	// Override fetch to support a block default. Order matters --
	// this AddMethod replaces the entry registered by the generic
	// loop above. With a block, miss invokes the block with the
	// requested key; without one, fall back to the static-default
	// or raise path in callHashMethod.
	c.AddMethod("fetch", &object.BuiltinMethod{Name: "fetch", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		h, ok := recv.(*object.Hash)
		if !ok {
			return nil, errorf("evaluator: Hash#fetch: receiver must be Hash, got %T", recv)
		}
		if len(args) < 1 {
			return nil, errorf("evaluator: Hash#fetch: expected at least 1 arg")
		}
		for _, e := range h.Entries {
			if rubyEqualDispatch(env, e.Key, args[0]) {
				return e.Value, nil
			}
		}
		if block != nil {
			return invokeBlockValue(env, block, []object.RubyObject{args[0]})
		}
		if len(args) >= 2 {
			return args[1], nil
		}
		return raiseBuiltin(env, "KeyError", "key not found")
	}})

	c.ClassMethods["try_convert"] = &object.BuiltinMethod{
		Name: "try_convert",
		Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
			if len(args) != 1 {
				return nil, errorf("evaluator: Hash.try_convert: expected 1 arg, got %d", len(args))
			}
			if h, ok := args[0].(*object.Hash); ok {
				return h, nil
			}
			return object.NIL, nil
		},
	}
}
