package evaluator

import (
	"github.com/lczyk/goruby/object"
)

var hashMethodNames = []string{
	"length", "size", "keys", "values", "merge", "delete", "store",
	"to_a", "empty?", "any?", "fetch", "dig", "sort", "min", "max",
	"invert", "except", "has_key?", "key?", "include?", "member?",
}

func init() {
	c := object.HashClass
	for _, name := range hashMethodNames {
		n := name
		c.AddMethod(n, &object.BuiltinMethod{
			Name: n,
			Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
				return callMethodLegacy(env, recv, n, args)
			},
		})
	}
}
