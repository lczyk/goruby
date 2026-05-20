package object

import (
	"strconv"
	"testing"
)

// BenchmarkSymbolPoolInternHit measures the steady-state intern path
// where every name is already in the pool -- the common case in a
// running interpreter where most symbols are interned at boot.
func BenchmarkSymbolPoolInternHit(b *testing.B) {
	pool := NewSymbolPool()
	names := []string{"a", "b", "foo", "bar", "puts", "to_s", "inspect", "each", "map", "include?"}
	for _, n := range names {
		pool.Intern(n)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = pool.Intern(names[i%len(names)])
	}
}

// BenchmarkSymbolPoolInternMiss measures the fresh-intern path with a
// distinct name on every iteration. Dominated by the map insert and
// the byID slice append.
func BenchmarkSymbolPoolInternMiss(b *testing.B) {
	pool := NewSymbolPool()
	names := make([]string, b.N)
	for i := range names {
		names[i] = "sym_" + strconv.Itoa(i)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = pool.Intern(names[i])
	}
}

// BenchmarkStringPoolInternHit mirrors the symbol-pool hit benchmark
// for the frozen-string pool.
func BenchmarkStringPoolInternHit(b *testing.B) {
	pool := NewStringPool()
	vals := []string{"", "hello", "world", "a longer literal that is still smallish", "x"}
	for _, v := range vals {
		pool.Intern(v)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = pool.Intern(vals[i%len(vals)])
	}
}

// BenchmarkSendHit measures Send's hot path: cls.LookupMethod finds
// the method on the receiver's own class, no super walk needed.
func BenchmarkSendHit(b *testing.B) {
	c := NewClass("Bench", ObjectClass)
	c.AddMethod("noop", &BuiltinMethod{
		Name: "noop",
		Fn: func(env *Environment, recv RubyObject, args []RubyObject, block any) (RubyObject, error) {
			return recv, nil
		},
	})
	inst := NewInstance(c)
	env := NewMainEnvironment()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = Send(env, inst, "noop", nil, nil)
	}
}

// BenchmarkSendSuperWalk measures Send when the method lives several
// levels up the inheritance chain. Stresses LookupMethod's loop.
func BenchmarkSendSuperWalk(b *testing.B) {
	root := NewClass("L0", ObjectClass)
	root.AddMethod("deep", &BuiltinMethod{
		Name: "deep",
		Fn: func(env *Environment, recv RubyObject, args []RubyObject, block any) (RubyObject, error) {
			return recv, nil
		},
	})
	cur := root
	for i := 1; i < 8; i++ {
		cur = NewClass("L"+strconv.Itoa(i), cur)
	}
	inst := NewInstance(cur)
	env := NewMainEnvironment()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = Send(env, inst, "deep", nil, nil)
	}
}

// BenchmarkSendMiss measures the NoMethodError path -- LookupMethod
// walks the whole chain twice (once for the name, once for
// method_missing) before returning found=false.
func BenchmarkSendMiss(b *testing.B) {
	c := NewClass("Bench", ObjectClass)
	inst := NewInstance(c)
	env := NewMainEnvironment()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = Send(env, inst, "nonexistent", nil, nil)
	}
}

// BenchmarkLookupMethodIncludeChain stresses the include-walk by
// putting the target method behind a Module include several layers
// deep.
func BenchmarkLookupMethodIncludeChain(b *testing.B) {
	mod := NewClass("M", nil)
	mod.IsModule = true
	mod.AddMethod("via_module", &BuiltinMethod{
		Name: "via_module",
		Fn: func(env *Environment, recv RubyObject, args []RubyObject, block any) (RubyObject, error) {
			return recv, nil
		},
	})
	c := NewClass("WithInclude", ObjectClass)
	c.Includes = append(c.Includes, mod)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = c.LookupMethod("via_module")
	}
}

// BenchmarkNewInstance covers the per-call object cost: struct alloc
// plus the empty Ivars map.
func BenchmarkNewInstance(b *testing.B) {
	c := NewClass("Bench", ObjectClass)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = NewInstance(c)
	}
}

// BenchmarkNewArraySmall sizes the alloc baseline for the most common
// short-array shape (literal `[a, b, c]`).
func BenchmarkNewArraySmall(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = NewArray(NIL, NIL, NIL)
	}
}

// BenchmarkNewHashSmall sizes the alloc baseline for a small literal
// hash.
func BenchmarkNewHashSmall(b *testing.B) {
	k1 := NewString("a")
	k2 := NewString("b")
	k3 := NewString("c")
	v := NIL
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = NewHash(
			HashEntry{Key: k1, Value: v},
			HashEntry{Key: k2, Value: v},
			HashEntry{Key: k3, Value: v},
		)
	}
}

// BenchmarkEnvironmentGetLocal measures the cheapest Get: name resolves
// in the local scope without an outer walk.
func BenchmarkEnvironmentGetLocal(b *testing.B) {
	env := NewMainEnvironment()
	env.Set("x", NewInteger(42))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = env.Get("x")
	}
}

// BenchmarkEnvironmentGetOuter measures Get with several enclosing
// scopes -- the cost the block/method dispatchers pay on every local
// lookup.
func BenchmarkEnvironmentGetOuter(b *testing.B) {
	root := NewMainEnvironment()
	root.Set("x", NewInteger(42))
	cur := root
	for i := 0; i < 6; i++ {
		cur = NewEnclosedEnvironment(cur)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = cur.Get("x")
	}
}

// BenchmarkEnvironmentSetLocal stresses the local-scope write path
// (map assign).
func BenchmarkEnvironmentSetLocal(b *testing.B) {
	env := NewMainEnvironment()
	v := NewInteger(1)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		env.Set("x", v)
	}
}

// BenchmarkIsAncestorDeep walks a multi-level inheritance chain to
// the root looking for an ancestor that doesn't exist -- the
// pessimistic path through IsAncestor.
func BenchmarkIsAncestorDeep(b *testing.B) {
	target := NewClass("Other", ObjectClass)
	cur := NewClass("L0", ObjectClass)
	for i := 1; i < 8; i++ {
		cur = NewClass("L"+strconv.Itoa(i), cur)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cur.IsAncestor(target)
	}
}
