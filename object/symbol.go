package object

import (
	"fmt"
	"hash/fnv"

	"github.com/lczyk/goruby/trace"
)

var (
	symbolClass RubyClassObject = nil_class
	// TRUE  RubyObject = NewSymbol("true")
	// FALSE RubyObject = NewSymbol("false")
	// NIL   RubyObject = NewSymbol("nil")

	// unique symbols
	TRUE  RubyObject = &Symbol{Value: "true"}
	FALSE RubyObject = &Symbol{Value: "false"}
	NIL   RubyObject = &Symbol{Value: "nil"}
	FUNCS RubyObject = &Symbol{Value: "funcs"}
)

// instantiate the symbol class
func initSymbolClass() {
	symbolClass = newClass(
		"Symbol",
		symbolMethods,
		symbolClassMethods,
		notInstantiatable, // not instantiatable through new
	)
}

func init() {
	initSymbolClass()
	CLASSES.Set("Symbol", symbolClass)
}

func NewSymbol(value string) *Symbol {
	switch value {
	case "true":
		return TRUE.(*Symbol)
	case "false":
		return FALSE.(*Symbol)
	case "nil":
		return NIL.(*Symbol)
	case "funcs":
		return FUNCS.(*Symbol)
	default:
		return &Symbol{Value: value}
	}
}

type Symbol struct {
	Value string
}

func (s *Symbol) Inspect() string { return ":" + s.Value }

func (s *Symbol) Class() RubyClass {
	if symbolClass == nil {
		panic("symbolClass is nil")
	}
	if symbolClass == nil_class {
		initSymbolClass()
		if symbolClass == nil_class {
			panic("symbolClass is *still* nil_class")
		}
	}
	return symbolClass
}

func (s *Symbol) HashKey() HashKey {
	h := fnv.New64a()
	h.Write([]byte(s.Value))
	return HashKey(h.Sum64())
}

var (
	_ RubyObject = &Symbol{}
	_ RubyClass  = symbolClass
)

var symbolClassMethods = map[string]RubyMethod{}

var symbolMethods = map[string]RubyMethod{
	"to_s": withArity(0, newMethod(symbolToS)),
	"to_i": withArity(0, newMethod(symbolToI)),
	"size": withArity(0, newMethod(symbolSize)),
}

func symbolToS(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	if sym, ok := context.Receiver().(*Symbol); ok {
		return NewString(sym.Value), nil
	}
	return nil, nil
}

func symbolSize(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	if sym, ok := context.Receiver().(*Symbol); ok {
		return NewInteger(int64(len(sym.Value))), nil
	}
	return nil, nil
}

func symbolToI(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
	if tracer != nil {
		defer tracer.Un(tracer.Trace(trace.Here()))
	}
	if boolean, ok := SymbolToBool(context.Receiver()); ok {
		if boolean {
			return NewInteger(1), nil
		}
		return NewInteger(0), nil
	} else {
		return nil, NewTypeError("Symbol to_i: expected boolean")
	}
}

// If a symbol is "true" or "false", it returns the corresponding boolean value.
// Otherwise, it returns false and ok as false.
func SymbolToBool(o RubyObject) (val bool, ok bool) {
	if sym, ok := o.(*Symbol); ok {
		if sym == nil {
			// nil pointer, not ok
			return false, false
		}
		if sym.Value == "true" {
			return true, true
		} else if sym.Value == "false" {
			return false, true
		} else {
			return false, false
		}
	}
	return false, false
}

// If a symbol is "nil", returns ok as 'true'.
func SymbolToNil(o RubyObject) (ok bool) {
	if sym, ok := o.(*Symbol); ok {
		if sym == nil {
			// nil pointer, not ok
			// *must be a ruby object!*
			return false
		}
		if sym.Value == "nil" {
			return true
		} else {
			fmt.Println("SymbolToNil: unknown symbol value:", sym.Value)
			return false
		}
	}
	return false
}
