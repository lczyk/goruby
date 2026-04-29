package object

import (
	"reflect"
	"testing"

	"github.com/MarcinKonowalczyk/goruby/trace"
	"github.com/lczyk/assert"
	"github.com/pkg/errors"
)

type testRubyObject struct {
	class RubyClassObject
	Name  string
}

func (t *testRubyObject) Inspect() string { return "TEST OBJECT" }
func (t *testRubyObject) Class() RubyClass {
	if t.class != nil {
		return t.class
	}
	return bottomClass
}
func (t *testRubyObject) HashKey() HashKey {
	return HashKey(99)
}

func TestSend(t *testing.T) {
	// }
	methods := map[string]RubyMethod{
		"a_method": newMethod(func(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
			return TRUE, nil
		}),
		"another_method": newMethod(func(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
			return FALSE, nil
		}),
	}
	t.Run("normal object as context", func(t *testing.T) {
		context := &callContext{
			receiver: &testRubyObject{
				class: &class{
					name:            "base class",
					instanceMethods: NewMethodSet(methods),
				},
			},
		}

		tests := []struct {
			method         string
			expectedResult RubyObject
			expectedError  error
		}{
			{
				"a_method",
				TRUE,
				nil,
			},
			{
				"another_method",
				FALSE,
				nil,
			},
			{
				"unknown_method",
				nil,
				NewNoMethodError(context.receiver, "unknown_method"),
			},
		}

		for _, testCase := range tests {
			result, err := Send(context, testCase.method, nil)

			assert.Error(t, errors.Cause(err), testCase.expectedError)
			assert.EqualCmpAny(t, result, testCase.expectedResult, CompareRubyObjectsForTests)
		}
	})
}

func TestAddMethod(t *testing.T) {
	t.Run("vanilla object", func(t *testing.T) {
		context := &testRubyObject{
			class: &class{
				name:            "base class",
				instanceMethods: NewMethodSet(map[string]RubyMethod{}),
			},
		}

		fn := &Function{
			Parameters: []*FunctionParameter{
				{Name: "x"},
			},
			Env:  &environment{store: map[string]RubyObject{}},
			Body: nil,
		}

		newContext, _ := AddMethod(context, "foo", fn)

		_, ok := newContext.Class().Methods().Get("foo")
		assert.That(t, ok, "Expected object to have method foo")
	})
	t.Run("class object", func(t *testing.T) {
		context := &class{
			name:            "A",
			instanceMethods: NewMethodSet(map[string]RubyMethod{}),
		}

		fn := &Function{
			Parameters: []*FunctionParameter{
				{Name: "x"},
			},
			Env:  &environment{store: map[string]RubyObject{}},
			Body: nil,
		}

		newContext, _ := AddMethod(context, "foo", fn)

		class, ok := newContext.(*class)
		assert.That(t, ok, "Expected returned object to be a class, got %T", newContext)

		_, ok = class.Methods().Get("foo")
		assert.That(t, ok, "Expected object to have method foo")
	})
	t.Run("extended object", func(t *testing.T) {
		context := newExtendedObject(
			&testRubyObject{
				class: &class{
					name:            "base class",
					instanceMethods: NewMethodSet(map[string]RubyMethod{}),
				},
			},
		)

		context.eigenclass.addMethod("bar", newMethod(func(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
			return NIL, nil
		}))

		fn := &Function{
			Parameters: []*FunctionParameter{
				{Name: "x"},
			},
			Env:  &environment{store: map[string]RubyObject{}},
			Body: nil,
		}

		newContext, _ := AddMethod(context, "foo", fn)

		_, ok := newContext.Class().Methods().Get("foo")
		assert.That(t, ok, "Expected object to have method foo")

		_, ok = newContext.Class().Methods().Get("bar")
		assert.That(t, ok, "Expected object to have method bar")
	})
	t.Run("vanilla self object", func(t *testing.T) {
		vanillaObject := &testRubyObject{
			class: &class{
				name:            "base class",
				instanceMethods: NewMethodSet(map[string]RubyMethod{}),
			},
			Name: "main",
		}
		context := vanillaObject

		fn := &Function{
			Parameters: []*FunctionParameter{
				{Name: "x"},
			},
			Env:  &environment{store: map[string]RubyObject{}},
			Body: nil,
		}

		newContext, extended := AddMethod(context, "foo", fn)

		_, ok := newContext.Class().Methods().Get("foo")
		assert.That(t, ok, "Expected object to have method foo")
		assert.That(t, extended, "Expected object to be extended")

		returnPointer := reflect.ValueOf(newContext).Pointer()
		contextPointer := reflect.ValueOf(context).Pointer()
		assert.NotEqual(t, returnPointer, contextPointer)

		extendedRubyObject := newContext.(*extendedObject).RubyObject
		assert.EqualCmpAny(t, vanillaObject, extendedRubyObject, CompareRubyObjectsForTests)
	})
	t.Run("extended self object", func(t *testing.T) {
		context := newExtendedObject(
			&testRubyObject{
				class: &class{
					name:            "base class",
					instanceMethods: NewMethodSet(map[string]RubyMethod{}),
				},
			},
		)

		context.eigenclass.addMethod("bar", newMethod(func(context CallContext, tracer trace.Tracer, args ...RubyObject) (RubyObject, error) {
			return NIL, nil
		}))

		fn := &Function{
			Parameters: []*FunctionParameter{
				{Name: "x"},
			},
			Env:  &environment{store: map[string]RubyObject{}},
			Body: nil,
		}

		newContext, extended := AddMethod(context, "foo", fn)

		_, ok := newContext.Class().Methods().Get("foo")
		assert.That(t, ok, "Expected object to have method foo")

		_, ok = newContext.Class().Methods().Get("bar")
		assert.That(t, ok, "Expected object to have method bar")

		returnPointer := reflect.ValueOf(newContext).Pointer()
		contextPointer := reflect.ValueOf(context).Pointer()

		assert.That(t, !extended, "Expected object not to be extended")
		assert.Equal(t, returnPointer, contextPointer)
	})
}
