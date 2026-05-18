package pratt_test

import (
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/goruby/internal/pratt"
	"github.com/lczyk/goruby/internal/pratt/torture"
)

func BenchmarkTorture_PrecMix(b *testing.B) {
	toks := torture.Tokenise(torture.PrecMix())
	b.ReportAllocs()
	for b.Loop() {
		p := &torture.Parser{Toks: toks}
		_ = pratt.Climb(p, torture.Config, torture.PrecLowest)
	}
}

func BenchmarkTorture_CallChain(b *testing.B) {
	for _, d := range []int{1, 8, 64} {
		toks := torture.Tokenise(torture.CallChain(d))
		b.Run(itoa(d), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				p := &torture.Parser{Toks: toks}
				_ = pratt.Climb(p, torture.Config, torture.PrecLowest)
			}
		})
	}
}

func BenchmarkTorture_MemberChain(b *testing.B) {
	for _, d := range []int{1, 8, 64} {
		toks := torture.Tokenise(torture.MemberChain(d))
		b.Run(itoa(d), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				p := &torture.Parser{Toks: toks}
				_ = pratt.Climb(p, torture.Config, torture.PrecLowest)
			}
		})
	}
}

func BenchmarkTorture_PowerTower(b *testing.B) {
	for _, d := range []int{1, 8, 64} {
		toks := torture.Tokenise(torture.PowerTower(d))
		b.Run(itoa(d), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				p := &torture.Parser{Toks: toks}
				_ = pratt.Climb(p, torture.Config, torture.PrecLowest)
			}
		})
	}
}

func BenchmarkTorture_TernaryNest(b *testing.B) {
	for _, d := range []int{1, 8, 64, 256, 1024} {
		toks := torture.Tokenise(torture.TernaryNest(d))
		b.Run(itoa(d), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				p := &torture.Parser{Toks: toks}
				_ = pratt.Climb(p, torture.Config, torture.PrecLowest)
			}
		})
	}
}

func BenchmarkTorture_Pathological(b *testing.B) {
	for _, n := range []int{8, 64, 256, 1024} {
		toks := torture.Tokenise(torture.Pathological(n))
		b.Run(itoa(n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				p := &torture.Parser{Toks: toks}
				_ = pratt.Climb(p, torture.Config, torture.PrecLowest)
			}
		})
	}
}

func BenchmarkTorture_RightAssign(b *testing.B) {
	for _, d := range []int{8, 64, 256, 1024} {
		toks := torture.Tokenise(torture.RightAssign(d))
		b.Run(itoa(d), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				p := &torture.Parser{Toks: toks}
				_ = pratt.Climb(p, torture.Config, torture.PrecLowest)
			}
		})
	}
}

func BenchmarkTorture_Random(b *testing.B) {
	for _, n := range []int{8, 64, 256, 1024} {
		toks := torture.Tokenise(torture.Random(n))
		b.Run(itoa(n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				p := &torture.Parser{Toks: toks}
				_ = pratt.Climb(p, torture.Config, torture.PrecLowest)
			}
		})
	}
}

func BenchmarkTorture_Juxtapose(b *testing.B) {
	for _, n := range []int{8, 64, 256, 1024} {
		toks := torture.Tokenise(torture.Juxtapose(n))
		b.Run(itoa(n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				p := &torture.Parser{Toks: toks}
				_ = pratt.Climb(p, torture.Config, torture.PrecLowest)
			}
		})
	}
}

func BenchmarkTorture_MixedDeepNest(b *testing.B) {
	for _, d := range []int{8, 64, 256, 1024} {
		toks := torture.Tokenise(torture.MixedDeepNest(d))
		b.Run(itoa(d), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				p := &torture.Parser{Toks: toks}
				_ = pratt.Climb(p, torture.Config, torture.PrecLowest)
			}
		})
	}
}

// Sanity: torture grammar actually parses things end-to-end + hook fires.
func TestTorture_Sanity(t *testing.T) {
	src := "a = b + c * d ** e .. f ? g : h"
	p := &torture.Parser{Toks: torture.Tokenise(src)}
	got := pratt.Climb(p, torture.Config, torture.PrecLowest)
	assert.NotNil(t, got)
	assert.Equal(t, got.Op, "=")
	assert.That(t, p.Hits > 0, "hook never fired")
}
