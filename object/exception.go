package object

import (
	"fmt"
	"hash/fnv"
	"strings"

	"github.com/lczyk/goruby/trace"
)

var (
	exceptionClass RubyClassObject = newClass(
		"Exception",
		exceptionMethods,
		nil,
		func(c RubyClassObject, args ...RubyObject) (RubyObject, error) {
			return &Exception{message: c.Name()}, nil
		},
	)
)

func init() {
	CLASSES.Set("Exception", exceptionClass)
}

func formatException(exception RubyObject, message string) string {
	return fmt.Sprintf("%s: %s", RubyObjectToTypeString(exception), message)
}

type exception interface {
	RubyObject
	setErrorMessage(string)
	error
}

// NewException creates a new exception with the given message template and
// uses fmt.Sprintf to interpolate the args into message into the message.
func NewException(message string, args ...interface{}) *Exception {
	return &Exception{message: fmt.Sprintf(message, args...)}
}

type Exception struct {
	message string
}

func (e *Exception) Inspect() string            { return formatException(e, e.message) }
func (e *Exception) Error() string              { return e.message }
func (e *Exception) setErrorMessage(msg string) { e.message = msg }
func (e *Exception) Class() RubyClass           { return exceptionClass }
func (e *Exception) HashKey() HashKey           { return hashException(e) }

var (
	_ RubyObject = &Exception{}
	// _ RubyClass  = &Exception{}
	_ exception = &Exception{}
	_ error     = &Exception{}
)
var exceptionMethods = map[string]RubyMethod{
	"initialize": newMethod(exceptionInitialize),
	"exception":  newMethod(exceptionException),
	"to_s":       withArity(0, newMethod(exceptionToS)),
}

func exceptionInitialize(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	receiver := context.Receiver()
	var message string
	message = receiver.Class().Name()
	if len(args) == 1 {
		msg, err := stringify(args[0])
		if err != nil {
			return nil, err
		}
		message = msg
	}
	if exception, ok := receiver.(exception); ok {
		exception.setErrorMessage(message)
	}
	return receiver, nil
}

func exceptionException(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	receiver := context.Receiver()
	if len(args) == 0 {
		return receiver, nil
	}
	var oldMessage string
	if err, ok := receiver.(error); ok {
		oldMessage = err.Error()
	}
	message, err := stringify(args[0])
	if err != nil {
		return nil, err
	}

	if oldMessage != message {
		class := receiver.Class()
		exc, err := class.New()
		if err != nil {
			return nil, err
		}
		if exception, ok := exc.(exception); ok {
			exception.setErrorMessage(message)
		}
		return exc, nil
	}
	return receiver, nil
}

func exceptionToS(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	receiver := context.Receiver()
	if err, ok := receiver.(exception); ok {
		return NewString(err.Error()), nil
	}
	return nil, nil
}

func hashException(exception RubyObject) HashKey {
	h := fnv.New64a()
	h.Write([]byte(fmt.Sprintf("%T", exception)))
	if err, ok := exception.(error); ok {
		h.Write([]byte(err.Error()))
	}
	return HashKey(h.Sum64())
}

func NewStandardError(message string) *StandardError {
	return &StandardError{message: message}
}

type StandardError struct {
	message string
}

func (e *StandardError) Inspect() string            { return formatException(e, e.message) }
func (e *StandardError) Error() string              { return e.message }
func (e *StandardError) setErrorMessage(msg string) { e.message = msg }
func (e *StandardError) Class() RubyClass           { return exceptionClass }
func (e *StandardError) HashKey() HashKey           { return hashException(e) }

var (
	_ RubyObject = &StandardError{}
	_ error      = &StandardError{}
	_ exception  = &StandardError{}
)

func NewRuntimeError(format string, args ...interface{}) *RuntimeError {
	return &RuntimeError{
		message: fmt.Sprintf(format, args...),
	}
}

type RuntimeError struct {
	message string
}

func (e *RuntimeError) Inspect() string            { return formatException(e, e.message) }
func (e *RuntimeError) Error() string              { return e.message }
func (e *RuntimeError) setErrorMessage(msg string) { e.message = msg }
func (e *RuntimeError) Class() RubyClass           { return exceptionClass }
func (e *RuntimeError) HashKey() HashKey           { return hashException(e) }

