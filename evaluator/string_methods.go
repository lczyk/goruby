package evaluator

import (
	"github.com/lczyk/goruby/object"
)

// String method names registered on StringClass. Each adapter routes
// into the per-name body in callStringMethod (string.go), which is the
// String-side analogue of callArrayMethod / callHashMethod.
var stringMethodNames = []string{
	"length", "size", "upcase", "downcase", "capitalize", "swapcase",
	"strip", "reverse", "include?", "to_s", "empty?", "chars", "lines",
	"split", "chomp", "start_with?", "end_with?", "replace", "match?",
	"match", "scan", "sub", "gsub", "ord", "to_sym", "tr", "count",
	"bytes", "bytesize", "ljust", "rjust", "center", "succ", "next",
	"to_i",
}

func init() {
	c := object.StringClass
	for _, name := range stringMethodNames {
		n := name
		c.AddMethod(n, &object.BuiltinMethod{
			Name: n,
			Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
				v, _, err := callStringMethod(env, recv, n, args)
				return v, err
			},
		})
	}
}
