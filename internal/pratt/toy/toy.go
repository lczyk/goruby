// Package toy provides a minimal calculator grammar used to exercise the
// pratt core in isolation, independent of ruby semantics.
//
// Token types are borrowed from goruby's token package since pratt is sized
// against token.TypeMax.
//
// Supported:
//   - INT literals
//   - prefix: + - !
//   - infix:  + - * /
//   - grouping: ( expr )
//
// N is string; each parse fn returns an s-expression so test assertions are
// just string comparisons.
package toy

import (
	"fmt"
	"strings"

	"github.com/lczyk/goruby/internal/pratt"
	"github.com/lczyk/goruby/token"
)

// Precedence levels.
const (
	PrecLowest = 1
	PrecAddSub = 2
	PrecMulDiv = 3
	PrecPrefix = 4
)

// Prec maps operator token types to their precedence.
var Prec = map[token.Type]int{
	token.PLUS:     PrecAddSub,
	token.MINUS:    PrecAddSub,
	token.ASTERISK: PrecMulDiv,
	token.SLASH:    PrecMulDiv,
}

// Tok is one toy token.
type Tok struct {
	T   token.Type
	Lit string
}

// Parser is the toy grammar's parser state.
type Parser struct {
	Toks []Tok
	Pos  int
}

func (p *Parser) cur() Tok  { return p.Toks[p.Pos] }
func (p *Parser) peek() Tok { return p.Toks[p.Pos+1] }
func (p *Parser) advance()  { p.Pos++ }

func curType(p *Parser) token.Type  { return p.cur().T }
func peekType(p *Parser) token.Type { return p.peek().T }
func advance(p *Parser)             { p.advance() }
func peekPrec(p *Parser) int {
	if v, ok := Prec[p.peek().T]; ok {
		return v
	}
	return PrecLowest
}

// Config is the toy grammar's pratt Config, built once at init.
var Config = func() *pratt.Config[Parser, string] {
	cfg := &pratt.Config[Parser, string]{
		CurType:    curType,
		PeekType:   peekType,
		Advance:    advance,
		PeekPrec:   peekPrec,
		OnNoPrefix: func(_ *Parser, _ token.Type) {},
	}

	// Prefix
	cfg.Prefix[token.INT] = func(p *Parser) string { return p.cur().Lit }
	cfg.Prefix[token.PLUS] = func(p *Parser) string {
		p.advance()
		right := pratt.Climb(p, cfg, PrecPrefix)
		return fmt.Sprintf("(+ %s)", right)
	}
	cfg.Prefix[token.MINUS] = func(p *Parser) string {
		p.advance()
		right := pratt.Climb(p, cfg, PrecPrefix)
		return fmt.Sprintf("(- %s)", right)
	}
	cfg.Prefix[token.BANG] = func(p *Parser) string {
		p.advance()
		right := pratt.Climb(p, cfg, PrecPrefix)
		return fmt.Sprintf("(! %s)", right)
	}
	cfg.Prefix[token.LPAREN] = func(p *Parser) string {
		p.advance()
		inner := pratt.Climb(p, cfg, PrecLowest)
		if p.peek().T == token.RPAREN {
			p.advance()
		}
		return inner
	}

	// Infix
	bin := func(op string) pratt.InfixFn[Parser, string] {
		return func(p *Parser, left string) string {
			prec := Prec[p.cur().T]
			p.advance()
			right := pratt.Climb(p, cfg, prec)
			return fmt.Sprintf("(%s %s %s)", op, left, right)
		}
	}
	cfg.Infix[token.PLUS] = bin("+")
	cfg.Infix[token.MINUS] = bin("-")
	cfg.Infix[token.ASTERISK] = bin("*")
	cfg.Infix[token.SLASH] = bin("/")

	return cfg
}()

// Tokenise turns a whitespace-separated source string into Toks.
func Tokenise(src string) []Tok {
	var out []Tok
	for f := range strings.FieldsSeq(src) {
		switch f {
		case "+":
			out = append(out, Tok{token.PLUS, "+"})
		case "-":
			out = append(out, Tok{token.MINUS, "-"})
		case "*":
			out = append(out, Tok{token.ASTERISK, "*"})
		case "/":
			out = append(out, Tok{token.SLASH, "/"})
		case "(":
			out = append(out, Tok{token.LPAREN, "("})
		case ")":
			out = append(out, Tok{token.RPAREN, ")"})
		case "!":
			out = append(out, Tok{token.BANG, "!"})
		default:
			out = append(out, Tok{token.INT, f})
		}
	}
	out = append(out, Tok{token.EOF, ""})
	return out
}

// Parse is the top-level convenience entry: tokenise + climb.
func Parse(src string) string {
	p := &Parser{Toks: Tokenise(src)}
	return pratt.Climb(p, Config, PrecLowest)
}