var (
	_ RubyObject = &RuntimeError{}
	_ error      = &RuntimeError{}
	_ exception  = &RuntimeError{}
)

func NewZeroDivisionError() *ZeroDivisionError {
	return &ZeroDivisionError{
		message: "divided by 0",
	}
}

type ZeroDivisionError struct {
	message string
}

func (e *ZeroDivisionError) Inspect() string            { return formatException(e, e.message) }
func (e *ZeroDivisionError) Error() string              { return e.message }
func (e *ZeroDivisionError) setErrorMessage(msg string) { e.message = msg }
func (e *ZeroDivisionError) Class() RubyClass           { return exceptionClass }
func (e *ZeroDivisionError) HashKey() HashKey           { return hashException(e) }

var (
	_ RubyObject = &ZeroDivisionError{}
	_ error      = &ZeroDivisionError{}
	_ exception  = &ZeroDivisionError{}
)

func NewWrongNumberOfArgumentsError(expected, actual int) *ArgumentError {
	return &ArgumentError{
		message: fmt.Sprintf(
			"wrong number of arguments (given %d, expected %d)",
			actual,
			expected,
		),
	}
}

func NewArgumentError(format string, args ...interface{}) *ArgumentError {
	return &ArgumentError{
		message: fmt.Sprintf(format, args...),
	}
}

type ArgumentError struct {
	message string
}

func (e *ArgumentError) Inspect() string            { return formatException(e, e.message) }
func (e *ArgumentError) Error() string              { return e.message }
func (e *ArgumentError) setErrorMessage(msg string) { e.message = msg }
func (e *ArgumentError) Class() RubyClass           { return exceptionClass }
func (e *ArgumentError) HashKey() HashKey           { return hashException(e) }

var (
	_ RubyObject = &ArgumentError{}
	_ error      = &ArgumentError{}
	_ exception  = &ArgumentError{}
)

func NewUninitializedConstantNameError(name string) *NameError {
	return &NameError{
		message: fmt.Sprintf(
			"uninitialized constant %s",
			name,
		),
	}
}

type NameError struct {
	message string
}

func (e *NameError) Inspect() string            { return formatException(e, e.message) }
func (e *NameError) Error() string              { return e.message }
func (e *NameError) setErrorMessage(msg string) { e.message = msg }
func (e *NameError) Class() RubyClass           { return exceptionClass }
func (e *NameError) HashKey() HashKey           { return hashException(e) }

var (
	_ RubyObject = &NameError{}
	_ error      = &NameError{}
	_ exception  = &NameError{}
)

func NewNoMethodError(context RubyObject, method string) *NoMethodError {
	// panic("stop here")
	return &NoMethodError{
		message: fmt.Sprintf(
			"undefined method `%s' for %s:%s",
			method,
			context.Inspect(),
			context.Class().Inspect(),
		),
	}
}

type NoMethodError struct {
	message string
}

func (e *NoMethodError) Inspect() string            { return formatException(e, e.message) }
func (e *NoMethodError) Error() string              { return e.message }
func (e *NoMethodError) setErrorMessage(msg string) { e.message = msg }
func (e *NoMethodError) Class() RubyClass           { return exceptionClass }
func (e *NoMethodError) HashKey() HashKey           { return hashException(e) }

var (
	_ RubyObject = &NoMethodError{}
	_ error      = &NoMethodError{}
	_ exception  = &NoMethodError{}
)

func RubyObjectToTypeString(rubyType RubyObject) string {
	ts := fmt.Sprintf("%T", rubyType)
	ts = strings.TrimPrefix(ts, "*object.")
	ts = strings.TrimPrefix(ts, "object.")
	return ts
}

func NewWrongArgumentTypeError(expected, actual RubyObject) *TypeError {
	return &TypeError{
		Message: fmt.Sprintf("wrong argument type %s (expected %s)", actual, expected),
	}
}

func NewCoercionTypeError(expected, actual RubyObject) *TypeError {
	return &TypeError{
		Message: fmt.Sprintf("%s can't be coerced into %s", RubyObjectToTypeString(actual), RubyObjectToTypeString(expected)),
	}
}

