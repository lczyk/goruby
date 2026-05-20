package evaluator

import (
	"github.com/lczyk/goruby/object"
)

var arrayMethodNames = []string{
	"length", "size", "count", "first", "last", "push", "append",
	"pop", "shift", "unshift", "prepend", "delete", "delete_at",
	"concat", "reverse", "sort", "include?", "empty?", "any?", "all?",
	"none?", "one?", "to_h", "to_a", "dig", "each_with_index",
	"each_slice", "each_cons", "zip", "take", "drop", "join", "min",
	"minmax", "max", "sum", "grep", "uniq", "compact", "flatten",
	"reduce", "inject", "tally",
}

func init() {
	c := object.ArrayClass
	for _, name := range arrayMethodNames {
		n := name
		c.AddMethod(n, &object.BuiltinMethod{
			Name: n,
			Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
				return callArrayMethod(env, recv.(*object.Array), n, args)
			},
		})
	}
}
