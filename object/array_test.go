package object

import (
	"testing"

	"github.com/lczyk/assert"
)

func TestArrayPush(t *testing.T) {
	t.Run("one argument", func(t *testing.T) {
		array := NewArray()
		env := NewEnvironment()
		context := &callContext{
			receiver: array,
			env:      env,
		}

		result, err := arrayPush(context, nil, NewInteger(17))

		assert.NoError(t, err)
		assert.EqualCmpAny(t, result, NewArray(NewInteger(17)), CompareRubyObjectsForTests)
	})

	t.Run("more than one argument", func(t *testing.T) {
		array := NewArray()
		env := NewEnvironment()
		context := &callContext{
			receiver: array,
			env:      env,
		}

		result, err := arrayPush(context, nil, NewInteger(17), NIL, TRUE, FALSE)

		assert.NoError(t, err)
		assert.EqualCmpAny(t, result, NewArray(NewInteger(17), NIL, TRUE, FALSE), CompareRubyObjectsForTests)
	})
}

func TestArrayUnshift(t *testing.T) {
	t.Run("one argument", func(t *testing.T) {
		array := NewArray(NewString("first element"))
		env := NewEnvironment()
		context := &callContext{
			receiver: array,
			env:      env,
		}

		result, err := arrayUnshift(context, nil, NewInteger(17))

		assert.NoError(t, err)
		assert.EqualCmpAny(t, result, NewArray(NewInteger(17), NewString("first element")), CompareRubyObjectsForTests)
	})
	t.Run("more than one argument", func(t *testing.T) {
		array := NewArray(NewString("first element"))
		env := NewEnvironment()
		context := &callContext{
			receiver: array,
			env:      env,
		}

		result, err := arrayUnshift(context, nil, NewInteger(17), NIL, TRUE, FALSE)

		assert.NoError(t, err)
		assert.EqualCmpAny(t, result, NewArray(NewInteger(17), NIL, TRUE, FALSE, NewString("first element")), CompareRubyObjectsForTests)
	})
}