func NewImplicitConversionTypeError(expected, actual RubyObject) *TypeError {
	return &TypeError{
		Message: fmt.Sprintf("no implicit conversion of %s into %s", RubyObjectToTypeString(actual), RubyObjectToTypeString(expected)),
	}
}

func NewImplicitConversionTypeErrorMany(actual RubyObject, expected ...RubyObject) *TypeError {
	if len(expected) == 0 {
		return nil
	}
	if len(expected) == 1 {
		return NewImplicitConversionTypeError(expected[0], actual)
	}
	types := make([]string, len(expected))
	for i, e := range expected {
		types[i] = RubyObjectToTypeString(e)
	}

	return &TypeError{
		Message: fmt.Sprintf(
			"no implicit conversion of %s into one of [%s]",
			RubyObjectToTypeString(actual),
			strings.Join(types, ", "),
		),
	}
}

func NewTypeError(message string) *TypeError {
	return &TypeError{Message: message}
}

type TypeError struct {
	Message string
}

func (e *TypeError) Inspect() string            { return formatException(e, e.Message) }
func (e *TypeError) Error() string              { return e.Message }
func (e *TypeError) setErrorMessage(msg string) { e.Message = msg }
func (e *TypeError) Class() RubyClass           { return exceptionClass }
func (e *TypeError) HashKey() HashKey           { return hashException(e) }

var (
	_ RubyObject = &TypeError{}
	_ error      = &TypeError{}
	_ exception  = &TypeError{}
)

func NewScriptError(format string, args ...interface{}) *ScriptError {
	return &ScriptError{message: fmt.Sprintf(format, args...)}
}

type ScriptError struct {
	message string
}

func (e *ScriptError) Inspect() string            { return formatException(e, e.message) }
func (e *ScriptError) Error() string              { return e.message }
func (e *ScriptError) setErrorMessage(msg string) { e.message = msg }
func (e *ScriptError) Class() RubyClass           { return exceptionClass }
func (e *ScriptError) HashKey() HashKey           { return hashException(e) }

var (
	_ RubyObject = &ScriptError{}
	_ error      = &ScriptError{}
	_ exception  = &ScriptError{}
)

func NewSyntaxError(syntaxError error) *SyntaxError {
	return &SyntaxError{
		message: fmt.Sprintf(
			"syntax error, %s",
			syntaxError.Error(),
		),
		err: syntaxError,
	}
}

type SyntaxError struct {
	err     error
	message string
}

func (e *SyntaxError) Inspect() string            { return formatException(e, e.message) }
func (e *SyntaxError) Error() string              { return e.message }
func (e *SyntaxError) setErrorMessage(msg string) { e.message = msg }
func (e *SyntaxError) Class() RubyClass           { return exceptionClass }
func (e *SyntaxError) UnderlyingError() error     { return e.err }
func (e *SyntaxError) HashKey() HashKey           { return hashException(e) }

var (
	_ RubyObject = &SyntaxError{}
	_ error      = &SyntaxError{}
	_ exception  = &SyntaxError{}
)

func NewNotImplementedError(format string, args ...interface{}) *NotImplementedError {
	return &NotImplementedError{message: fmt.Sprintf(format, args...)}
}

type NotImplementedError struct {
	message string
}

func (e *NotImplementedError) Inspect() string            { return formatException(e, e.message) }
func (e *NotImplementedError) Error() string              { return e.message }
func (e *NotImplementedError) setErrorMessage(msg string) { e.message = msg }
func (e *NotImplementedError) Class() RubyClass           { return exceptionClass }
func (e *NotImplementedError) HashKey() HashKey           { return hashException(e) }

var (
	_ RubyObject = &NotImplementedError{}
	_ error      = &NotImplementedError{}
	_ exception  = &NotImplementedError{}
)

// IsError returns true if the given RubyObject is an object.Error or an
// object.Exception (or any subclass of object.Exception)
func IsError(obj RubyObject) bool {
	if obj == nil {
		return false
	}
	_, ok := obj.(exception)
	return ok
}
