// Package builtinapi exposes the minimal surface that builtin class
// implementations need to plug into the evaluator: an Error type for
// returning evaluator-internal errors, a NativeFn marker for attaching
// Go-implemented method bodies onto UserMethod.Body, and small value
// helpers (StringText, SymbolOrString) that decode common ruby value
// shapes.
//
// The package is intentionally tiny and depends only on object, so
// subpackages under evaluator (e.g. evaluator/stdlib) can register
// builtins without pulling in the AST walker / dispatch core.
package builtinapi

import (
	"fmt"

	"github.com/lczyk/goruby/object"
)

// Error represents an evaluator-internal error. Distinct from ruby
// exceptions raised by user code -- those flow as object.Exception
// values through the RubyObject return channel.
type Error struct {
	Msg string
}

func (e *Error) Error() string { return e.Msg }

// Errorf creates a new *Error with the formatted message. Errors
// produced this way carry the evaluator: prefix convention by
// convention of callers (mirrors fmt.Errorf usage).
func Errorf(format string, args ...any) *Error {
	return &Error{Msg: fmt.Sprintf(format, args...)}
}

// NativeFn is a body marker for Go-implemented methods. Wraps a Go
// function with the canonical method-body signature. The evaluator's
// def-dispatch recognises NativeFn as a UserMethod body and invokes
// its Fn directly without parsing a ruby body.
type NativeFn struct {
	Fn func(env *object.Environment, args []object.RubyObject) (object.RubyObject, error)
}

// StringText extracts the raw text of a ruby String / FrozenString.
// Returns ("", false) for non-string operands. The env is needed to
// resolve frozen string IDs through the string pool.
func StringText(env *object.Environment, o object.RubyObject) (string, bool) {
	switch v := o.(type) {
	case *object.String:
		return string(v.Buf), true
	case *object.FrozenString:
		return env.Strings().Get(v.ID), true
	}
	return "", false
}

// SymbolOrString extracts text from a Symbol or String value. Used by
// builtin methods that accept either form for a method-name or key.
func SymbolOrString(env *object.Environment, o object.RubyObject) (string, bool) {
	switch v := o.(type) {
	case *object.Symbol:
		return env.Symbols().Name(v.ID), true
	case *object.String:
		return string(v.Buf), true
	case *object.FrozenString:
		return env.Strings().Get(v.ID), true
	}
	return "", false
}
