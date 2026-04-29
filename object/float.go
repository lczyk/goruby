package object

import (
	"fmt"
	"math"
	"runtime"
	"unsafe"

	"github.com/lczyk/goruby/trace"
	"github.com/pkg/errors"
)

var floatClass RubyClassObject = newClass(
	"Float", floatMethods, nil, notInstantiatable,
)

func init() {
	CLASSES.Set("Float", floatClass)
}

// NewFloat returns a new Float with the given value
func NewFloat(value float64) *Float {
	return &Float{Value: value}
}

// Float represents an float in Ruby
type Float struct {
	Value float64
}

func (i *Float) Inspect() string  { return fmt.Sprintf("%.16f", i.Value) }
func (i *Float) Class() RubyClass { return floatClass }

func reinterpretCastFloatToUint64(value float64) uint64 {
	// reinterpret the float as uint64
	value_reinterpret := (*[8]byte)(unsafe.Pointer(&value))[:]
	result := uint64(0)
	for i := 0; i < 8; i++ {
		result |= uint64(value_reinterpret[i]) << (8 * i)
	}
	return result
}

func (i *Float) HashKey() HashKey {
	return HashKey(reinterpretCastFloatToUint64(i.Value))
}

var (
	_ RubyObject = &Float{}
)

var floatMethods = map[string]RubyMethod{
	"div":  withArity(1, newMethod(floatDiv)),
	"/":    withArity(1, newMethod(floatDiv)),
	"*":    withArity(1, newMethod(floatMul)),
	"+":    withArity(1, newMethod(floatAdd)),
	"-":    withArity(1, newMethod(floatSub)),
	"<":    withArity(1, newMethod(floatLt)),
	">":    withArity(1, newMethod(floatGt)),
	">=":   withArity(1, newMethod(floatGte)),
	"<=":   withArity(1, newMethod(floatLte)),
	"<=>":  withArity(1, newMethod(floatSpaceship)),
	"to_i": withArity(0, newMethod(floatToI)),
	"**":   withArity(1, newMethod(floatPow)),
}

func floatDiv(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	i := context.Receiver().(*Float)
	var divisor float64
	switch arg := args[0].(type) {
	case *Integer:
		divisor = float64(arg.Value)
	case *Float:
		divisor = arg.Value
	default:
		return nil, NewCoercionTypeError(args[0], i)
	}
	if divisor == 0 {
		return nil, NewZeroDivisionError()
	}
	return NewFloat(i.Value / divisor), nil
}

func floatMul(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	i := context.Receiver().(*Float)
	var factor float64
	switch arg := args[0].(type) {
	case *Integer:
		factor = float64(arg.Value)
	case *Float:
		factor = arg.Value
	default:
		return nil, NewCoercionTypeError(args[0], i)
	}

	return NewFloat(i.Value * factor), nil
}

func floatAdd(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	i := context.Receiver().(*Float)
	add, ok := args[0].(*Float)
	if !ok {
		return nil, NewCoercionTypeError(args[0], i)
	}
	return NewFloat(i.Value + add.Value), nil
}

func floatSub(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	i := context.Receiver().(*Float)
	sub, ok := args[0].(*Float)
	if !ok {
		return nil, NewCoercionTypeError(args[0], i)
	}
	return NewFloat(i.Value - sub.Value), nil
}

// Objects which can *safely* be converted to a float
func safeObjectToFloat(arg RubyObject) (float64, bool) {
	var right float64
	switch arg := arg.(type) {
	case *Float:
		right = arg.Value
	case *Integer:
		right = float64(arg.Value)
	default:
		return 0, false
	}
	return right, true
}

func callersName() string {
	parent, _, _, _ := runtime.Caller(1)
	fn := runtime.FuncForPC(parent)
	return fn.Name()
}

func floatCmpHelper(args []RubyObject) (float64, error) {
	if len(args) != 1 {
		return 0, errors.WithMessage(
			NewWrongNumberOfArgumentsError(1, len(args)),
			callersName(),
		)
	}
	right, ok := safeObjectToFloat(args[0])
	if !ok {
		return 0, errors.WithMessage(
			NewArgumentError(
				"comparison of Float with %s failed",
				args[0].Class().(RubyObject).Inspect(),
			),
			callersName(),
		)
	}
	return right, nil
}
func floatLt(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	i := context.Receiver().(*Float)
	right, err := floatCmpHelper(args)
	if err != nil {
		return nil, err
	}
	if i.Value < right {
		return TRUE, nil
	}
	return FALSE, nil
}

func floatGt(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	i := context.Receiver().(*Float)
	right, err := floatCmpHelper(args)
	if err != nil {
		return nil, err
	}
	if i.Value > right {
		return TRUE, nil
	}
	return FALSE, nil
}

func floatSpaceship(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	i := context.Receiver().(*Float)
	right, err := floatCmpHelper(args)
	if err != nil {
		return NIL, err
	}
	switch {
	case i.Value > right:
		return NewFloat(1), nil
	case i.Value < right:
		return NewFloat(-1), nil
	case i.Value == right:
		return NewFloat(0), nil
	default:
		panic("not reachable")
	}
}

func floatGte(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	i := context.Receiver().(*Float)
	right, err := floatCmpHelper(args)
	if err != nil {
		return nil, err
	}
	if i.Value >= right {
		return TRUE, nil
	}
	return FALSE, nil
}

func floatLte(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	i := context.Receiver().(*Float)
	right, err := floatCmpHelper(args)
	if err != nil {
		return nil, err
	}
	if i.Value <= right {
		return TRUE, nil
	}
	return FALSE, nil
}

func floatToI(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	i := context.Receiver().(*Float)
	return NewInteger(int64(i.Value)), nil
}

func floatPow(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	i := context.Receiver().(*Float)
	if len(args) != 1 {
		return nil, NewWrongNumberOfArgumentsError(1, len(args))
	}
	right, ok := safeObjectToFloat(args[0])
	if !ok {
		return nil, NewArgumentError(
			"comparison of Float with %s failed",
			args[0].Class().(RubyObject).Inspect(),
		)
	}
	if right < 0 {
		return nil, NewArgumentError("negative exponent")
	}
	result := math.Pow(i.Value, right)
	return NewFloat(result), nil
}
