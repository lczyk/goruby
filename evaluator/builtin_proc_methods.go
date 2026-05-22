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
}
