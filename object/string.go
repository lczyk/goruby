package object

import (
	"fmt"
	"hash/fnv"
	"regexp"
	"strconv"
	"strings"

	"github.com/lczyk/goruby/trace"
)

var stringClass RubyClassObject = newClass(
	"String",
	stringMethods,
	stringClassMethods,
	func(RubyClassObject, ...RubyObject) (RubyObject, error) {
		return NewString(""), nil
	},
)

func init() {
	CLASSES.Set("String", stringClass)
}

func NewString(value string) *String {
	return &String{Value: value}
}

func NewStringf(format string, args ...interface{}) *String {
	return NewString(fmt.Sprintf(format, args...))
}

type String struct {
	Value string
}

func (s *String) Inspect() string  { return s.Value }
func (s *String) Class() RubyClass { return stringClass }

func (s *String) HashKey() HashKey {
	h := fnv.New64a()
	h.Write([]byte(s.Value))
	return HashKey(h.Sum64())
}

var (
	_ RubyObject = &String{}
)

func stringify(obj RubyObject) (string, error) {
	if obj == nil {
		return "", NewTypeError(
			"can't convert nil into String",
		)
	}
	stringObj, err := Send(NewCallContext(nil, obj), "to_s", nil)
	if err != nil {
		return "", NewTypeError(
			fmt.Sprintf(
				"can't convert %s into String",
				obj.Class().Name(),
			),
		)
	}
	str, ok := stringObj.(*String)
	if !ok {
		return "", NewTypeError(
			fmt.Sprintf(
				"can't convert %s to String (%s#to_s gives %s)",
				obj.Class().Name(),
				obj.Class().Name(),
				stringObj.Class().Name(),
			),
		)
	}
	return str.Value, nil
}

var stringClassMethods = map[string]RubyMethod{}

var stringMethods = map[string]RubyMethod{
	"to_s":   withArity(0, newMethod(stringToS)),
	"+":      withArity(1, newMethod(stringAdd)),
	"gsub":   withArity(2, newMethod(stringGsub)),
	"length": withArity(0, newMethod(stringLength)),
	"size":   withArity(0, newMethod(stringLength)),
	"lines":  withArity(0, newMethod(stringLines)),
	"to_f":   withArity(0, newMethod(stringToF)),
}

func stringToS(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	str := context.Receiver().(*String)
	return NewString(str.Value), nil
}

func stringAdd(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	s := context.Receiver().(*String)
	add, ok := args[0].(*String)
	if !ok {
		return nil, NewImplicitConversionTypeError(add, args[0])
	}
	return NewString(s.Value + add.Value), nil
}

func stringGsub(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	s := context.Receiver().(*String)
	pattern, ok := args[0].(*String)
	if !ok {
		return nil, NewImplicitConversionTypeError(pattern, args[0])
	}
	replacement, ok := args[1].(*String)
	if !ok {
		return nil, NewImplicitConversionTypeError(replacement, args[1])
	}

	// Perform the gsub operation
	re, err := regexp.Compile(pattern.Value)
	if err != nil {
		return nil, NewTypeError(fmt.Sprintf("Invalid regex pattern: %s", err))
	}

	result := re.ReplaceAllString(s.Value, replacement.Value)

	// Return the modified string
	return NewString(result), nil
}

func stringLength(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	s := context.Receiver().(*String)
	return NewInteger(int64(len(s.Value))), nil
}

func stringLines(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	s := context.Receiver().(*String)
	lines := strings.Split(s.Value, "\n")
	arr := NewArray()
	for _, line := range lines {
		arr.Elements = append(arr.Elements, NewString(line+"\n"))
	}
	return arr, nil
}

var FLOAT_RE = regexp.MustCompile(`[-+]?\d*\.?\d+`)

func stringToF(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	s := context.Receiver().(*String)
	if s.Value == "" {
		return NewFloat(0.0), nil
	}
	match := FLOAT_RE.FindString(s.Value)
	if match == "" {
		return NewFloat(0.0), nil
	}
	// Convert the string to a float
	val, err := strconv.ParseFloat(match, 64)
	if err != nil {
		return nil, NewTypeError("Invalid float value: " + s.Value)
	}
	return NewFloat(val), nil
}
