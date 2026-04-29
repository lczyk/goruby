package object

import (
	"testing"

	"github.com/lczyk/assert"
)

func TestEnvironmentSet(t *testing.T) {
	env := &environment{store: make(map[string]RubyObject)}

	env.Set("foo", NIL)

	val, ok := env.store["foo"]
	assert.That(t, ok, "Expected store to contain 'foo'")
	assert.EqualCmpAny(t, val, NIL, CompareRubyObjectsForTests)
}

func TestEnvironmentSetGlobal(t *testing.T) {
	t.Run("toplevel env", func(t *testing.T) {
		env := &environment{store: make(map[string]RubyObject)}

		env.SetGlobal("$foo", NIL)

		val, ok := env.store["$foo"]
		assert.That(t, ok, "Expected store to contain '$foo'")
		assert.EqualCmpAny(t, val, NIL, CompareRubyObjectsForTests)
	})
	t.Run("inner env one level", func(t *testing.T) {
		outer := &environment{store: make(map[string]RubyObject)}
		env := &environment{store: make(map[string]RubyObject), outer: outer}

		env.SetGlobal("$foo", NIL)

		_, ok := env.store["$foo"]
		assert.That(t, !ok, "Expected env store to not contain '$foo'")

		val, ok := outer.store["$foo"]

		assert.That(t, ok, "Expected outer store to contain '$foo'")
		assert.EqualCmpAny(t, val, NIL, CompareRubyObjectsForTests)
	})
	t.Run("inner env two level", func(t *testing.T) {
		root := &environment{store: make(map[string]RubyObject)}
		outer := &environment{store: make(map[string]RubyObject), outer: root}
		env := &environment{store: make(map[string]RubyObject), outer: outer}

		env.SetGlobal("$foo", NIL)

		_, ok := env.store["$foo"]
		assert.That(t, !ok, "Expected env store to not contain '$foo'")

		_, ok = outer.store["$foo"]
		assert.That(t, !ok, "Expected outer store to not contain '$foo'")

		val, ok := root.store["$foo"]
		assert.That(t, ok, "Expected root store to contain '$foo'")
		assert.EqualCmpAny(t, val, NIL, CompareRubyObjectsForTests)
	})
}

func TestEnvironmentGet(t *testing.T) {
	t.Run("toplevel env", func(t *testing.T) {
		env := &environment{store: map[string]RubyObject{"foo": TRUE}}

		val, ok := env.Get("foo")
		assert.That(t, ok, "Expected store to contain 'foo'")
		assert.EqualCmpAny(t, val, TRUE, CompareRubyObjectsForTests)
	})
	t.Run("inner env one level", func(t *testing.T) {
		outer := &environment{store: map[string]RubyObject{"foo": TRUE}}
		env := &environment{store: make(map[string]RubyObject), outer: outer}

		val, ok := env.Get("foo")
		assert.That(t, ok, "Expected store to contain 'foo'")
		assert.EqualCmpAny(t, val, TRUE, CompareRubyObjectsForTests)
	})
	t.Run("inner env two level", func(t *testing.T) {
		root := &environment{store: map[string]RubyObject{"foo": TRUE}}
		outer := &environment{store: make(map[string]RubyObject), outer: root}
		env := &environment{store: make(map[string]RubyObject), outer: outer}

		val, ok := env.Get("foo")
		assert.That(t, ok, "Expected store to contain 'foo'")
		assert.EqualCmpAny(t, val, TRUE, CompareRubyObjectsForTests)
	})
	t.Run("inner env two level overridden key", func(t *testing.T) {
		root := &environment{store: map[string]RubyObject{"foo": FALSE}}
		outer := &environment{store: map[string]RubyObject{"foo": TRUE}, outer: root}
		env := &environment{store: make(map[string]RubyObject), outer: outer}

		val, ok := env.Get("foo")
		assert.That(t, ok, "Expected store to contain 'foo'")
		assert.EqualCmpAny(t, val, TRUE, CompareRubyObjectsForTests)
	})
}
