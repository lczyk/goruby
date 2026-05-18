// Package torture provides a deliberately-pathological grammar for stress
// testing the pratt core. Many precedence levels, mixed left/right
// associativity, prefix + postfix unary, ternary, index/call postfixes, an
// always-firing Hook with real work + occasional Handled path, pointer-based
// ast nodes so allocation numbers reflect parsing cost rather than
// fmt.Sprintf bookkeeping.
//
// Precedence layout (high binds tighter):
//
//	16  postfix  . ( [   IDENT (juxtaposition)
//	15  prefix   - ! ~ +
//	14  power    ** (right-assoc)
//	13  mul/div  * / %
//	12  add/sub  + -
//	11  shift    << >>
//	10  bitwise  & | ^
//	 9  compare  < <= > >= == !=
//	 8  logical  && ||
//	 7  range    .. ...
//	 6  ternary  ? :
//	 5  assign   = (right-assoc)
//	 1  lowest
package torture

import (
	"strings"

	"github.com/lczyk/goruby/internal/pratt"
	"github.com/lczyk/goruby/token"
)

// Precedence levels.
const (
	PrecLowest  = 1
	PrecAssign  = 5
	PrecTernary = 6
	PrecRange   = 7
	PrecLogical = 8
	PrecCompare = 9
	PrecBitwise = 10
	PrecShift   = 11
	PrecAddSub  = 12
	PrecMulDiv  = 13
	PrecPower   = 14
	PrecPrefix  = 15
	PrecPostfix = 16
)

// Prec maps tokens to their precedence.
var Prec = map[token.Type]int{
	token.ASSIGN:     PrecAssign,
	token.QMARK:      PrecTernary,
	token.RANGE:      PrecRange,
	token.RANGEEX:    PrecRange,
	token.LOGICALOR:  PrecLogical,
	token.LOGICALAND: PrecLogical,
	token.LT:         PrecCompare,
	token.LTE:        PrecCompare,
	token.GT:         PrecCompare,
	token.GTE:        PrecCompare,
	token.EQ:         PrecCompare,
	token.NOTEQ:      PrecCompare,
	token.AND:        PrecBitwise,
	token.PIPE:       PrecBitwise,
	token.XOR:        PrecBitwise,
	token.LSHIFT:     PrecShift,
	token.RSHIFT:     PrecShift,
	token.PLUS:       PrecAddSub,
	token.MINUS:      PrecAddSub,
	token.ASTERISK:   PrecMulDiv,
	token.SLASH:      PrecMulDiv,
	token.MODULO:     PrecMulDiv,
	token.POWER:      PrecPower,
	token.DOT:        PrecPostfix,
	token.LPAREN:     PrecPostfix,
	token.LBRACKET:   PrecPostfix,
	token.IDENT:      PrecPostfix, // juxtaposition -- triggers Hook Handled path
}

// RightAssoc operators subtract 1 from their precedence when recursing for
// the RHS.
var RightAssoc = map[token.Type]bool{
	token.POWER:  true,
	token.ASSIGN: true,
}

// Node is one torture-grammar ast node.
type Node struct {
	Op   string
	Lit  string
	Kids []*Node
}

// Tok is one torture token.
type Tok struct {
	T   token.Type
	Lit string
}

