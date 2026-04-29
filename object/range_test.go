package object

import (
	"testing"

	"github.com/lczyk/assert"
)

func TestRangeEvalToArray(t *testing.T) {
	t.Run("empty range", func(t *testing.T) {
		rng := &Range{
			Left:      1,
			Right:     1,
			Inclusive: false,
		}
		arr := rng.ToArray()
		assert.EqualCmpAny(t, arr, NewArray(), CompareRubyObjectsForTests)
	})
	t.Run("inclusive range", func(t *testing.T) {
		rng := &Range{
			Left:      1,
			Right:     3,
			Inclusive: true,
		}
		arr := rng.ToArray()
		expected := NewArray(
			NewInteger(1),
			NewInteger(2),
			NewInteger(3),
		)
		assert.EqualCmpAny(t, arr, expected, CompareRubyObjectsForTests)
	})
	t.Run("exclusive range", func(t *testing.T) {
		rng := &Range{
			Left:      1,
			Right:     3,
			Inclusive: false,
		}
		arr := rng.ToArray()
		expected := NewArray(
			NewInteger(1),
			NewInteger(2),
		)
		assert.EqualCmpAny(t, arr, expected, CompareRubyObjectsForTests)
	})
}
