package evaluator

import (
	"github.com/lczyk/goruby/object"
)

// String method names handled by the legacy callStringMethod. Register
// each on StringClass via a thin BuiltinMethod adapter so Send becomes
// the dispatch path; the per-method implementations stay where they
// are until each gets inlined here.
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
