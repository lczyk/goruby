package pratt_test

import (
	"strings"
	"testing"

	"github.com/lczyk/goruby/internal/pratt"
	"github.com/lczyk/goruby/internal/pratt/toy"
)

// Benchmarks for the pratt core via the toy calculator grammar. They measure
// the cost of the climb loop + generic dispatch, not real ruby parsing.

func benchExprFlat(n int) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = "1"
	}
	return strings.Join(parts, " + ")
}

func benchExprMixed(n int) string {
	unit := "1 + 2 * 3 - 4 / 5"
	parts := make([]string, n)
	for i := range parts {
		parts[i] = unit
	}
	return strings.Join(parts, " + ")
}

func benchExprNested(depth int) string {
	var b strings.Builder
	for range depth {
		b.WriteString("( ")
	}
	b.WriteString("1 + 2")
	for range depth {
		b.WriteString(" )")
	}
	return b.String()
}

func BenchmarkClimb_Flat(b *testing.B) {
	for _, n := range []int{1, 8, 64, 512} {
		toks := toy.Tokenise(benchExprFlat(n))
		b.Run(itoa(n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				p := &toy.Parser{Toks: toks}
				_ = pratt.Climb(p, toy.Config, toy.PrecLowest)
			}
		})
	}
}

func BenchmarkClimb_Mixed(b *testing.B) {
	for _, n := range []int{1, 8, 64} {
		toks := toy.Tokenise(benchExprMixed(n))
		b.Run(itoa(n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				p := &toy.Parser{Toks: toks}
				_ = pratt.Climb(p, toy.Config, toy.PrecLowest)
			}
		})
	}
}

func BenchmarkClimb_Nested(b *testing.B) {
	for _, d := range []int{1, 8, 64} {
		toks := toy.Tokenise(benchExprNested(d))
		b.Run(itoa(d), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				p := &toy.Parser{Toks: toks}
				_ = pratt.Climb(p, toy.Config, toy.PrecLowest)
			}
		})
	}
}

// itoa avoids fmt.Sprintf alloc in sub-bench names.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
