package evaluator

import (
	"reflect"

	"github.com/lczyk/goruby/ast"
	"github.com/lczyk/goruby/object"
)

// objectIDOf returns a Go-pointer-derived ID that's stable for the
// lifetime of recv. For pointer-like Ruby values (Instance / Array /
// Hash / String / Class) the address is the natural ID. For
// value-shaped Integer/Symbol pools, two Ruby-equal values share an
// address by design, so the IDs match -- close enough to MRI's
// small-Integer / Symbol unique-id behaviour.
func objectIDOf(o object.RubyObject) uintptr {
	v := reflect.ValueOf(o)
	if v.IsValid() && v.Kind() == reflect.Ptr {
		return v.Pointer()
	}
	return 0
}

// Universal Object methods migrated onto object.ObjectClass. Every
// builtin class inherits from Object so Send finds these on any
// receiver type that doesn't override them.

func init() {
	c := object.ObjectClass

	add := func(name string, fn func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error)) {
		c.AddMethod(name, &object.BuiltinMethod{
			Name: name,
			Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
				return fn(env, recv, args)
			},
		})
	}

	add("nil?", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
		_, isNil := recv.(*object.Nil)
		return object.BooleanOf(isNil), nil
	})
	add("class", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
		return classOf(env, recv), nil
	})
	add("inspect", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
		return object.NewString(env.Inspect(recv)), nil
	})
	add("itself", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
		return recv, nil
	})
	// Object#initialize: MRI's default-noop constructor. Concrete
	// classes override; chained super-calls eventually land here and
	// must succeed silently with whatever args were passed in.
	add("initialize", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
		return object.NIL, nil
	})
	// Object#object_id returns a stable integer unique to recv for its
	// lifetime. MRI uses the address-derived value; we approximate via
	// a Go pointer value cast to int64 -- different objects get
	// different IDs, the same object always returns the same ID for
	// the duration of the run.
	// Object#hash -- default to object_id. Hash keys using arbitrary
	// objects rely on this for keyed lookup; without it dispatch
	// raises NoMethodError when those objects are used as hash keys.
	add("hash", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
		return object.NewInteger(int64(objectIDOf(recv))), nil
	})
	// Object#define_singleton_method(name) { ... } installs a per-
	// instance singleton method backed by the block. Common per-test
	// stubbing idiom; works without touching the receiver's class.
	c.AddMethod("define_singleton_method", &object.BuiltinMethod{Name: "define_singleton_method", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		if len(args) < 1 {
			return nil, errorf("evaluator: define_singleton_method: missing name")
		}
		name, ok := symbolOrString(env, args[0])
		if !ok {
			return nil, errorf("evaluator: define_singleton_method: name must be Symbol or String")
		}
		// Pick up the block: via block param, or via a Proc passed as
		// args[1] (the &p form).
		bm, _ := block.(*goBlockMarker)
		var p *object.Proc
		if len(args) >= 2 {
			if pp, ok := args[1].(*object.Proc); ok {
				p = pp
			}
		}
		inst, ok := recv.(*object.Instance)
		if !ok {
			return nil, errorf("evaluator: define_singleton_method: non-Instance recv %T", recv)
		}
		if inst.SingletonMethods == nil {
			inst.SingletonMethods = map[string]object.RubyMethod{}
		}
		var fn func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error)
		if p != nil {
			pf := p
			fn = func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
				return invokeProc(env, pf, args)
			}
		} else if bm != nil && bm.blk != nil {
			bf := bm
			fn = func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
				return invokeBlock(env, bf.blk, args)
			}
		} else {
			return nil, errorf("evaluator: define_singleton_method: needs a block or Proc")
		}
		inst.SingletonMethods[name] = &object.BuiltinMethod{Name: name, Fn: fn}
		return env.Symbols().Intern(name), nil
	}})
	// Object#methods returns the symbol names of all methods visible
	// on the receiver -- singleton + class + walked super chain.
	add("methods", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
		out := make([]object.RubyObject, 0)
		seen := map[string]bool{}
		emit := func(name string) {
			if !seen[name] {
				seen[name] = true
				out = append(out, env.Symbols().Intern(name))
			}
		}
		if inst, ok := recv.(*object.Instance); ok {
			for name := range inst.SingletonMethods {
				emit(name)
			}
			if inst.SingletonClass != nil {
				for cur := inst.SingletonClass; cur != nil; cur = cur.Super {
					for name := range cur.Methods {
						emit(name)
					}
				}
			}
			for cur := inst.C; cur != nil; cur = cur.Super {
				for name := range cur.Methods {
					emit(name)
				}
			}
		} else if cls := classOfRaw(env, recv); cls != nil {
			for cur := cls; cur != nil; cur = cur.Super {
				for name := range cur.Methods {
					emit(name)
				}
			}
		}
		return object.NewArray(out...), nil
	})
	add("object_id", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
		// Stable interface pointer: take the address via an
		// interface-typed local. Builtins like Integer / Symbol are
		// fly-weighted so two Ruby-equal values share an ID; that
		// matches MRI for small Integers and Symbols (Float / Bignum
		// differ from MRI but no fixture hits them through object_id).
		return object.NewInteger(int64(objectIDOf(recv))), nil
	})
	c.AddMethod("equal?", &object.BuiltinMethod{
		Name: "equal?",
		Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
			if len(args) != 1 {
				return nil, errorf("evaluator: equal? expects 1 arg, got %d", len(args))
			}
			return object.BooleanOf(objectIDOf(recv) == objectIDOf(args[0])), nil
		},
	})
	// extend(mod, ...) -- copy each mod's instance methods onto recv's
	// singleton table so they become callable as singleton methods on
	// recv directly. `extend self` inside a module body (self == the
	// module) makes the module's instance methods callable as
	// module-level methods on the module itself, the standard pattern
	// for "namespace + utility" modules (FileUtils, Kernel, ...).
	add("extend", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
		if len(args) == 0 {
			return nil, errorf("evaluator: extend: wrong number of arguments (given 0, expected 1+)")
		}
		// install copies methods from src onto recv. Walks src's include
		// chain so transitively included modules (e.g. extend Rake::DSL
		// where DSL includes FileUtilsExt which includes FileUtils)
		// surface their methods too. MRI achieves this by splicing the
		// modules into the singleton's ancestors chain; goruby's extend
		// is copy-based so the walk has to happen explicitly. Methods
		// declared directly on the deepest module win over inherited
		// ones because we install includes first, then the module
		// itself.
		var install func(src *object.Class) error
		install = func(src *object.Class) error {
			for _, inc := range src.Includes {
				if err := install(inc); err != nil {
					return err
				}
			}
			for name, m := range src.Methods {
				switch r := recv.(type) {
				case *object.Class:
					r.AddClassMethod(name, m)
				case *object.Instance:
					if r.SingletonMethods == nil {
						r.SingletonMethods = map[string]object.RubyMethod{}
					}
					r.SingletonMethods[name] = m
				default:
					return errorf("evaluator: extend: cannot extend %T", recv)
				}
			}
			return nil
		}
		for _, a := range args {
			mod, ok := a.(*object.Class)
			if !ok {
				return nil, errorf("evaluator: extend: expected Module, got %T", a)
			}
			if err := install(mod); err != nil {
				return nil, err
			}
		}
		return recv, nil
	})
	add("instance_variable_get", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
		if len(args) != 1 {
			return nil, errorf("evaluator: instance_variable_get expects 1 arg")
		}
		name, ok := symbolOrString(env, args[0])
		if !ok {
			return nil, errorf("evaluator: instance_variable_get: name must be Symbol or String")
		}
		key := name
		if len(key) == 0 || key[0] != '@' {
			key = "@" + key
		}
		switch r := recv.(type) {
		case *object.Instance:
			if v, ok := r.Ivars[key]; ok {
				return v, nil
			}
		case *object.Class:
			if v, ok := r.Ivars[key]; ok {
				return v, nil
			}
		}
		return object.NIL, nil
	})
	add("instance_variable_set", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
		if len(args) != 2 {
			return nil, errorf("evaluator: instance_variable_set expects 2 args")
		}
		name, ok := symbolOrString(env, args[0])
		if !ok {
			return nil, errorf("evaluator: instance_variable_set: name must be Symbol or String")
		}
		key := name
		if len(key) == 0 || key[0] != '@' {
			key = "@" + key
		}
		switch r := recv.(type) {
		case *object.Instance:
			r.Ivars[key] = args[1]
			return args[1], nil
		case *object.Class:
			if r.Ivars == nil {
				r.Ivars = map[string]object.RubyObject{}
			}
			r.Ivars[key] = args[1]
			return args[1], nil
		}
		return nil, errorf("evaluator: instance_variable_set: receiver must be an instance, got %T", recv)
	})
	add("instance_variables", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
		inst, ok := recv.(*object.Instance)
		if !ok {
			return object.NewArray(), nil
		}
		out := make([]object.RubyObject, 0, len(inst.Ivars))
		for k := range inst.Ivars {
			out = append(out, env.Symbols().Intern(k))
		}
		return object.NewArray(out...), nil
	})
	add("frozen?", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
		switch v := recv.(type) {
		case *object.Symbol, *object.Integer, *object.Float, *object.Nil, *object.Boolean, *object.FrozenString:
			return object.TRUE, nil
		case *object.String:
			return object.BooleanOf(v.Frozen()), nil
		case *object.Array:
			return object.BooleanOf(v.Frozen()), nil
		}
		return object.FALSE, nil
	})
	add("freeze", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
		switch v := recv.(type) {
		case *object.String:
			v.Freeze()
		case *object.Array:
			v.Freeze()
		}
		return recv, nil
	})
	dup := func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
		copy := shallowCopy(env, recv)
		// MRI: clone / dup invoke initialize_copy(source) on the copy
		// after the shallow byte-copy. Lets mixins (Rake::Cloneable
		// being the canonical example) deep-clone instance variables.
		if inst, ok := copy.(*object.Instance); ok {
			if _, found := dispatchClass(env, inst).LookupMethod("initialize_copy"); found {
				if _, err := callMethod(env, inst, "initialize_copy", []object.RubyObject{recv}); err != nil {
					return nil, err
				}
			}
		}
		return copy, nil
	}
	add("dup", dup)
	add("clone", dup)
	// initialize_copy default: no-op. Mixins like Rake::Cloneable
	// override and call super, which lands here.
	add("initialize_copy", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
		return recv, nil
	})
	add("tap", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
		return recv, nil
	})
	add("equal?", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
		if len(args) != 1 {
			return nil, errorf("evaluator: equal? expects 1 arg")
		}
		return object.BooleanOf(recv == args[0]), nil
	})
	add("eql?", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
		if len(args) != 1 {
			return nil, errorf("evaluator: eql? expects 1 arg")
		}
		return object.BooleanOf(rubyEqual(recv, args[0])), nil
	})
	sendImpl := func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
		if len(args) < 1 {
			return nil, errorf("evaluator: send needs a method name")
		}
		mname, ok := symbolOrString(env, args[0])
		if !ok {
			return nil, errorf("evaluator: send: method name must be Symbol or String")
		}
		return callMethod(env, recv, mname, args[1:])
	}
	publicSendImpl := func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
		if len(args) < 1 {
			return nil, errorf("evaluator: public_send needs a method name")
		}
		mname, ok := symbolOrString(env, args[0])
		if !ok {
			return nil, errorf("evaluator: public_send: method name must be Symbol or String")
		}
		if inst, ok := recv.(*object.Instance); ok {
			if isPrivateMethod(inst.C, mname) {
				return raiseBuiltin(env, "NoMethodError", "private method `"+mname+"' called for instance of "+inst.C.Name)
			}
			if isProtectedMethod(inst.C, mname) && !callerIsKin(env, inst.C) {
				return raiseBuiltin(env, "NoMethodError", "protected method `"+mname+"' called for instance of "+inst.C.Name)
			}
		}
		return callMethod(env, recv, mname, args[1:])
	}
	add("send", sendImpl)
	add("__send__", sendImpl)
	add("public_send", publicSendImpl)
	add("method", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
		if len(args) != 1 {
			return nil, errorf("evaluator: Object#method expects 1 arg")
		}
		mname, ok := symbolOrString(env, args[0])
		if !ok {
			return nil, errorf("evaluator: Object#method: name must be Symbol or String")
		}
		return procFromBound(env, recv, mname), nil
	})
	add("singleton_class", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
		switch v := recv.(type) {
		case *object.Instance:
			return v.EnsureSingletonClass(), nil
		case *object.Class:
			// `Foo.singleton_class` is the class's own metaclass.
			// Without per-Class singleton modelling we return ClassClass
			// as a coarse stand-in -- good enough for the few callers
			// that just want `.class`-style introspection on it.
			return object.ClassClass, nil
		}
		return classOf(env, recv), nil
	})
	add("singleton_methods", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
		out := make([]object.RubyObject, 0)
		switch v := recv.(type) {
		case *object.Instance:
			for k := range v.SingletonMethods {
				out = append(out, env.Symbols().Intern(k))
			}
		case *object.Class:
			for k := range v.ClassMethods {
				out = append(out, env.Symbols().Intern(k))
			}
		}
		return object.NewArray(out...), nil
	})
	add("respond_to?", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
		// MRI: respond_to?(name, include_all = false). include_all
		// controls whether private methods count; we don't track
		// per-method privacy uniformly so the flag is ignored.
		if len(args) < 1 || len(args) > 2 {
			return nil, errorf("evaluator: wrong number of arguments to respond_to? (given %d, expected 1..2)", len(args))
		}
		mname, ok := symbolOrString(env, args[0])
		if !ok {
			return nil, errorf("evaluator: respond_to? needs Symbol or String, got %T", args[0])
		}
		return object.BooleanOf(receiverResponds(env, recv, mname)), nil
	})
	isA := func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
		if len(args) != 1 {
			return nil, errorf("evaluator: wrong number of arguments to Object#is_a? (given %d, expected 1)", len(args))
		}
		target, ok := args[0].(*object.Class)
		if !ok {
			return nil, errorf("evaluator: TypeError: class or module required")
		}
		cl := classOfRaw(env, recv)
		if cl == nil {
			return object.FALSE, nil
		}
		return object.BooleanOf(cl.IsAncestor(target)), nil
	}
	add("is_a?", isA)
	add("kind_of?", isA)
	add("instance_of?", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
		if len(args) != 1 {
			return nil, errorf("evaluator: wrong number of arguments to Object#instance_of? (given %d, expected 1)", len(args))
		}
		target, ok := args[0].(*object.Class)
		if !ok {
			return nil, errorf("evaluator: TypeError: class or module required")
		}
		cl := classOfRaw(env, recv)
		if cl == nil {
			return object.FALSE, nil
		}
		return object.BooleanOf(cl == target), nil
	})
	// instance_eval(&block) runs the block with self rebound to the
	// receiver. The block's lexical closure is preserved through the
	// underlying Proc; only self changes. Method dispatch inside the
	// body lands on receiver.class methods.
	c.AddMethod("instance_eval", &object.BuiltinMethod{
		Name: "instance_eval",
		Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, blockAny any) (object.RubyObject, error) {
			// Resolve the block payload to a Proc. Three shapes:
			//   - Proc passed positionally (rare; arrives via args[0])
			//   - literal block (ast.BlockExpression on blockAny)
			//   - goBlockMarker wrapping either a literal block AST
			//     or a Proc-via-&capture
			var p *object.Proc
			if len(args) == 1 {
				if pp, ok := args[0].(*object.Proc); ok {
					p = pp
				}
			}
			if p == nil {
				switch b := blockAny.(type) {
				case *ast.BlockExpression:
					if b != nil {
						inner := object.NewEnclosedEnvironment(env)
						inner.Self = recv
						return invokeBlock(inner, b, nil)
					}
				case *goBlockMarker:
					if b != nil && b.blk != nil {
						inner := object.NewEnclosedEnvironment(env)
						inner.Self = recv
						return invokeBlock(inner, b.blk, nil)
					}
					if b != nil && b.proc != nil {
						p = b.proc
					}
				}
			}
			// String form: instance_eval("src" [, filename [, lineno]])
			// parses src and evals it with self = recv. Defs install on
			// recv's singleton class. Used by rake's test_show_lines
			// which builds a small method on the fly.
			if p == nil && len(args) >= 1 {
				if src, ok := stringText(env, args[0]); ok {
					var cls *object.Class
					if inst, isInst := recv.(*object.Instance); isInst {
						cls = inst.EnsureSingletonClass()
					} else if c, isCls := recv.(*object.Class); isCls {
						cls = c
					}
					if cls != nil {
						return evalClassEvalString(env, cls, src)
					}
				}
			}
			if p == nil {
				return nil, errorf("evaluator: instance_eval needs a block")
			}
			// Proc shapes:
			//   - procFromBlock: Body = *ast.BlockStatement, run directly.
			//   - procFromGoBlock: Params = *goBlockMarker; the marker's
			//     blk field holds the original BlockExpression.
			if be, ok := p.Body.(*ast.BlockStatement); ok {
				defEnv, _ := p.DefEnv.(*object.Environment)
				if defEnv == nil {
					defEnv = env
				}
				inner := object.NewEnclosedEnvironment(defEnv)
				inner.Self = recv
				return evalBlockStatement(inner, be)
			}
			if marker, ok := p.Params.(*goBlockMarker); ok && marker != nil && marker.blk != nil {
				inner := object.NewEnclosedEnvironment(env)
				inner.Self = recv
				return invokeBlock(inner, marker.blk, nil)
			}
			return nil, errorf("evaluator: instance_eval: cannot extract block body from proc")
		},
	})
	c.AddMethod("instance_exec", c.Methods["instance_eval"])
}

