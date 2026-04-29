package object

import (
	"testing"

	"github.com/lczyk/assert"
)

func TestBottomIsNil(t *testing.T) {
	context := &callContext{receiver: TRUE}
	result, err := bottomIsNil(context, nil)

	assert.NoError(t, err)

	boolean, ok := SymbolToBool(result)
	assert.That(t, ok, "Expected boolean, got %T", result)
	assert.That(t, !boolean, "Expected false, got true")
}

func TestBottomToS(t *testing.T) {
	t.Run("object as receiver", func(t *testing.T) {
		context := &callContext{receiver: &Bottom{}}
		result, err := bottomToS(context, nil)
		assert.NoError(t, err)
		assert.EqualCmpAny(t, result, NewStringf("#<Bottom:%p>", context.receiver), CompareRubyObjectsForTests)
	})
	t.Run("self object as receiver", func(t *testing.T) {
		self := &Bottom{}
		context := &callContext{receiver: self}
		result, err := bottomToS(context, nil)
		assert.NoError(t, err)
		assert.EqualCmpAny(t, result, NewStringf("#<Bottom:%p>", self), CompareRubyObjectsForTests)
	})
}

func TestBottomRaise(t *testing.T) {
	object := &Bottom{}
	env := NewMainEnvironment()
	context := &callContext{
		receiver: object,
		env:      env,
	}

	t.Run("without args", func(t *testing.T) {
		result, err := bottomRaise(context, nil)
		assert.EqualCmpAny(t, result, nil, CompareRubyObjectsForTests)
		assert.Error(t, err, NewRuntimeError(""))
	})

	t.Run("with 1 arg", func(t *testing.T) {
		t.Run("string argument", func(t *testing.T) {
			result, err := bottomRaise(context, nil, NewString("ouch"))
			assert.EqualCmpAny(t, result, nil, CompareRubyObjectsForTests)
			assert.Error(t, err, NewRuntimeError("ouch"))
		})
		t.Run("integer argument", func(t *testing.T) {
			obj := NewInteger(5)
			result, err := bottomRaise(context, nil, obj)
			assert.EqualCmpAny(t, result, nil, CompareRubyObjectsForTests)
			assert.Error(t, err, NewRuntimeError("%s", obj.Inspect()))
		})
	})
}
