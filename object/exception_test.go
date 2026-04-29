package object

import (
	"testing"

	"github.com/lczyk/assert"
)

func TestExceptionInitialize(t *testing.T) {
	context := &callContext{
		receiver: &Exception{},
		env:      NewMainEnvironment(),
	}
	t.Run("without args", func(t *testing.T) {
		result, err := exceptionInitialize(context, nil)

		assert.NoError(t, err)

		assert.EqualCmpAny(t, result, &Exception{message: "Exception"}, CompareRubyObjectsForTests)
	})
	t.Run("with arg", func(t *testing.T) {
		t.Run("string", func(t *testing.T) {
			result, err := exceptionInitialize(context, nil, NewString("err"))

			assert.NoError(t, err)

			assert.EqualCmpAny(t, result, &Exception{message: "err"}, CompareRubyObjectsForTests)
		})
		t.Run("other object", func(t *testing.T) {
			result, err := exceptionInitialize(context, nil, NewSymbol("symbol"))

			assert.NoError(t, err)

			assert.EqualCmpAny(t, result, &Exception{message: "symbol"}, CompareRubyObjectsForTests)
		})
	})
}

func TestExceptionException(t *testing.T) {
	contextObject := &Exception{message: "x"}
	context := &callContext{
		receiver: contextObject,
		env:      NewMainEnvironment(),
	}
	t.Run("without args", func(t *testing.T) {
		result, err := exceptionException(context, nil)

		assert.NoError(t, err)
		assert.EqualCmpAny(t, result, contextObject, CompareRubyObjectsForTests)
		assert.EqualCmpAny(t, result, &Exception{message: "x"}, CompareRubyObjectsForTests)
	})
	t.Run("with arg", func(t *testing.T) {
		result, err := exceptionException(context, nil, NewString("x"))

		assert.NoError(t, err)
		assert.EqualCmpAny(t, result, contextObject, CompareRubyObjectsForTests)
		assert.EqualCmpAny(t, result, &Exception{message: "x"}, CompareRubyObjectsForTests)
	})
	t.Run("with arg but different message", func(t *testing.T) {
		result, err := exceptionException(context, nil, NewString("err"))

		assert.NoError(t, err)

		assert.EqualCmpAny(t, result, &Exception{message: "err"}, CompareRubyObjectsForTests)
	})
}

func TestExceptionToS(t *testing.T) {
	contextObject := &Exception{message: "x"}
	context := &callContext{
		receiver: contextObject,
		env:      NewMainEnvironment(),
	}

	result, err := exceptionToS(context, nil)

	assert.NoError(t, err)

	assert.EqualCmpAny(t, result, NewString("x"), CompareRubyObjectsForTests)
}
