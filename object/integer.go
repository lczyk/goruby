package object

import (
	"fmt"
	"math"

	"github.com/lczyk/goruby/trace"
	"github.com/pkg/errors"
)

var integerClass RubyClassObject = newClass(
	"Integer", integerMethods, nil, notInstantiatable,
)

func init() {
	CLASSES.Set("Integer", integerClass)
}

// NewInteger returns a new Integer with the given value
func NewInteger(value int64) *Integer {
	return &Integer{Value: value}
}

// Integer represents an integer in Ruby
type Integer struct {
	Value int64
}

// Inspect returns the value as string
func (i *Integer) Inspect() string { return fmt.Sprintf("%d", i.Value) }

// Class returns integerClass
func (i *Integer) Class() RubyClass { return integerClass }

var (
	_ RubyObject = &Integer{}
)

func (i *Integer) HashKey() HashKey {
	return HashKey(uint64(i.Value))
}

var integerMethods = map[string]RubyMethod{
	"div":  withArity(1, newMethod(integerDiv)),
	"/":    withArity(1, newMethod(integerDiv)),
	"*":    withArity(1, newMethod(integerMul)),
	"+":    withArity(1, newMethod(integerAdd)),
	"-":    withArity(1, newMethod(integerSub)),
	"%":    withArity(1, newMethod(integerModulo)),
	"<":    withArity(1, newMethod(integerLt)),
	">":    withArity(1, newMethod(integerGt)),
	">=":   withArity(1, newMethod(integerGte)),
	"<=":   withArity(1, newMethod(integerLte)),
	"<=>":  withArity(1, newMethod(integerSpaceship)),
	"to_i": withArity(0, newMethod(integerToI)),
	"**":   withArity(1, newMethod(integerPow)),
	"chr":  withArity(0, newMethod(integerChr)),
}

func integerDiv(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	i := context.Receiver().(*Integer)
	divisor, ok := args[0].(*Integer)
	if !ok {
		return nil, NewCoercionTypeError(args[0], i)
	}
	if divisor.Value == 0 {
		return nil, NewZeroDivisionError()
	}
	return NewInteger(i.Value / divisor.Value), nil
}

func integerMul(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	i := context.Receiver().(*Integer)
	factor, ok := args[0].(*Integer)
	if !ok {
		return nil, NewCoercionTypeError(args[0], i)
	}
	return NewInteger(i.Value * factor.Value), nil
}

func integerAdd(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	i := context.Receiver().(*Integer)
	add, ok := args[0].(*Integer)
	if !ok {
		return nil, NewCoercionTypeError(args[0], i)
	}
	return NewInteger(i.Value + add.Value), nil
}

func integerSub(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	i := context.Receiver().(*Integer)
	sub, ok := args[0].(*Integer)
	if !ok {
		return nil, NewCoercionTypeError(args[0], i)
	}
	return NewInteger(i.Value - sub.Value), nil
}

// Objects which can *safely* be converted to an integer
func safeObjectToInteger(arg RubyObject) (int64, bool) {
	var right int64
	switch arg := arg.(type) {
	case *Integer:
		right = int64(arg.Value)
	// case *Boolean:
	// 	if arg.Value {
	// 		right = 1
	// 	} else {
	// 		right = 0
	// 	}
	default:
		return 0, false
	}
	return right, true
}

func integerCmpHelper(args []RubyObject) (int64, error) {
	right, ok := safeObjectToInteger(args[0])
	if !ok {
		return 0, errors.WithMessage(
			NewArgumentError(
				"comparison of Integer with %s failed",
				args[0].Class().(RubyObject).Inspect(),
			),
			callersName(),
		)
	}
	return right, nil
}

func integerModulo(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	i := context.Receiver().(*Integer)
	right, err := integerCmpHelper(args)
	if err != nil {
		return nil, err
	}
	return NewInteger(i.Value % right), nil
}

func integerLt(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	i := context.Receiver().(*Integer)
	right, err := integerCmpHelper(args)
	if err != nil {
		return nil, err
	}
	if i.Value < right {
		return TRUE, nil
	}
	return FALSE, nil
}

func integerGt(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	i := context.Receiver().(*Integer)
	right, err := integerCmpHelper(args)
	if err != nil {
		return nil, err
	}
	if i.Value > right {
		return TRUE, nil
	}
	return FALSE, nil
}

func integerSpaceship(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	i := context.Receiver().(*Integer)
	right, err := integerCmpHelper(args)
	if err != nil {
		return NIL, err
	}
	switch {
	case i.Value > right:
		return NewInteger(1), nil
	case i.Value < right:
		return NewInteger(-1), nil
	case i.Value == right:
		return NewInteger(0), nil
	default:
		panic("not reachable")
	}
}

func integerGte(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	i := context.Receiver().(*Integer)
	right, err := integerCmpHelper(args)
	if err != nil {
		return NIL, err
	}
	if i.Value >= right {
		return TRUE, nil
	}
	return FALSE, nil
}

func integerLte(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	i := context.Receiver().(*Integer)
	right, err := integerCmpHelper(args)
	if err != nil {
		return NIL, err
	}
	if i.Value <= right {
		return TRUE, nil
	}
	return FALSE, nil
}

func integerToI(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	i := context.Receiver().(*Integer)
	return i, nil
}

func integerPow(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	i := context.Receiver().(*Integer)
	switch arg := args[0].(type) {
	case *Integer:
		result := int64(1)
		for j := int64(0); j < arg.Value; j++ {
			result *= i.Value
		}
		return NewInteger(result), nil
	case *Float:
		result := math.Pow(float64(i.Value), arg.Value)
		return NewFloat(result), nil
	default:
		return nil, NewCoercionTypeError(args[0], i)
	}
}

func integerChr(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	i := context.Receiver().(*Integer)
	if i.Value < 0 || i.Value > 255 {
		return nil, NewArgumentError("chr out of range")
	}
	// return NewString(string(i.Value)), nil
	return NewString(string(rune(i.Value))), nil
}
