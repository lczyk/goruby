package pratt_test

import (
	"fmt"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/goruby/internal/pratt"
	"github.com/lczyk/goruby/internal/pratt/toy"
	"github.com/lczyk/goruby/token"
)

func TestPratt_Climb(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{"1", "1"},
		{"1 + 2", "(+ 1 2)"},
		{"1 + 2 + 3", "(+ (+ 1 2) 3)"},
		{"1 + 2 * 3", "(+ 1 (* 2 3))"},
		{"1 * 2 + 3", "(+ (* 1 2) 3)"},
		{"1 * 2 * 3", "(* (* 1 2) 3)"},
		{"1 - 2 - 3", "(- (- 1 2) 3)"},
		{"- 1", "(- 1)"},
		{"- 1 + 2", "(+ (- 1) 2)"},
		{"! 1", "(! 1)"},
		{"( 1 + 2 ) * 3", "(* (+ 1 2) 3)"},
		{"1 * ( 2 + 3 )", "(* 1 (+ 2 3))"},
		{"1 + 2 * 3 - 4 / 2", "(- (+ 1 (* 2 3)) (/ 4 2))"},
	}
	for _, tc := range cases {
		t.Run(tc.src, func(t *testing.T) {
			assert.Equal(t, toy.Parse(tc.src), tc.want)
		})
	}
}

func TestPratt_NoPrefix(t *testing.T) {
	var gotType token.Type
	cfg := &pratt.Config[toy.Parser, string]{
		CurType:  func(p *toy.Parser) token.Type { return p.Toks[p.Pos].T },
		PeekType: func(p *toy.Parser) token.Type { return p.Toks[p.Pos+1].T },
		Advance:  func(p *toy.Parser) { p.Pos++ },
		PeekPrec: func(p *toy.Parser) int { return toy.PrecLowest },
		OnNoPrefix: func(_ *toy.Parser, t token.Type) {
			gotType = t
		},
	}
	p := &toy.Parser{Toks: []toy.Tok{{T: token.PLUS, Lit: "+"}, {T: token.EOF}}}
	got := pratt.Climb(p, cfg, toy.PrecLowest)
	assert.Equal(t, got, "")
	assert.Equal(t, gotType, token.PLUS)
}

// TestPratt_Hook_Stop verifies the Hook can short-circuit the loop.
func TestPratt_Hook_Stop(t *testing.T) {
	cfg := *toy.Config
	cfg.Hook = func(p *toy.Parser, left string) pratt.HookResult[string] {
		if p.Toks[p.Pos+1].T == token.SEMICOLON {
			return pratt.HookResult[string]{Stop: true}
		}
		return pratt.HookResult[string]{}
	}
	// 1 + 2 ; * 3   -> hook stops at `;`, result is (+ 1 2)
	p := &toy.Parser{Toks: []toy.Tok{
		{T: token.INT, Lit: "1"}, {T: token.PLUS, Lit: "+"}, {T: token.INT, Lit: "2"},
		{T: token.SEMICOLON, Lit: ";"}, {T: token.ASTERISK, Lit: "*"}, {T: token.INT, Lit: "3"},
		{T: token.EOF},
	}}
	got := pratt.Climb(p, &cfg, toy.PrecLowest)
	assert.Equal(t, got, "(+ 1 2)")
}

// TestPratt_Hook_Handled verifies the Hook can replace left + skip default
// infix dispatch.
func TestPratt_Hook_Handled(t *testing.T) {
	cfg := *toy.Config
	cfg.Hook = func(p *toy.Parser, left string) pratt.HookResult[string] {
		if p.Toks[p.Pos+1].T == token.INT {
			right := p.Toks[p.Pos+1].Lit
			p.Pos++
			return pratt.HookResult[string]{
				Replace: fmt.Sprintf("(* %s %s)", left, right),
				Handled: true,
			}
		}
		return pratt.HookResult[string]{}
	}
	// PeekPrec must return > minPrec for the loop to enter the hook.
	cfg.PeekPrec = func(p *toy.Parser) int {
		if p.Toks[p.Pos+1].T == token.INT {
			return toy.PrecMulDiv
		}
		if v, ok := toy.Prec[p.Toks[p.Pos+1].T]; ok {
			return v
		}
		return toy.PrecLowest
	}
	// 2 3   -> implicit-mul -> (* 2 3)
	p := &toy.Parser{Toks: []toy.Tok{
		{T: token.INT, Lit: "2"}, {T: token.INT, Lit: "3"}, {T: token.EOF},
	}}
	got := pratt.Climb(p, &cfg, toy.PrecLowest)
	assert.Equal(t, got, "(* 2 3)")
}
