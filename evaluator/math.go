package evaluator

import (
	"math"

	"github.com/lczyk/goruby/object"
)

// bootstrapMathModule installs the Math module with the common
// constants and methods used by the corpus / tests.
func bootstrapMathModule(env *object.Environment) *object.Class {
	if existing, ok := env.Get("Math"); ok {
		if c, ok := existing.(*object.Class); ok {
			return c
		}
	}
	m := object.NewClass("Math", nil)
	m.IsModule = true
	m.Constants["PI"] = object.NewFloat(math.Pi)
	m.Constants["E"] = object.NewFloat(math.E)
	m.ClassMethods["sqrt"] = &object.UserMethod{Name: "sqrt", Body: mathFn1{fn: math.Sqrt}}
	m.ClassMethods["sin"] = &object.UserMethod{Name: "sin", Body: mathFn1{fn: math.Sin}}
	m.ClassMethods["cos"] = &object.UserMethod{Name: "cos", Body: mathFn1{fn: math.Cos}}
	m.ClassMethods["tan"] = &object.UserMethod{Name: "tan", Body: mathFn1{fn: math.Tan}}
	m.ClassMethods["log"] = &object.UserMethod{Name: "log", Body: mathFn1{fn: math.Log}}
	m.ClassMethods["log2"] = &object.UserMethod{Name: "log2", Body: mathFn1{fn: math.Log2}}
	m.ClassMethods["log10"] = &object.UserMethod{Name: "log10", Body: mathFn1{fn: math.Log10}}
	m.ClassMethods["exp"] = &object.UserMethod{Name: "exp", Body: mathFn1{fn: math.Exp}}
	m.ClassMethods["atan"] = &object.UserMethod{Name: "atan", Body: mathFn1{fn: math.Atan}}
	m.ClassMethods["floor"] = &object.UserMethod{Name: "floor", Body: mathFn1{fn: math.Floor}}
	m.ClassMethods["ceil"] = &object.UserMethod{Name: "ceil", Body: mathFn1{fn: math.Ceil}}
	m.ClassMethods["hypot"] = &object.UserMethod{Name: "hypot", Body: mathFn2{fn: math.Hypot}}
	m.ClassMethods["atan2"] = &object.UserMethod{Name: "atan2", Body: mathFn2{fn: math.Atan2}}
	m.ClassMethods["pow"] = &object.UserMethod{Name: "pow", Body: mathFn2{fn: math.Pow}}
	env.SetGlobal("Math", m)
	return m
}

// mathFn1 / mathFn2 are body sentinels for one- and two-arg Math
// callouts. dispatchAttrMarker handles them.
type mathFn1 struct{ fn func(float64) float64 }
type mathFn2 struct{ fn func(float64, float64) float64 }
