package evaluator

import (
	"github.com/lczyk/goruby/object"
)

func init() {
	c := object.NilClassClass
	c.AddMethod("to_s", &object.BuiltinMethod{Name: "to_s", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		return object.NewString(""), nil
	}})
	c.AddMethod("to_a", &object.BuiltinMethod{Name: "to_a", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		return object.NewArray(), nil
	}})
	c.AddMethod("to_h", &object.BuiltinMethod{Name: "to_h", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		return object.NewHash(), nil
	}})
	c.AddMethod("inspect", &object.BuiltinMethod{Name: "inspect", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		return object.NewString("nil"), nil
	}})
}
