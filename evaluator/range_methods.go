package evaluator

import (
	"github.com/lczyk/goruby/object"
)

var rangeMethodNames = []string{
	"to_a", "size", "count", "length", "first", "last", "min", "max",
	"sum", "reduce", "inject", "min_by", "max_by", "sort", "sort_by",
	"tally", "uniq", "each_slice", "each_cons", "each_with_index",
	"zip", "take", "drop", "join", "flatten", "include?", "cover?",
}

func init() {
	c := object.RangeClass
	for _, name := range rangeMethodNames {
		n := name
		c.AddMethod(n, &object.BuiltinMethod{
			Name: n,
			Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
				return callMethodLegacy(env, recv, n, args)
			},
		})
	}
}