// shallowCopy returns a fresh object whose mutable contents are a
// shallow copy of recv. Element / value identity is preserved (no deep
// recursion). For immutable primitives (Integer, Float, Symbol,
// Boolean, Nil, Range) recv is returned as-is -- MRI's `dup` on
// Integer/Symbol/etc raises in older versions and returns self in 3.0+;
// returning self matches the corpus we care about. Ruby distinguishes
// dup vs clone w/ respect to frozen state, which we don't track yet.
func shallowCopy(env *object.Environment, recv object.RubyObject) object.RubyObject {
	switch v := recv.(type) {
	case *object.Array:
		elems := make([]object.RubyObject, len(v.Elements))
		copy(elems, v.Elements)
		return object.NewArray(elems...)
	case *object.Hash:
		entries := make([]object.HashEntry, len(v.Entries))
		copy(entries, v.Entries)
		out := object.NewHash(entries...)
		out.Default = v.Default
		out.DefaultBlock = v.DefaultBlock
		return out
	case *object.String:
		cp := make([]byte, len(v.Buf))
		copy(cp, v.Buf)
		return object.NewStringFromBytes(cp)
	case *object.FrozenString:
		s := env.Strings().Get(v.ID)
		cp := make([]byte, len(s))
		copy(cp, s)
		return object.NewStringFromBytes(cp)
	case *object.Instance:
		ivs := make(map[string]object.RubyObject, len(v.Ivars))
		for k, val := range v.Ivars {
			ivs[k] = val
		}
		return &object.Instance{C: v.C, Ivars: ivs}
	}
	return recv
}
