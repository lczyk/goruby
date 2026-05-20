package object

import (
	"bytes"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/goruby/token"
)

func TestSingletonsArePointerStable(t *testing.T) {
	assert.That(t, TRUE == BooleanOf(true), "TRUE singleton")
	assert.That(t, FALSE == BooleanOf(false), "FALSE singleton")
	assert.NotNil(t, NIL)
}

func TestSmallIntCacheReturnsSamePointer(t *testing.T) {
	for _, v := range []int64{-128, -1, 0, 1, 42, 1152} {
		a := NewInteger(v)
		b := NewInteger(v)
		assert.That(t, a == b, "cached int %d should share pointer", v)
	}
	a := NewInteger(1153)
	b := NewInteger(1153)
	assert.That(t, a != b, "out-of-range int should not be cached")
	assert.Equal(t, int64(1153), a.Value)
}

func TestIntegerInspect(t *testing.T) {
	assert.Equal(t, "0", NewInteger(0).Inspect())
	assert.Equal(t, "-42", NewInteger(-42).Inspect())
	assert.Equal(t, "9999", NewInteger(9999).Inspect())
}

func TestFloatInspectAddsTrailingZero(t *testing.T) {
	assert.Equal(t, "1.0", NewFloat(1.0).Inspect())
	assert.Equal(t, "3.14", NewFloat(3.14).Inspect())
	assert.Equal(t, "-0.5", NewFloat(-0.5).Inspect())
}

func TestSymbolPoolInterns(t *testing.T) {
	p := NewSymbolPool()
	a := p.Intern("foo")
	b := p.Intern("foo")
	c := p.Intern("bar")
	assert.Equal(t, a.ID, b.ID, "same name -> same id")
	assert.NotEqual(t, a.ID, c.ID, "different names -> different ids")
	assert.Equal(t, "foo", p.Name(a.ID))
	assert.Equal(t, "bar", p.Name(c.ID))
}

func TestStringPoolInterns(t *testing.T) {
	p := NewStringPool()
	a := p.Intern("hi")
	b := p.Intern("hi")
	assert.Equal(t, a.ID, b.ID)
	assert.Equal(t, "hi", p.Get(a.ID))
}

func TestStringInspectEscapes(t *testing.T) {
	assert.Equal(t, `"hello"`, NewString("hello").Inspect())
	assert.Equal(t, `"a\nb"`, NewString("a\nb").Inspect())
	assert.Equal(t, `"q\"q"`, NewString(`q"q`).Inspect())
}

func TestArrayInspectRecurses(t *testing.T) {
	a := NewArray(NewInteger(1), NewInteger(2), NewString("x"))
	assert.Equal(t, `[1, 2, "x"]`, a.Inspect())
}

func TestHashInspectVersionAware(t *testing.T) {
	env := NewMainEnvironment()
	sym := env.Symbols().Intern("a")
	h := NewHash(HashEntry{Key: sym, Value: NewInteger(1)})

	pre34 := token.MustParseVersion("2.6")
	post34 := token.MustParseVersion("3.4")

	assert.Equal(t, "{:a=>1}", h.inspectAt(inspectCtx{v: pre34, env: env}))
	assert.Equal(t, "{a: 1}", h.inspectAt(inspectCtx{v: post34, env: env}))
}

func TestEnvironmentInspectUsesEnvVersionAndPools(t *testing.T) {
	env := NewMainEnvironment(WithVersion(token.MustParseVersion("2.6")))
	sym := env.Symbols().Intern("k")
	h := NewHash(HashEntry{Key: sym, Value: NewInteger(7)})
	assert.Equal(t, "{:k=>7}", env.Inspect(h))
}

func TestEnvironmentStdoutOverride(t *testing.T) {
	var buf bytes.Buffer
	env := NewMainEnvironment(WithStdout(&buf))
	_, err := env.Stdout().Write([]byte("hi"))
	assert.NoError(t, err)
	assert.Equal(t, "hi", buf.String())
}

func TestEnclosedEnvironmentChainsLookup(t *testing.T) {
	root := NewMainEnvironment()
	root.Set("x", NewInteger(10))
	child := NewEnclosedEnvironment(root)
	v, ok := child.Get("x")
	assert.That(t, ok)
	assert.Equal(t, int64(10), v.(*Integer).Value)
	assert.Equal(t, root.Version(), child.Version())
}