// Parser is the torture grammar's parser state.
type Parser struct {
	Toks []Tok
	Pos  int
	Hits int // hook fires; non-zero confirms hook was invoked
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

func leaf(op, lit string, kids ...*Node) *Node {
	return &Node{Op: op, Lit: lit, Kids: kids}
}

// Config is the torture grammar's pratt Config, built once at init.
var Config = func() *pratt.Config[Parser, *Node] {
	cfg := &pratt.Config[Parser, *Node]{
		CurType:    curType,
		PeekType:   peekType,
		Advance:    advance,
		PeekPrec:   peekPrec,
		OnNoPrefix: func(_ *Parser, _ token.Type) {},
	}

	// Hook always fires + does real work: inspect peek, branch on multiple
	// conditions, occasionally handle. Mirrors what a real ruby-shaped hook
	// (suppressDoBlock + suppressHashrocket + command-call special case)
	// would cost per iteration.
	cfg.Hook = func(p *Parser, left *Node) pratt.HookResult[*Node] {
		p.Hits++
		pk := p.peek().T
		// SEMICOLON / NEWLINE -- hard barriers.
		if pk == token.SEMICOLON || pk == token.NEWLINE {
			return pratt.HookResult[*Node]{Stop: true}
		}
		// Juxtaposition: IDENT followed by IDENT -> implicit call w/ first
		// IDENT as arg. Fires only when the input deliberately triggers it.
		if pk == token.IDENT {
			arg := leaf("IDENT", p.peek().Lit)
			p.advance()
			return pratt.HookResult[*Node]{
				Replace: leaf("jcall", "", left, arg),
				Handled: true,
			}
		}
		return pratt.HookResult[*Node]{}
	}

	// Prefix table
	cfg.Prefix[token.INT] = func(p *Parser) *Node {
		return leaf("INT", p.cur().Lit)
	}
	cfg.Prefix[token.IDENT] = func(p *Parser) *Node {
		return leaf("IDENT", p.cur().Lit)
	}
	prefixUnary := func(op string) pratt.PrefixFn[Parser, *Node] {
		return func(p *Parser) *Node {
			p.advance()
			rhs := pratt.Climb(p, cfg, PrecPrefix)
			return leaf(op, "", rhs)
		}
	}
	cfg.Prefix[token.MINUS] = prefixUnary("neg")
	cfg.Prefix[token.PLUS] = prefixUnary("pos")
	cfg.Prefix[token.BANG] = prefixUnary("not")
	cfg.Prefix[token.TILDE] = prefixUnary("bnot")
	cfg.Prefix[token.LPAREN] = func(p *Parser) *Node {
		p.advance()
		inner := pratt.Climb(p, cfg, PrecLowest)
		if p.peek().T == token.RPAREN {
			p.advance()
		}
		return inner
	}

	// Infix table
	bin := func(op string) pratt.InfixFn[Parser, *Node] {
		return func(p *Parser, left *Node) *Node {
			t := p.cur().T
			prec := Prec[t]
			if RightAssoc[t] {
				prec--
			}
			p.advance()
			right := pratt.Climb(p, cfg, prec)
			return leaf(op, "", left, right)
		}
	}
	cfg.Infix[token.PLUS] = bin("+")
	cfg.Infix[token.MINUS] = bin("-")
	cfg.Infix[token.ASTERISK] = bin("*")
	cfg.Infix[token.SLASH] = bin("/")
	cfg.Infix[token.MODULO] = bin("%")
	cfg.Infix[token.POWER] = bin("**")
	cfg.Infix[token.LSHIFT] = bin("<<")
	cfg.Infix[token.RSHIFT] = bin(">>")
	cfg.Infix[token.AND] = bin("&")
	cfg.Infix[token.PIPE] = bin("|")
	cfg.Infix[token.XOR] = bin("^")
	cfg.Infix[token.LT] = bin("<")
	cfg.Infix[token.LTE] = bin("<=")
	cfg.Infix[token.GT] = bin(">")
	cfg.Infix[token.GTE] = bin(">=")
	cfg.Infix[token.EQ] = bin("==")
	cfg.Infix[token.NOTEQ] = bin("!=")
	cfg.Infix[token.LOGICALAND] = bin("&&")
	cfg.Infix[token.LOGICALOR] = bin("||")
	cfg.Infix[token.RANGE] = bin("..")
	cfg.Infix[token.RANGEEX] = bin("...")
	cfg.Infix[token.ASSIGN] = bin("=")

	// Ternary: `cond ? then : else`.
	cfg.Infix[token.QMARK] = func(p *Parser, left *Node) *Node {
		p.advance() // past ?
		thenE := pratt.Climb(p, cfg, PrecLowest)
		if p.peek().T == token.COLON {
			p.advance() // to :
			p.advance() // past :
		}
		elseE := pratt.Climb(p, cfg, PrecTernary-1)
		return leaf("?:", "", left, thenE, elseE)
	}

	// Postfix-as-infix: . member, ( call, [ index.
	cfg.Infix[token.DOT] = func(p *Parser, left *Node) *Node {
		p.advance() // past .
		name := leaf("IDENT", p.cur().Lit)
		return leaf(".", "", left, name)
	}
	cfg.Infix[token.LPAREN] = func(p *Parser, left *Node) *Node {
		p.advance() // past (
		args := []*Node{}
		if p.cur().T != token.RPAREN {
			args = append(args, pratt.Climb(p, cfg, PrecLowest))
			for p.peek().T == token.COMMA {
				p.advance() // to ,
				p.advance() // past ,
				args = append(args, pratt.Climb(p, cfg, PrecLowest))
			}
		}
		if p.peek().T == token.RPAREN {
			p.advance()
		}
		return leaf("call", "", append([]*Node{left}, args...)...)
	}
	cfg.Infix[token.LBRACKET] = func(p *Parser, left *Node) *Node {
		p.advance() // past [
		idx := pratt.Climb(p, cfg, PrecLowest)
		if p.peek().T == token.RBRACKET {
			p.advance()
		}
		return leaf("idx", "", left, idx)
	}

	return cfg
}()

// Tokenise turns a whitespace-separated source string into Toks.
func Tokenise(src string) []Tok {
	var out []Tok
	for f := range strings.FieldsSeq(src) {
		var tok Tok
		switch f {
		case "+":
			tok = Tok{token.PLUS, "+"}
		case "-":
			tok = Tok{token.MINUS, "-"}
		case "*":
			tok = Tok{token.ASTERISK, "*"}
		case "/":
			tok = Tok{token.SLASH, "/"}
		case "%":
			tok = Tok{token.MODULO, "%"}
		case "**":
			tok = Tok{token.POWER, "**"}
		case "<<":
			tok = Tok{token.LSHIFT, "<<"}
		case ">>":
			tok = Tok{token.RSHIFT, ">>"}
		case "&":
			tok = Tok{token.AND, "&"}
		case "|":
			tok = Tok{token.PIPE, "|"}
		case "^":
			tok = Tok{token.XOR, "^"}
		case "&&":
			tok = Tok{token.LOGICALAND, "&&"}
		case "||":
			tok = Tok{token.LOGICALOR, "||"}
		case "<":
			tok = Tok{token.LT, "<"}
		case "<=":
			tok = Tok{token.LTE, "<="}
		case ">":
			tok = Tok{token.GT, ">"}
		case ">=":
			tok = Tok{token.GTE, ">="}
		case "==":
			tok = Tok{token.EQ, "=="}
		case "!=":
			tok = Tok{token.NOTEQ, "!="}
		case "..":
			tok = Tok{token.RANGE, ".."}
		case "...":
			tok = Tok{token.RANGEEX, "..."}
		case "=":
			tok = Tok{token.ASSIGN, "="}
		case "?":
			tok = Tok{token.QMARK, "?"}
		case ":":
			tok = Tok{token.COLON, ":"}
		case ".":
			tok = Tok{token.DOT, "."}
		case ",":
			tok = Tok{token.COMMA, ","}
		case "(":
			tok = Tok{token.LPAREN, "("}
		case ")":
			tok = Tok{token.RPAREN, ")"}
		case "[":
			tok = Tok{token.LBRACKET, "["}
		case "]":
			tok = Tok{token.RBRACKET, "]"}
		case "!":
			tok = Tok{token.BANG, "!"}
		case "~":
			tok = Tok{token.TILDE, "~"}
		default:
			if f[0] >= '0' && f[0] <= '9' {
				tok = Tok{token.INT, f}
			} else {
				tok = Tok{token.IDENT, f}
			}
		}
		out = append(out, tok)
	}
	out = append(out, Tok{token.EOF, ""})
	return out
}

// Input builders for pathological cases.

// PrecMix returns one expression cycling through every prec level so the
// climb loop recurses/descends across the full table.
func PrecMix() string {
	return "a = b || c && d == e < f | g ^ h & i << j + k * l ** m .. n ? o : p"
}

// CallChain -- "f ( a ) ( a ) ( a ) ..." postfix call chains.
func CallChain(depth int) string {
	var b strings.Builder
	b.WriteString("f")
	for range depth {
		b.WriteString(" ( a )")
	}
	return b.String()
}

// MemberChain -- "a . b . c . d ..." left-assoc postfix chain.
func MemberChain(depth int) string {
	var b strings.Builder
	b.WriteString("a")
	for range depth {
		b.WriteString(" . b")
	}
	return b.String()
}

// PowerTower -- "2 ** 2 ** 2 ..." right-assoc; tests prec-minus-1 trick.
func PowerTower(depth int) string {
	var b strings.Builder
	b.WriteString("2")
	for range depth {
		b.WriteString(" ** 2")
	}
	return b.String()
}

// TernaryNest -- nested ternaries on the else branch.
func TernaryNest(depth int) string {
	var b strings.Builder
	for range depth {
		b.WriteString("a ? b : ")
	}
	b.WriteString("c")
	return b.String()
}

// Pathological -- alternates across every prec level, forcing the climb
// loop to descend + ascend the full precedence ladder repeatedly. Each unit
// exercises 6 prec levels in one chain.
func Pathological(reps int) string {
	unit := "a + b * c ** d .. e == f & g << h"
	parts := make([]string, reps)
	for i := range parts {
		parts[i] = unit
	}
	return strings.Join(parts, " + ")
}

// RightAssign -- right-assoc chain via `=`. Forces recursion depth equal to
// chain length.
func RightAssign(depth int) string {
	var b strings.Builder
	for range depth {
		b.WriteString("a = ")
	}
	b.WriteString("1")
	return b.String()
}

// Random -- deterministic pseudo-random expression. Cycles operators +
// literals to defeat any branch predictor + cache patterns.
func Random(n int) string {
	ops := []string{
		"+", "-", "*", "/", "%", "<<", ">>", "&", "|", "^",
		"<", ">", "==", "&&", "||", "..",
	}
	atoms := []string{"a", "b", "c", "1", "2", "3", "x", "y"}
	var b strings.Builder
	b.WriteString(atoms[0])
	for i := range n {
		b.WriteByte(' ')
		b.WriteString(ops[i%len(ops)])
		b.WriteByte(' ')
		b.WriteString(atoms[(i*7)%len(atoms)])
	}
	return b.String()
}

// Juxtapose -- IDENT IDENT IDENT ... triggers Hook Handled path every
// iteration. Worst case for hook overhead.
func Juxtapose(n int) string {
	var b strings.Builder
	b.WriteString("f")
	for range n {
		b.WriteString(" g")
	}
	return b.String()
}

// MixedDeepNest -- alternates grouping w/ mixed-prec ops. Combines recursion
// (LPAREN prefix) + climb depth (mixed prec) per level.
func MixedDeepNest(depth int) string {
	var b strings.Builder
	for range depth {
		b.WriteString("( a + b * ")
	}
	b.WriteString("c")
	for range depth {
		b.WriteString(" )")
	}
	return b.String()
}

// Parse is the top-level convenience entry: tokenise + climb.
func Parse(src string) *Node {
	p := &Parser{Toks: Tokenise(src)}
	return pratt.Climb(p, Config, PrecLowest)
}
