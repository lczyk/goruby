// Package evaluator implements the goruby tree-walking evaluator.
//
// The evaluator dispatches over ast.Node and produces object.RubyObject
// values. Version-aware behaviour follows the same pattern as the lexer
// and parser: a token.RubyVersion is held on the environment, gated
// features test against it via .AtLeast().
package evaluator

import (
	"github.com/lczyk/goruby/evaluator/builtinapi"
)

// Error and errorf are local aliases over the builtinapi exports so
// existing evaluator code keeps its lowercase names while subpackages
// (e.g. evaluator/stdlib) can construct the same type via the
// exported names.
type Error = builtinapi.Error

func errorf(format string, args ...any) *Error {
	return builtinapi.Errorf(format, args...)
}
