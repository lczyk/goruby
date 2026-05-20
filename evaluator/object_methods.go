package evaluator

import (
	"github.com/lczyk/goruby/object"
)

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
	add("instance_variable_get", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
		if len(args) != 1 {
			return nil, errorf("evaluator: instance_variable_get expects 1 arg")
		}
		name, ok := symbolOrString(env, args[0])
		if !ok {
			return nil, errorf("evaluator: instance_variable_get: name must be Symbol or String")
		}
		inst, ok := recv.(*object.Instance)
		if !ok {
			return object.NIL, nil
		}
		key := name
		if len(key) == 0 || key[0] != '@' {
			key = "@" + key
		}
		if v, ok := inst.Ivars[key]; ok {
			return v, nil
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
		inst, ok := recv.(*object.Instance)
		if !ok {
			return nil, errorf("evaluator: instance_variable_set: receiver must be an instance, got %T", recv)
		}
		key := name
		if len(key) == 0 || key[0] != '@' {
			key = "@" + key
		}
		inst.Ivars[key] = args[1]
		return args[1], nil
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
		switch recv.(type) {
		case *object.Symbol, *object.Integer, *object.Float, *object.Nil, *object.Boolean:
			return object.TRUE, nil
		}
		return object.FALSE, nil
	})
	dup := func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
		return recv, nil
	}
	add("dup", dup)
	add("clone", dup)
	add("tap", dup)
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
	add("send", sendImpl)
	add("__send__", sendImpl)
	add("public_send", sendImpl)
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
	add("respond_to?", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
		if len(args) != 1 {
			return nil, errorf("evaluator: wrong number of arguments to respond_to? (given %d, expected 1)", len(args))
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
}
