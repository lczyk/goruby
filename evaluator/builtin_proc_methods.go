package evaluator

import (
	"github.com/lczyk/goruby/object"
)

func init() {
	c := object.ProcClass
	invoke := func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		return invokeProc(env, recv.(*object.Proc), args)
	}
	c.AddMethod("call", &object.BuiltinMethod{Name: "call", Fn: invoke})
	c.AddMethod("yield", &object.BuiltinMethod{Name: "yield", Fn: invoke})
	c.AddMethod("()", &object.BuiltinMethod{Name: "()", Fn: invoke})
	c.AddMethod("[]", &object.BuiltinMethod{Name: "[]", Fn: invoke})
	c.AddMethod("lambda?", &object.BuiltinMethod{Name: "lambda?", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		return object.BooleanOf(recv.(*object.Proc).IsLambda), nil
	}})
	c.AddMethod("arity", &object.BuiltinMethod{Name: "arity", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		return procArity(recv.(*object.Proc)), nil
	}})
	// Proc#to_proc returns self -- mirrors MRI. Lets a Proc value flow
	// through &-capture chains that re-emit via to_proc (the idiom
	// where a method receiving a block-payload passes it onwards via
	// `another_meth(&block)` and that method's signature accepts an
	// object responding to to_proc).
	c.AddMethod("to_proc", &object.BuiltinMethod{Name: "to_proc", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		return recv, nil
	}})
	c.AddMethod("curry", &object.BuiltinMethod{Name: "curry", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		// Minimal stub: returns self. MRI's curry partially applies
		// args; full impl is meaty. Sufficient for callers that
		// reach for it to validate availability rather than rely on
		// the partial-application behaviour.
		return recv, nil
	}})

	// Method class -- Object#method(:foo) returns a Proc with
	// IsMethod=true; its .class is MethodClass. Install the
	// dispatch + introspection surface separately so callers see
	// Method-shaped methods on the value.
	mc := object.MethodClass
	mc.AddMethod("call", &object.BuiltinMethod{Name: "call", Fn: invoke})
	mc.AddMethod("()", &object.BuiltinMethod{Name: "()", Fn: invoke})
	mc.AddMethod("[]", &object.BuiltinMethod{Name: "[]", Fn: invoke})
	mc.AddMethod("to_proc", &object.BuiltinMethod{Name: "to_proc", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		// Method#to_proc returns a normal (non-Method) Proc with the
		// same backing marker so call sites that auto-flatten via &m
		// don't double-tag.
		p := recv.(*object.Proc)
		dup := *p
		dup.IsMethod = false
		return &dup, nil
	}})
	mc.AddMethod("arity", &object.BuiltinMethod{Name: "arity", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		return procArity(recv.(*object.Proc)), nil
	}})
	mc.AddMethod("name", &object.BuiltinMethod{Name: "name", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		p := recv.(*object.Proc)
		if bm, ok := p.Params.(*boundMethodMarker); ok {
			return env.Symbols().Intern(bm.Name), nil
		}
		return object.NIL, nil
	}})
	mc.AddMethod("receiver", &object.BuiltinMethod{Name: "receiver", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		p := recv.(*object.Proc)
		if bm, ok := p.Params.(*boundMethodMarker); ok {
			return bm.Recv, nil
		}
		return object.NIL, nil
	}})
}
