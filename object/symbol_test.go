package object

import (
	"testing"

	"github.com/lczyk/assert"
)

func TestSymbol_hashKey(t *testing.T) {
	hello1 := NewSymbol("Hello World")
	hello2 := NewSymbol("Hello World")
	diff1 := NewSymbol("My name is johnny")
	diff2 := NewSymbol("My name is johnny")

	assert.Equal(t, hello1.HashKey(), hello2.HashKey())
	assert.Equal(t, diff1.HashKey(), diff2.HashKey())
	assert.NotEqual(t, hello1.HashKey(), diff1.HashKey())
}

func TestSymbolToS(t *testing.T) {
	context := &callContext{
		receiver: NewSymbol("foo"),
	}

	result, err := symbolToS(context, nil)

	assert.NoError(t, err)

	expected := NewString("foo")

	assert.EqualCmpAny(t, result, expected, CompareRubyObjectsForTests)
}

func TestSymbolToBool(t *testing.T) {
	t.Run("true object", func(t *testing.T) {
		val, ok := SymbolToBool(TRUE)
		assert.That(t, ok)
		assert.That(t, val)
	})

	t.Run("false object", func(t *testing.T) {
		val, ok := SymbolToBool(FALSE)
		assert.That(t, ok)
		assert.That(t, !val)
	})

	t.Run("some other symbol", func(t *testing.T) {
		val, ok := SymbolToBool(NewSymbol("foo"))
		assert.That(t, !ok)
		assert.That(t, !val)
	})

	t.Run("some non-symbol object", func(t *testing.T) {
		val, ok := SymbolToBool(NewString("foo"))
		assert.That(t, !ok)
		assert.That(t, !val)
	})

	t.Run("nil", func(t *testing.T) {
		val, ok := SymbolToBool(nil)
		assert.That(t, !ok)
		assert.That(t, !val)
	})

	t.Run("nil pointer", func(t *testing.T) {
		var b *Symbol = nil
		val, ok := SymbolToBool(b)
		assert.That(t, !ok)
		assert.That(t, !val)
	})
}
