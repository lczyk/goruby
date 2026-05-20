// Package evaluator implements the goruby tree-walking evaluator.
//
// The evaluator dispatches over ast.Node and produces object.RubyObject
// values. Version-aware behaviour follows the same pattern as the lexer
// and parser: a token.RubyVersion is held on the environment, gated
// features test against it via .AtLeast().
package evaluator

import (
	"fmt"
)

// Error represents an evaluator-internal error. Distinct from ruby
// exceptions raised by user code -- those are object.Exception values
// returned via the RubyObject channel.
type Error struct {
	Msg string
}

func (e *Error) Error() string { return e.Msg }

func errorf(format string, args ...any) *Error {
	return &Error{Msg: fmt.Sprintf(format, args...)}
}

