package evaluator

import (
	"github.com/lczyk/goruby/object"
)

func init() {
	for _, c := range []*object.Class{object.TrueClassClass, object.FalseClassClass} {
		c.AddMethod("to_s", &object.BuiltinMethod{Name: "to_s", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
			b := recv.(*object.Boolean)
			if b.Value {
				return object.NewString("true"), nil
			}
			return object.NewString("false"), nil
		}})
		c.AddMethod("inspect", &object.BuiltinMethod{Name: "inspect", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
			b := recv.(*object.Boolean)
			if b.Value {
				return object.NewString("true"), nil
			}
			return object.NewString("false"), nil
		}})
		c.AddMethod("&", &object.BuiltinMethod{Name: "&", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
			if len(args) != 1 {
				return nil, errorf("evaluator: Boolean#& expects 1 arg")
			}
			b := recv.(*object.Boolean)
			return object.BooleanOf(b.Value && truthy(args[0])), nil
		}})
		c.AddMethod("|", &object.BuiltinMethod{Name: "|", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
			if len(args) != 1 {
				return nil, errorf("evaluator: Boolean#| expects 1 arg")
			}
			b := recv.(*object.Boolean)
			return object.BooleanOf(b.Value || truthy(args[0])), nil
		}})
		c.AddMethod("^", &object.BuiltinMethod{Name: "^", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
			if len(args) != 1 {
				return nil, errorf("evaluator: Boolean#^ expects 1 arg")
			}
			b := recv.(*object.Boolean)
			return object.BooleanOf(b.Value != truthy(args[0])), nil
		}})
	}
}
