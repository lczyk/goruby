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
	case *object.Symbol:
		// Symbols stringify by name for purposes of regex /
		// case-equal matching (`/^test_/ === :test_x`). MRI also
		// treats Symbols this way in regex context. Distinct from
		// general String coercion (which MRI doesn't do for
		// symbols), but the methods that consume stringText all
		// want this behaviour.
		return env.Symbols().Name(v.ID), true
	}
	return "", false
}

// RaiseBuiltin is set by the evaluator package at init time. Stdlib
// builtins call this to raise a ruby exception of the named built-in
// class with the given message. Function-pointer injection avoids
// pulling the evaluator's raiseSignal type into builtinapi, which
// would create an import cycle.
var RaiseBuiltin func(env *object.Environment, className, msg string) (object.RubyObject, error)

// InvokeCurrentBlock invokes env.CurrentBlock with the given args.
// Returns (value, ok, err): ok=false means no block was present.
// Lets stdlib stubs that need to yield (e.g. OptionParser.new { |p| }
// passing the parser to its block) do so without importing the
// evaluator's BlockExpression / goBlockMarker types.
var InvokeCurrentBlock func(env *object.Environment, args []object.RubyObject) (object.RubyObject, bool, error)

// InvokeBlockValue invokes a previously-captured block value (whatever
// shape env.CurrentBlock was when the stdlib stored it) with the
// given args. Used by stubs that capture a block on one call and
// invoke it later (e.g. OptionParser#on stores handlers that
// OptionParser#parse! later fires).
var InvokeBlockValue func(env *object.Environment, blk any, args []object.RubyObject) (object.RubyObject, error)

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
