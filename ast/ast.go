package ast

import (
	"bytes"
	"fmt"
	"math/big"
	"sort"
	"strings"

	"github.com/lczyk/goruby/token"
)

// Node represents a node within the AST
//
// All node types implement the Node interface.
type Node interface {
	// Pos returns the position of first character belonging to the node
	Pos() int
	// End returns the position of first character immediately after the node
	End() int
	// TokenLiteral returns the literal of the node
	TokenLiteral() string
	// String returns a string representation of the node
	String() string
}

// A Statement represents a statement within the AST
//
// All statement nodes implement the Statement interface.
type Statement interface {
	Node
	statementNode()
}

// An Expression represents an expression within the AST
//
// All expression nodes implement the Expression interface.
type Expression interface {
	Node
	expressionNode()
}

// literal
type literal interface {
	Node
	literalNode()
}

// IsLiteral returns true if n is a literal node, false otherwise
func IsLiteral(n Node) bool {
	_, ok := n.(literal)
	return ok
}

// A Program node is the root node within the AST.
type Program struct {
	pos        int
	Statements []Statement
}

// Pos returns the position of first character belonging to the node
func (p *Program) Pos() int { return p.pos }

// End returns the position of first character immediately after the node
func (p *Program) End() int {
	if len(p.Statements) == 0 {
		return p.pos
	}
	return p.Statements[len(p.Statements)-1].End()
}
func (p *Program) String() string {
	var out bytes.Buffer
	first := true
	for _, s := range p.Statements {
		if s == nil {
			continue
		}
		if !first {
			out.WriteByte('\n')
		}
		out.WriteString(s.String())
		first = false
	}
	return relocateHeredocBodies(out.String())
}

// Heredoc body markers used internally by StringLiteral.String() and
// relocateHeredocBodies. \x01 opens a body, \x02 closes it. Balanced like
// brackets so nested heredocs (e.g. `<<OUTER ... #{<<INNER ... INNER} ...
// OUTER`) can be paired correctly without left-to-right ambiguity.
const (
	heredocBodyOpen  = '\x01'
	heredocBodyClose = '\x02'
)

// findHeredocBodyEnd returns the index of the matching heredocBodyClose for
// the heredocBodyOpen at openIdx, balancing nested pairs. Returns -1 if
// unbalanced.
func findHeredocBodyEnd(s string, openIdx int) int {
	depth := 1
	for j := openIdx + 1; j < len(s); j++ {
		switch s[j] {
		case heredocBodyOpen:
			depth++
		case heredocBodyClose:
			depth--
			if depth == 0 {
				return j
			}
		}
	}
	return -1
}

// relocateHeredocBodies moves heredoc bodies from their inline position
// (right after the tag, inside `\x01...\x02` markers emitted by
// StringLiteral.String()) to after the line containing the tag.
//
// At the top level, multiple tags can share a line (chained heredocs:
// `puts <<A, <<B`), so bodies are deferred and flushed in order at the next
// `\n`. Inside another heredoc's body (recursive call), a nested heredoc tag
// has no following `\n` in the linearised string -- the source `\n` lived
// inside the outer body but doesn't appear in the printed Parts -- so the
// body is inserted immediately after the tag, prefixed with `\n` to break
// the tag line.
func relocateHeredocBodies(s string) string {
	if !strings.ContainsRune(s, heredocBodyOpen) {
		return s
	}
	var result strings.Builder
	result.Grow(len(s))
	var pending []string

	i := 0
	for i < len(s) {
		if s[i] == heredocBodyOpen {
			end := findHeredocBodyEnd(s, i)
			if end >= 0 {
				pending = append(pending, relocateHeredocBodiesNested(s[i+1:end]))
				i = end + 1
				continue
			}
		}
		result.WriteByte(s[i])
		if s[i] == '\n' && len(pending) > 0 {
			for _, body := range pending {
				result.WriteString(body)
			}
			pending = pending[:0]
		}
		i++
	}
	if len(pending) > 0 {
		if result.Len() == 0 || result.String()[result.Len()-1] != '\n' {
			result.WriteByte('\n')
		}
		for _, body := range pending {
			result.WriteString(body)
		}
	}
	return result.String()
}

// relocateHeredocBodiesNested processes a heredoc body that itself may
// contain nested heredoc bodies. A nested body is inserted immediately at
// the tag position (with a `\n` to break the tag line), because the outer
// body string does not carry a `\n` between the tag and the rest of the
// outer body's content.
func relocateHeredocBodiesNested(s string) string {
	if !strings.ContainsRune(s, heredocBodyOpen) {
		return s
	}
	var result strings.Builder
	result.Grow(len(s))
	i := 0
	for i < len(s) {
		if s[i] == heredocBodyOpen {
			end := findHeredocBodyEnd(s, i)
			if end >= 0 {
				body := relocateHeredocBodiesNested(s[i+1 : end])
				if result.Len() > 0 && result.String()[result.Len()-1] != '\n' {
					result.WriteByte('\n')
				}
				result.WriteString(body)
				i = end + 1
				continue
			}
		}
		result.WriteByte(s[i])
		i++
	}
	return result.String()
}

// TokenLiteral returns the literal of the first statement and empty string if
// there is no statement.
func (p *Program) TokenLiteral() string {
	if len(p.Statements) > 0 {
		return p.Statements[0].TokenLiteral()
	}
	return ""
}

// A ReturnStatement represents a return node which yields another Expression.
type ReturnStatement struct {
	Token       token.Token // the 'return' token
	ReturnValue Expression
}

func (rs *ReturnStatement) String() string {
	var out bytes.Buffer
	out.WriteString(rs.TokenLiteral() + " ")
	if rs.ReturnValue != nil {
		if al, ok := rs.ReturnValue.(*ArrayLiteral); ok && al.Token.Type != token.LBRACKET {
			elems := make([]string, len(al.Elements))
			for i, e := range al.Elements {
				elems[i] = e.String()
			}
			out.WriteString(strings.Join(elems, ", "))
		} else {
			out.WriteString(rs.ReturnValue.String())
		}
	}
	return out.String()
}
func (rs *ReturnStatement) statementNode() {}

// TokenLiteral returns the 'return' token literal
func (rs *ReturnStatement) TokenLiteral() string { return rs.Token.Literal }

// Pos returns the position of first character belonging to the node
func (rs *ReturnStatement) Pos() int { return rs.Token.Pos }

// End returns the position of first character immediately after the node
func (rs *ReturnStatement) End() int { return rs.ReturnValue.End() }

// An ExpressionStatement is a Statement wrapping an Expression
type ExpressionStatement struct {
	Token      token.Token // the first token of the expression
	Expression Expression
}

func (es *ExpressionStatement) String() string {
	if es.Expression != nil {
		return es.Expression.String()
	}
	return ""
}
func (es *ExpressionStatement) statementNode() {}

// Pos returns the position of first character belonging to the node
func (es *ExpressionStatement) Pos() int { return es.Expression.Pos() }

// End returns the position of first character immediately after the node
func (es *ExpressionStatement) End() int { return es.Expression.End() }

// TokenLiteral returns the first token of the Expression
func (es *ExpressionStatement) TokenLiteral() string { return es.Token.Literal }

// BlockStatement represents a list of statements
type BlockStatement struct {
	// the { token or the first token from the first statement
	Token      token.Token
	EndToken   token.Token // the } token
	Statements []Statement
}

func (bs *BlockStatement) statementNode() {}

// Pos returns the position of first character belonging to the node
func (bs *BlockStatement) Pos() int { return bs.Token.Pos }

// End returns the position of first character immediately after the node
func (bs *BlockStatement) End() int { return bs.EndToken.Pos }

// TokenLiteral returns '{' or the first token from the first statement
func (bs *BlockStatement) TokenLiteral() string { return bs.Token.Literal }
func (bs *BlockStatement) String() string {
	stmts := make([]string, 0, len(bs.Statements))
	for _, s := range bs.Statements {
		if s != nil {
			stmts = append(stmts, s.String())
		}
	}
	return strings.Join(stmts, "\n")
}

// ExceptionHandlingBlock represents a begin/end block where exceptions are rescued
type ExceptionHandlingBlock struct {
	BeginToken token.Token
	EndToken   token.Token
	TryBody    *BlockStatement
	Rescues    []*RescueBlock
	ElseBody   *BlockStatement
	EnsureBody *BlockStatement
}

func (eh *ExceptionHandlingBlock) expressionNode() {}

// Pos returns the position of first character belonging to the node
func (eh *ExceptionHandlingBlock) Pos() int { return eh.BeginToken.Pos }

// End returns the position of first character immediately after the node
func (eh *ExceptionHandlingBlock) End() int { return eh.EndToken.Pos }

// TokenLiteral returns the token literal from 'begin'
func (eh *ExceptionHandlingBlock) TokenLiteral() string { return eh.BeginToken.Literal }
func (eh *ExceptionHandlingBlock) String() string {
	var out bytes.Buffer
	out.WriteString(eh.BeginToken.Literal)
	out.WriteString("\n")
	out.WriteString(eh.TryBody.String())
	out.WriteString("\n")
	for _, r := range eh.Rescues {
		out.WriteString(r.String())
	}
	if eh.ElseBody != nil {
		out.WriteString("else\n")
		out.WriteString(eh.ElseBody.String())
		out.WriteString("\n")
	}
	if eh.EnsureBody != nil {
		out.WriteString("ensure\n")
		out.WriteString(eh.EnsureBody.String())
		out.WriteString("\n")
	}
	out.WriteString("end")
	return out.String()
}

// A RescueBlock represents a rescue block
type RescueBlock struct {
	Token            token.Token
	ExceptionClasses []*Identifier
	Exception        *Identifier
	Body             *BlockStatement
}

func (rb *RescueBlock) expressionNode() {}

// Pos returns the position of first character belonging to the node
func (rb *RescueBlock) Pos() int { return rb.Token.Pos }

// End returns the position of first character immediately after the node
func (rb *RescueBlock) End() int { return rb.Body.End() }

// TokenLiteral returns the token literal from 'rescue'
func (rb *RescueBlock) TokenLiteral() string { return rb.Token.Literal }
func (rb *RescueBlock) String() string {
	var out bytes.Buffer
	out.WriteString("rescue")
	if len(rb.ExceptionClasses) != 0 {
		out.WriteString(" ")
		classes := make([]string, len(rb.ExceptionClasses))
		for i, c := range rb.ExceptionClasses {
			classes[i] = c.String()
		}
		out.WriteString(strings.Join(classes, ", "))
	}
	if rb.Exception != nil {
		out.WriteString(" => ")
		out.WriteString(rb.Exception.String())
	}
	out.WriteString("\n")
	out.WriteString(rb.Body.String())
	out.WriteString("\n")
	return out.String()
}

// Assignment represents a generic assignment
type Assignment struct {
	Token token.Token
	Left  Expression
	Right Expression
}

func (a *Assignment) String() string {
	var out bytes.Buffer
	out.WriteString(a.Left.String())
	op := a.Token.Literal
	if op == "" {
		op = "="
	}
	out.WriteString(" ")
	out.WriteString(op)
	out.WriteString(" ")
	rhs := a.Right
	if op != "=" {
		if inf, ok := rhs.(*InfixExpression); ok && inf.Left != nil && inf.Left.String() == a.Left.String() {
			rhs = inf.Right
		}
	}
	out.WriteString(rhs.String())
	return out.String()
}
func (a *Assignment) expressionNode() {}

// Pos returns the position of first character belonging to the node
func (a *Assignment) Pos() int { return a.Left.Pos() }

// End returns the position of first character immediately after the node
func (a *Assignment) End() int { return a.Right.End() }

// TokenLiteral returns the literal of the ASSIGN token
func (a *Assignment) TokenLiteral() string { return a.Token.Literal }

// An InstanceVariable represents an instance variable in the AST
type InstanceVariable struct {
	Token token.Token
	Name  *Identifier
}

func (i *InstanceVariable) String() string {
	var out bytes.Buffer
	out.WriteString(i.Token.Literal)
	out.WriteString(i.Name.String())
	return out.String()
}
func (i *InstanceVariable) literalNode()    {}
func (i *InstanceVariable) expressionNode() {}

// Pos returns the position of first character belonging to the node
func (i *InstanceVariable) Pos() int { return i.Token.Pos }

// End returns the position of first character immediately after the node
func (i *InstanceVariable) End() int { return i.Name.End() }

// TokenLiteral returns the literal of the AT token
func (i *InstanceVariable) TokenLiteral() string { return i.Token.Literal }

// A ClassVariable represents a class variable in the AST
type ClassVariable struct {
	Token token.Token
	Name  *Identifier
}

func (c *ClassVariable) String() string {
	var out bytes.Buffer
	out.WriteString(c.Token.Literal)
	out.WriteString(c.Name.String())
	return out.String()
}
func (c *ClassVariable) literalNode()    {}
func (c *ClassVariable) expressionNode() {}

// Pos returns the position of first character belonging to the node
func (c *ClassVariable) Pos() int { return c.Token.Pos }

// End returns the position of first character immediately after the node
func (c *ClassVariable) End() int { return c.Name.End() }

// TokenLiteral returns the literal of the CLASS_VAR token
func (c *ClassVariable) TokenLiteral() string { return c.Token.Literal }

// MultiAssignment represents multiple variables on the lefthand side
type MultiAssignment struct {
	Variables []*Identifier
	Values    []Expression
}

func (m *MultiAssignment) String() string {
	var out bytes.Buffer
	vars := make([]string, len(m.Variables))
	for i, v := range m.Variables {
		vars[i] = v.Value
	}
	out.WriteString(strings.Join(vars, ", "))
	out.WriteString(" = ")
	values := make([]string, len(m.Values))
	for i, v := range m.Values {
		values[i] = v.String()
	}
	out.WriteString(strings.Join(values, ", "))
	return out.String()
}
func (m *MultiAssignment) literalNode() {}

// Pos returns the position of first character belonging to the node
func (m *MultiAssignment) Pos() int { return m.Variables[0].Pos() }

// End returns the position of first character immediately after the node
func (m *MultiAssignment) End() int        { return m.Values[len(m.Values)-1].End() }
func (m *MultiAssignment) expressionNode() {}

// TokenLiteral returns the literal of the first variable token
func (m *MultiAssignment) TokenLiteral() string { return m.Variables[0].Token.Literal }

// Self represents self in the current context in the program
type Self struct {
	Token token.Token // the token.SELF token
}

func (s *Self) String() string  { return s.Token.Literal }
func (s *Self) expressionNode() {}
func (s *Self) literalNode()    {}

// Pos returns the position of first character belonging to the node
func (s *Self) Pos() int { return s.Token.Pos }

// End returns the position of first character immediately after the node
func (s *Self) End() int { return s.Token.Pos + 4 }

// TokenLiteral returns the literal of the token.SELF token
func (s *Self) TokenLiteral() string { return s.Token.Literal }

// YieldExpression represents self in the current context in the program
type YieldExpression struct {
	Token     token.Token      // the token.YIELD token
	Arguments []Expression     // The arguments to yield
	Block     *BlockExpression // optional block passed to yield
}

func (y *YieldExpression) String() string {
	var out bytes.Buffer
	out.WriteString(y.Token.Literal)
	if len(y.Arguments) != 0 {
		args := []string{}
		for _, a := range y.Arguments {
			args = append(args, a.String())
		}
		out.WriteString("(")
		out.WriteString(strings.Join(args, ", "))
		out.WriteString(")")
	}
	return out.String()
}
func (y *YieldExpression) expressionNode() {}

// Pos returns the position of first character belonging to the node
func (y *YieldExpression) Pos() int { return y.Token.Pos }

// End returns the position of first character immediately after the node
func (y *YieldExpression) End() int {
	if len(y.Arguments) == 0 {
		return y.Pos() + 5
	}
	return y.Arguments[len(y.Arguments)-1].End()
}

// TokenLiteral returns the literal of the token.YIELD token
func (y *YieldExpression) TokenLiteral() string { return y.Token.Literal }

// SuperExpression represents a `super` call with optional arguments
type SuperExpression struct {
	Token     token.Token      // the token.KW_SUPER token
	Arguments []Expression     // optional explicit arguments; nil means implicit forwarding
	Block     *BlockExpression // optional block passed to super
}

func (s *SuperExpression) String() string {
	var out bytes.Buffer
	out.WriteString(s.Token.Literal)
	if s.Arguments != nil {
		args := []string{}
		for _, a := range s.Arguments {
			args = append(args, a.String())
		}
		out.WriteString("(")
		out.WriteString(strings.Join(args, ", "))
		out.WriteString(")")
	}
	if s.Block != nil {
		if s.Block.Token.Type == token.LBRACE {
			out.WriteString(" ")
		}
		out.WriteString(s.Block.String())
	}
	return out.String()
}
func (s *SuperExpression) expressionNode() {}

func (s *SuperExpression) Pos() int { return s.Token.Pos }
func (s *SuperExpression) End() int {
	if len(s.Arguments) == 0 {
		return s.Pos() + len(s.Token.Literal)
	}
	return s.Arguments[len(s.Arguments)-1].End()
}
func (s *SuperExpression) TokenLiteral() string { return s.Token.Literal }

// BeginBlock represents a top-level BEGIN { ... } block
type BeginBlock struct {
	Token token.Token // the BEGIN token
	Body  *BlockStatement
}

func (b *BeginBlock) expressionNode()      {}
func (b *BeginBlock) Pos() int             { return b.Token.Pos }
func (b *BeginBlock) End() int             { return b.Body.End() }
func (b *BeginBlock) TokenLiteral() string { return b.Token.Literal }
func (b *BeginBlock) String() string {
	return "BEGIN {\n" + b.Body.String() + "\n}"
}

// EndBlock represents a top-level END { ... } block
type EndBlock struct {
	Token token.Token // the END token
	Body  *BlockStatement
}

func (e *EndBlock) expressionNode()      {}
func (e *EndBlock) Pos() int             { return e.Token.Pos }
func (e *EndBlock) End() int             { return e.Body.End() }
func (e *EndBlock) TokenLiteral() string { return e.Token.Literal }
func (e *EndBlock) String() string {
	return "END {\n" + e.Body.String() + "\n}"
}

// Keyword__FILE__ represents __FILE__ in the AST
type Keyword__FILE__ struct {
	Token    token.Token // the token.FILE__ token
	Filename string
}

func (f *Keyword__FILE__) String() string  { return f.Token.Literal }
func (f *Keyword__FILE__) expressionNode() {}
func (f *Keyword__FILE__) literalNode()    {}

// Pos returns the position of first character belonging to the node
func (f *Keyword__FILE__) Pos() int { return f.Token.Pos }

// End returns the position of first character immediately after the node
func (f *Keyword__FILE__) End() int { return f.Token.Pos + 8 }

// TokenLiteral returns the literal of the token.FILE__ token
func (f *Keyword__FILE__) TokenLiteral() string { return f.Token.Literal }

// Keyword__DIR__ represents __dir__ in the AST
type Keyword__DIR__ struct {
	Token token.Token // the KEYWORD__DIR__ token
}

func (d *Keyword__DIR__) String() string       { return d.Token.Literal }
func (d *Keyword__DIR__) expressionNode()      {}
func (d *Keyword__DIR__) literalNode()         {}
func (d *Keyword__DIR__) Pos() int             { return d.Token.Pos }
func (d *Keyword__DIR__) End() int             { return d.Token.Pos + 6 }
func (d *Keyword__DIR__) TokenLiteral() string { return d.Token.Literal }

// Keyword__CALLEE__ represents __callee__ in the AST
type Keyword__CALLEE__ struct {
	Token token.Token
}

func (c *Keyword__CALLEE__) String() string       { return c.Token.Literal }
func (c *Keyword__CALLEE__) expressionNode()      {}
func (c *Keyword__CALLEE__) literalNode()         {}
func (c *Keyword__CALLEE__) Pos() int             { return c.Token.Pos }
func (c *Keyword__CALLEE__) End() int             { return c.Token.Pos + 10 }
func (c *Keyword__CALLEE__) TokenLiteral() string { return c.Token.Literal }

// Keyword__METHOD__ represents __method__ in the AST
type Keyword__METHOD__ struct {
	Token token.Token
}

func (m *Keyword__METHOD__) String() string       { return m.Token.Literal }
func (m *Keyword__METHOD__) expressionNode()      {}
func (m *Keyword__METHOD__) literalNode()         {}
func (m *Keyword__METHOD__) Pos() int             { return m.Token.Pos }
func (m *Keyword__METHOD__) End() int             { return m.Token.Pos + 10 }
func (m *Keyword__METHOD__) TokenLiteral() string { return m.Token.Literal }

// Keyword__ENCODING__ represents __ENCODING__ in the AST
type Keyword__ENCODING__ struct {
	Token token.Token
}

func (e *Keyword__ENCODING__) String() string       { return e.Token.Literal }
func (e *Keyword__ENCODING__) expressionNode()      {}
func (e *Keyword__ENCODING__) literalNode()         {}
func (e *Keyword__ENCODING__) Pos() int             { return e.Token.Pos }
func (e *Keyword__ENCODING__) End() int             { return e.Token.Pos + 12 }
func (e *Keyword__ENCODING__) TokenLiteral() string { return e.Token.Literal }

// UsingExpression represents a `using Module` statement
type UsingExpression struct {
	Token token.Token // the using keyword
	Expr  Expression  // the module/refinement
}

func (u *UsingExpression) expressionNode()      {}
func (u *UsingExpression) Pos() int             { return u.Token.Pos }
func (u *UsingExpression) End() int             { return u.Expr.End() }
func (u *UsingExpression) TokenLiteral() string { return u.Token.Literal }
func (u *UsingExpression) String() string {
	return "using " + u.Expr.String()
}

// RefineExpression represents a `refine Class do ... end` block
type RefineExpression struct {
	Token    token.Token // the refine keyword
	EndToken token.Token // the end token
	Expr     Expression  // the target class
	Body     *BlockStatement
}

func (r *RefineExpression) expressionNode()      {}
func (r *RefineExpression) Pos() int             { return r.Token.Pos }
func (r *RefineExpression) End() int             { return r.EndToken.Pos }
func (r *RefineExpression) TokenLiteral() string { return r.Token.Literal }
func (r *RefineExpression) String() string {
	if r.Body == nil {
		return "refine " + r.Expr.String()
	}
	return "refine " + r.Expr.String() + " do\n" + r.Body.String() + "\nend"
}

// An Identifier represents an identifier in the program
type Identifier struct {
	Token token.Token // the token.IDENT token
	Value string
}

func (i *Identifier) String() string {
	if i == nil {
		return ""
	}
	return i.Value
}
func (i *Identifier) expressionNode() {}
func (i *Identifier) literalNode()    {}

// Pos returns the position of first character belonging to the node
func (i *Identifier) Pos() int { return i.Token.Pos }

// End returns the position of first character immediately after the node
func (i *Identifier) End() int { return i.Token.Pos + len(i.Value) }

// IsConstant returns true if the Identifier represents a Constant, false otherwise
func (i *Identifier) IsConstant() bool { return i.Token.Type == token.CONST }

// TokenLiteral returns the literal of the token.IDENT token
func (i *Identifier) TokenLiteral() string { return i.Token.Literal }

// Global represents a global in the AST
type Global struct {
	Token token.Token // the token.GLOBAL token
	Value string
}

func (g *Global) String() string  { return g.Value }
func (g *Global) expressionNode() {}

// Pos returns the position of first character belonging to the node
func (g *Global) Pos() int { return g.Token.Pos }

// End returns the position of first character immediately after the node
func (g *Global) End() int     { return g.Token.Pos + len(g.Value) }
func (g *Global) literalNode() {}

// TokenLiteral returns the literal of the token.GLOBAL token
func (g *Global) TokenLiteral() string { return g.Token.Literal }

// ScopedIdentifier represents a scoped Constant declaration
type ScopedIdentifier struct {
	Token token.Token // the token.SCOPE
	Outer *Identifier
	Inner Expression
}

func (i *ScopedIdentifier) String() string {
	var out bytes.Buffer
	if i.Outer != nil {
		out.WriteString(i.Outer.String())
	}
	out.WriteString(i.Token.Literal)
	if i.Inner != nil {
		out.WriteString(i.Inner.String())
	}
	return out.String()
}
func (i *ScopedIdentifier) expressionNode() {}
func (i *ScopedIdentifier) literalNode()    {}

// Pos returns the position of first character belonging to the node
func (i *ScopedIdentifier) Pos() int { return i.Outer.Pos() }

// End returns the position of first character immediately after the node
func (i *ScopedIdentifier) End() int { return i.Inner.End() }

// TokenLiteral returns the literal of the token.SCOPE token
func (i *ScopedIdentifier) TokenLiteral() string { return i.Token.Literal }

// IntegerLiteral represents an integer in the AST
type IntegerLiteral struct {
	Token  token.Token
	Value  int64
	BigInt *big.Int // set for values that overflow int64
}

func (il *IntegerLiteral) expressionNode() {}
func (il *IntegerLiteral) literalNode()    {}

// Pos returns the position of first character belonging to the node
func (il *IntegerLiteral) Pos() int { return il.Token.Pos }

// End returns the position of first character immediately after the node
func (il *IntegerLiteral) End() int {
	if il.BigInt != nil {
		return il.Token.Pos + len(il.BigInt.String())
	}
	return il.Token.Pos + len(fmt.Sprintf("%d", il.Value))
}

// TokenLiteral returns the literal from the token.INT token
func (il *IntegerLiteral) TokenLiteral() string { return il.Token.Literal }
func (il *IntegerLiteral) String() string {
	// Preserve the original literal form (0xFF, 0b101, 0o7, 0d10, with
	// underscores, etc) so MRI re-reads the same IntegerBaseFlags.
	if il.Token.Literal != "" {
		return il.Token.Literal
	}
	if il.BigInt != nil {
		return il.BigInt.String()
	}
	return fmt.Sprintf("%d", il.Value)
}

// FloatLiteral represents a floating-point number in the AST
type FloatLiteral struct {
	Token token.Token
	Value float64
}

func (fl *FloatLiteral) expressionNode() {}
func (fl *FloatLiteral) literalNode()    {}

// Pos returns the position of first character belonging to the node
func (fl *FloatLiteral) Pos() int { return fl.Token.Pos }

// End returns the position of first character immediately after the node
func (fl *FloatLiteral) End() int { return fl.Token.Pos + len(fl.Token.Literal) }

// TokenLiteral returns the literal from the token.FLOAT token
func (fl *FloatLiteral) TokenLiteral() string { return fl.Token.Literal }
func (fl *FloatLiteral) String() string       { return fl.Token.Literal }

// Nil represents the 'nil' keyword
type Nil struct {
	Token token.Token
}

func (n *Nil) expressionNode() {}
func (n *Nil) literalNode()    {}

// Pos returns the position of first character belonging to the node
func (n *Nil) Pos() int { return n.Token.Pos }

// End returns the position of first character immediately after the node
func (n *Nil) End() int { return n.Token.Pos + 3 }

// TokenLiteral returns the literal from the token token.NIL
func (n *Nil) TokenLiteral() string { return n.Token.Literal }
func (n *Nil) String() string       { return "nil" }

// Boolean represents a boolean in the AST
type Boolean struct {
	Token token.Token
	Value bool
}

func (b *Boolean) expressionNode() {}
func (b *Boolean) literalNode()    {}

// Pos returns the position of first character belonging to the node
func (b *Boolean) Pos() int { return b.Token.Pos }

// End returns the position of first character immediately after the node
func (b *Boolean) End() int { return b.Token.Pos + len(fmt.Sprintf("%t", b.Value)) }

// TokenLiteral returns the literal from the token token.BOOLEAN
func (b *Boolean) TokenLiteral() string { return b.Token.Literal }
func (b *Boolean) String() string       { return fmt.Sprintf("%t", b.Value) }

// StringLiteral represents a string in the AST. For non-interpolated strings,
// Value holds the content and Parts is nil. For interpolated strings, Parts
// holds StringContent and expression nodes.
type StringLiteral struct {
	Token           token.Token      // STRING_BEG or STRING
	Value           string           // for non-interpolated strings
	Parts           []Expression     // for interpolated strings
	HeredocTag      string           // e.g. "<<~EOS", "<<-'DOC'" -- empty for non-heredocs
	HeredocStripped bool             // true on `<<~` heredocs whose source had a positive common indent that was stripped -- preserves MRI's NODE_DSTR (vs NODE_STR) classification on roundtrip
	Adjacent        []*StringLiteral // adjacent string literals: `"a" "b"` -- MRI parses each separately and wraps in an outer InterpolatedString
}

func (sl *StringLiteral) expressionNode() {}
func (sl *StringLiteral) literalNode()    {}

// Pos returns the position of first character belonging to the node
func (sl *StringLiteral) Pos() int { return sl.Token.Pos }

// End returns the position of first character immediately after the node
func (sl *StringLiteral) End() int {
	if sl.Parts != nil {
		if len(sl.Parts) == 0 {
			return sl.Token.Pos + 2 // empty string: "" = 2 chars
		}
		return sl.Parts[len(sl.Parts)-1].End()
	}
	return sl.Token.Pos + len(sl.Value)
}

// TokenLiteral returns the literal from the string token
func (sl *StringLiteral) TokenLiteral() string { return sl.Token.Literal }
// heredocStyle classifies a heredoc tag by its prefix:
// "<<~" -> "squiggly", "<<-" -> "dash", "<<" -> "plain".
func heredocStyle(tag string) string {
	if len(tag) < 3 {
		return "plain"
	}
	switch tag[2] {
	case '~':
		return "squiggly"
	case '-':
		return "dash"
	}
	return "plain"
}

// heredocHasContentLine reports whether s contains any non-whitespace
// line. Used to decide whether the squiggly-heredoc indent workaround
// applies -- a purely whitespace body would parse to a different
// unescaped content after MRI's strip pass.
func heredocHasContentLine(s string) bool {
	i := 0
	for i < len(s) {
		j := i
		for j < len(s) && s[j] != '\n' {
			j++
		}
		// Line is s[i:j]. Non-whitespace if any non-space char.
		for k := i; k < j; k++ {
			if s[k] != ' ' && s[k] != '\t' {
				return true
			}
		}
		i = j + 1
	}
	return false
}

// minLeadingWS returns the smallest count of leading space chars across
// all non-empty lines of s. Returns 0 if s is empty or any non-empty line
// has no leading space.
func minLeadingWS(s string) int {
	if s == "" {
		return 0
	}
	min := -1
	i := 0
	for i < len(s) {
		// find next non-newline run -- skip blank lines (only `\n`).
		j := i
		for j < len(s) && s[j] != '\n' {
			j++
		}
		if j > i { // non-empty line
			lead := 0
			for lead < j-i && s[i+lead] == ' ' {
				lead++
			}
			if min < 0 || lead < min {
				min = lead
			}
		}
		i = j + 1
	}
	if min < 0 {
		return 0
	}
	return min
}

// indentLines prepends pad to every line in s except an empty trailing
// line (so a trailing `\n` doesn't produce a phantom indented empty line).
func indentLines(s, pad string) string {
	if s == "" {
		return s
	}
	var b bytes.Buffer
	b.Grow(len(s) + strings.Count(s, "\n")*len(pad) + len(pad))
	b.WriteString(pad)
	for i := 0; i < len(s); i++ {
		c := s[i]
		b.WriteByte(c)
		if c == '\n' && i+1 < len(s) {
			b.WriteString(pad)
		}
	}
	return b.String()
}

func heredocDelimFromTag(tag string) string {
	s := tag[2:]
	if len(s) > 0 && (s[0] == '~' || s[0] == '-') {
		s = s[1:]
	}
	if len(s) >= 2 {
		q := s[0]
		if q == '\'' || q == '"' || q == '`' {
			return s[1 : len(s)-1]
		}
	}
	return s
}

func (sl *StringLiteral) String() string {
	s := sl.stringOnce()
	for _, a := range sl.Adjacent {
		s += " " + a.String()
	}
	return s
}

func (sl *StringLiteral) stringOnce() string {
	if sl.HeredocTag != "" {
		delim := heredocDelimFromTag(sl.HeredocTag)
		// Heredoc style affects col-0 vs indented body handling. MRI's
		// parser fails to recognise the closing delimiter for *chained*
		// heredocs in interp (e.g. `"#{<<~A}#{<<~B}"`) when both body
		// lines and delim sit at column 0 -- the delim line is misread
		// as part of the previous body. Indenting body+delim by one
		// space sidesteps the quirk:
		//   - `<<~` (squiggly) strips common leading WS, so adding one
		//     space to every body line + delim is a content no-op.
		//   - `<<-` (dash) allows indented delim but preserves body
		//     verbatim; indenting only the delim is safe.
		//   - `<<` (plain) requires delim at column 0 and preserves body
		//     verbatim; no safe indent possible. Untouched.
		style := heredocStyle(sl.HeredocTag)
		var body bytes.Buffer
		if sl.Parts != nil {
			for _, p := range sl.Parts {
				switch x := p.(type) {
				case *StringContent:
					body.WriteString(x.Value)
				case *EmbeddedVariable:
					body.WriteString(x.String())
				default:
					body.WriteString("#{")
					body.WriteString(p.String())
					body.WriteString("}")
				}
			}
		} else {
			body.WriteString(sl.Value)
		}
		var out bytes.Buffer
		out.WriteString(sl.HeredocTag)
		out.WriteByte(heredocBodyOpen)
		// Compute delim indentation so that MRI strips exactly the body's
		// current leading WS (preserves squiggly content) but is also at
		// >= col 1 (works around the MRI chained-heredoc-in-interp quirk
		// where col-0 delim isn't recognised). For non-squiggly, keep
		// delim at col 0 unless we'd hit the quirk -- but plain `<<` only
		// accepts col-0 delim, so leave alone.
		bodyStr := body.String()
		switch style {
		case "squiggly":
			bodyMin := minLeadingWS(bodyStr)
			target := bodyMin
			// Re-indent the body (and delim) by 1 space only when the source
			// had a positive common indent that was stripped (HeredocStripped)
			// OR this heredoc was constructed inside `#{...}` (chained-in-interp
			// quirk). Otherwise leave at col 0 so MRI emits NODE_STR rather than
			// NODE_DSTR.
			if target < 1 && sl.HeredocStripped {
				target = 1
			}
			// Skip the indent workaround when the body has no non-whitespace
			// content lines. minLeadingWS returns 0 in that case (no real
			// lines), but adding " \n" would make MRI's squiggly strip
			// nothing (all-whitespace lines are skipped) -- the body content
			// would re-parse as " \n" rather than "\n", diverging from the
			// source's parsetree.
			if !heredocHasContentLine(bodyStr) {
				out.WriteString(bodyStr)
				out.WriteString(delim)
				out.WriteByte('\n')
				out.WriteByte(heredocBodyClose)
				return out.String()
			}
			if target > bodyMin {
				bodyStr = indentLines(bodyStr, strings.Repeat(" ", target-bodyMin))
			}
			out.WriteString(bodyStr)
			out.WriteString(strings.Repeat(" ", target))
		case "dash":
			// `<<-` allows indented delim; body preserved verbatim. Indent
			// delim by 1 if body min is 0, sidestepping the col-0 quirk
			// for chained heredocs.
			out.WriteString(bodyStr)
			if minLeadingWS(bodyStr) == 0 {
				out.WriteByte(' ')
			}
		default:
			// plain `<<` requires delim at col 0; body verbatim.
			out.WriteString(bodyStr)
		}
		out.WriteString(delim)
		out.WriteByte('\n')
		out.WriteByte(heredocBodyClose)
		return out.String()
	}
	var open, close string
	switch sl.Token.Type {
	case token.XSTR, token.XSTR_BEG:
		open, close = "`", "`"
	case token.STRING, token.STRING_BEG:
		// Respect source quote style. Lexer stores Value as raw source
		// bytes; round-tripping through the other quote form would
		// reinterpret \n etc. and change runtime semantics.
		if sl.Token.IsCharLit {
			// `?X` character literal -- re-emit as such so MRI's
			// StringFlags (forced_<source>_encoding) match on re-parse.
			return "?" + sl.Value
		}
		if sl.Token.SingleQuoted {
			open, close = "'", "'"
		} else {
			open, close = "\"", "\""
		}
	default:
		if sl.Parts == nil && (strings.Contains(sl.Value, "#{") || !strings.Contains(sl.Value, "'")) {
			open, close = "'", "'"
		} else {
			open, close = "\"", "\""
		}
	}
	if sl.Parts != nil {
		if open == "'" {
			for _, p := range sl.Parts {
				if _, ok := p.(*StringContent); !ok {
					open, close = "\"", "\""
					break
				}
			}
		}
		var out bytes.Buffer
		out.WriteString(open)
		for _, p := range sl.Parts {
			switch x := p.(type) {
			case *StringContent:
				v := x.Value
				if open == "\"" && strings.Contains(v, "\"") && !strings.Contains(v, "\\\"") {
					v = strings.ReplaceAll(v, "\"", "\\\"")
				}
				out.WriteString(v)
			case *EmbeddedVariable:
				out.WriteString(x.String())
			default:
				if pe, ok := p.(*ParenExpression); ok && pe.Expr == nil && len(pe.Stmts) == 0 {
					out.WriteString("#{}")
				} else {
					out.WriteString("#{")
					out.WriteString(p.String())
					out.WriteString("}")
				}
			}
		}
		out.WriteString(close)
		return out.String()
	}
	val := sl.Value
	if open == "\"" && strings.Contains(val, "\"") && !strings.Contains(val, "\\\"") {
		val = strings.ReplaceAll(val, "\"", "\\\"")
	}
	if open == "'" && strings.Contains(val, "'") && !strings.Contains(val, "\\'") {
		val = strings.ReplaceAll(val, "'", "\\'")
	}
	return open + val + close
}

// StringContent represents a literal text segment within an interpolated string.
type StringContent struct {
	Token token.Token // STRING_CONTENT
	Value string
}

func (sc *StringContent) expressionNode() {}
func (sc *StringContent) literalNode()    {}

// Pos returns the position of first character belonging to the node
func (sc *StringContent) Pos() int { return sc.Token.Pos }

// End returns the position of first character immediately after the node
func (sc *StringContent) End() int { return sc.Token.Pos + len(sc.Value) }

// TokenLiteral returns the literal of the STRING_CONTENT token
func (sc *StringContent) TokenLiteral() string { return sc.Token.Literal }
func (sc *StringContent) String() string       { return sc.Value }

// EmbeddedVariable represents a `#@ivar`, `#@@cvar`, or `#$gvar` shorthand
// interpolation inside a string -- distinguished from `#{@ivar}` etc., which
// MRI parses as EmbeddedStatementsNode.
type EmbeddedVariable struct {
	Variable Expression // *InstanceVariable, *ClassVariable, or *Global
}

func (ev *EmbeddedVariable) expressionNode()      {}
func (ev *EmbeddedVariable) Pos() int             { return ev.Variable.Pos() }
func (ev *EmbeddedVariable) End() int             { return ev.Variable.End() }
func (ev *EmbeddedVariable) TokenLiteral() string { return ev.Variable.TokenLiteral() }
func (ev *EmbeddedVariable) String() string       { return "#" + ev.Variable.String() }

// RegexLiteral represents a regex literal in the AST.
type RegexLiteral struct {
	Token   token.Token  // REGEX_BEG
	Value   string       // for non-interpolated regexes
	Parts   []Expression // for interpolated regexes
	Options string       // flags like "imx"
}

func (rl *RegexLiteral) expressionNode() {}
func (rl *RegexLiteral) literalNode()    {}
func (rl *RegexLiteral) Pos() int        { return rl.Token.Pos }
func (rl *RegexLiteral) End() int {
	if rl.Parts != nil {
		if len(rl.Parts) == 0 {
			return rl.Token.Pos + 2
		}
		return rl.Parts[len(rl.Parts)-1].End()
	}
	return rl.Token.Pos + len(rl.Value)
}
func (rl *RegexLiteral) TokenLiteral() string { return rl.Token.Literal }
func (rl *RegexLiteral) String() string {
	// If the regex content includes `/`, switch to %r-style delimiters so we
	// don't have to insert `\/` escapes (which MRI's parsetree records as
	// part of the literal content -- diverging from the original).
	openDelim, closeDelim := "/", "/"
	if regexContentHasSlash(rl) {
		openDelim, closeDelim = pickRegexDelim(rl)
	}
	var out bytes.Buffer
	if openDelim != "/" {
		out.WriteString("%r")
	}
	out.WriteString(openDelim)
	if rl.Parts != nil {
		for _, p := range rl.Parts {
			switch x := p.(type) {
			case *StringContent:
				if openDelim == "/" {
					out.WriteString(escapeRegexSlash(x.Value))
				} else {
					out.WriteString(x.Value)
				}
			case *EmbeddedVariable:
				out.WriteString(x.String())
			default:
				if pe, ok := p.(*ParenExpression); ok && pe.Expr == nil && len(pe.Stmts) == 0 {
					// Empty `#{}` placeholder -- emit bare to match MRI
					// EmbeddedStatementsNode(statements=nil) shape.
					out.WriteString("#{}")
				} else {
					out.WriteString("#{")
					out.WriteString(p.String())
					out.WriteString("}")
				}
			}
		}
	} else {
		if openDelim == "/" {
			out.WriteString(escapeRegexSlash(rl.Value))
		} else {
			out.WriteString(rl.Value)
		}
	}
	out.WriteString(closeDelim)
	out.WriteString(rl.Options)
	return out.String()
}

func regexContentHasSlash(rl *RegexLiteral) bool {
	if rl.Parts == nil {
		return strings.Contains(rl.Value, "/") && !strings.Contains(rl.Value, "\\/")
	}
	for _, p := range rl.Parts {
		if sc, ok := p.(*StringContent); ok {
			if strings.Contains(sc.Value, "/") && !strings.Contains(sc.Value, "\\/") {
				return true
			}
		}
	}
	return false
}

func pickRegexDelim(rl *RegexLiteral) (string, string) {
	pairs := [][2]string{
		{"{", "}"},
		{"!", "!"},
		{"|", "|"},
		{"#", "#"},
	}
	contains := func(s string) bool {
		if rl.Parts == nil {
			return strings.Contains(rl.Value, s)
		}
		for _, p := range rl.Parts {
			if sc, ok := p.(*StringContent); ok {
				if strings.Contains(sc.Value, s) {
					return true
				}
			}
		}
		return false
	}
	for _, dp := range pairs {
		if !contains(dp[0]) && !contains(dp[1]) {
			return dp[0], dp[1]
		}
	}
	return "{", "}"
}

// Comment represents a double quoted string in the AST
type Comment struct {
	Token token.Token // the #
	Value string
}

func (c *Comment) statementNode() {}
func (c *Comment) literalNode()   {}

// Pos returns the position of first character belonging to the node
func (c *Comment) Pos() int { return c.Token.Pos }

// End returns the position of first character immediately after the node
func (c *Comment) End() int { return c.Token.Pos + len(c.Value) }

// TokenLiteral returns the literal from token token.STRING
func (c *Comment) TokenLiteral() string { return c.Token.Literal }
func (c *Comment) String() string       { return "#" + c.Value }

// SymbolLiteral represents a symbol within the AST
type SymbolLiteral struct {
	Token token.Token // the ':'
	Value Expression
}

func (s *SymbolLiteral) expressionNode() {}
func (s *SymbolLiteral) literalNode()    {}

// Pos returns the position of first character belonging to the node
func (s *SymbolLiteral) Pos() int { return s.Token.Pos }

// End returns the position of first character immediately after the node
func (s *SymbolLiteral) End() int { return s.Value.End() }

// TokenLiteral returns the literal from token token.SYMBOL
func (s *SymbolLiteral) TokenLiteral() string { return s.Token.Literal }
func (s *SymbolLiteral) String() string       { return ":" + s.Value.String() }

// LabelString returns the symbol in label form (without leading colon).
func (s *SymbolLiteral) LabelString() string { return s.Value.String() }

// ConditionalExpression represents an if expression within the AST
type ConditionalExpression struct {
	Token       token.Token // The 'if' or 'unless' token
	EndToken    token.Token // The 'end' token
	Condition   Expression
	Consequence *BlockStatement
	Alternative *BlockStatement
}

// IsNegated indicates if the condition uses unless, i.e. is negated
func (ce *ConditionalExpression) IsNegated() bool {
	return ce.Token.Type == token.UNLESS
}

func (ce *ConditionalExpression) expressionNode() {}

// Pos returns the position of first character belonging to the node
func (ce *ConditionalExpression) Pos() int {
	if ce.EndToken.Type == token.ILLEGAL {
		return ce.Consequence.Pos()
	}
	return ce.Token.Pos
}

// End returns the position of first character immediately after the node
func (ce *ConditionalExpression) End() int {
	if ce.EndToken.Type == token.ILLEGAL {
		return ce.Consequence.Pos()
	}
	return ce.EndToken.Pos
}

// TokenLiteral returns the literal from token token.IF or token.UNLESS
func (ce *ConditionalExpression) TokenLiteral() string { return ce.Token.Literal }
func (ce *ConditionalExpression) String() string {
	var out bytes.Buffer
	if ce.Token.Type == token.QMARK {
		// Ternary: emit without outer parens. Callers that need disambiguation
		// (e.g. ParenExpression for user-written grouping) wrap explicitly.
		out.WriteString(ce.Condition.String())
		out.WriteString(" ? ")
		out.WriteString(ce.Consequence.String())
		out.WriteString(" : ")
		if ce.Alternative != nil {
			out.WriteString(ce.Alternative.String())
		}
		return out.String()
	}
	if ce.EndToken.Type == token.ILLEGAL && ce.Token.Type != token.KW_ELSIF && ce.Alternative == nil {
		out.WriteString(ce.Consequence.String())
		out.WriteString(" ")
		out.WriteString(ce.Token.Literal)
		out.WriteString(" ")
		out.WriteString(ce.Condition.String())
		return out.String()
	}
	out.WriteString(ce.Token.Literal)
	out.WriteString(" ")
	out.WriteString(ce.Condition.String())
	out.WriteString("\n")
	out.WriteString(ce.Consequence.String())
	out.WriteString("\n")
	if ce.Alternative != nil {
		if nested := extractElsif(ce.Alternative); nested != nil {
			out.WriteString(nested.String())
			return out.String()
		}
		out.WriteString("else\n")
		out.WriteString(ce.Alternative.String())
		out.WriteString("\n")
	}
	out.WriteString("end")
	return out.String()
}

func extractElsif(alt *BlockStatement) *ConditionalExpression {
	if len(alt.Statements) != 1 {
		return nil
	}
	es, ok := alt.Statements[0].(*ExpressionStatement)
	if !ok {
		return nil
	}
	ce, ok := es.Expression.(*ConditionalExpression)
	if !ok || ce.Token.Type != token.KW_ELSIF {
		return nil
	}
	return ce
}

// A LoopExpression represents a loop
type LoopExpression struct {
	Token     token.Token // while
	EndToken  token.Token // end
	Condition Expression
	Block     *BlockStatement
	// PostTest marks `begin ... end while cond` (do-while) form, where
	// the condition is checked AFTER each iteration. MRI parses this
	// as a post-test loop distinct from `while cond ... end`.
	PostTest bool
}

func (ce *LoopExpression) expressionNode() {}

// Pos returns the position of first character belonging to the node
func (ce *LoopExpression) Pos() int {
	return ce.Token.Pos
}

// End returns the position of first character immediately after the node
func (ce *LoopExpression) End() int {
	return ce.EndToken.Pos
}

// TokenLiteral returns the literal from token token.WHILE
func (ce *LoopExpression) TokenLiteral() string { return ce.Token.Literal }
func (ce *LoopExpression) String() string {
	var out bytes.Buffer
	if ce.PostTest && ce.Block != nil && len(ce.Block.Statements) == 1 {
		if es, ok := ce.Block.Statements[0].(*ExpressionStatement); ok {
			if bb, ok := es.Expression.(*ExceptionHandlingBlock); ok {
				out.WriteString(bb.String())
				out.WriteString(" ")
				out.WriteString(ce.Token.Literal)
				out.WriteString(" ")
				out.WriteString(ce.Condition.String())
				return out.String()
			}
		}
	}
	out.WriteString(ce.Token.Literal)
	out.WriteString(" ")
	out.WriteString(ce.Condition.String())
	if ce.Block != nil {
		out.WriteString("\n")
		out.WriteString(ce.Block.String())
		out.WriteString("\n")
	}
	out.WriteString("end")
	return out.String()
}

// ImplicitRest is a sentinel for the trailing comma on multi-assign LHS
// (`a, b, = X`). MRI represents this as ImplicitRestNode in the parsetree;
// we mark it in the ExpressionList tail so the printer keeps the trailing
// comma on re-emit.
type ImplicitRest struct {
	Token token.Token
}

func (i *ImplicitRest) expressionNode()      {}
func (i *ImplicitRest) literalNode()         {}
func (i *ImplicitRest) Pos() int             { return i.Token.Pos }
func (i *ImplicitRest) End() int             { return i.Token.Pos }
func (i *ImplicitRest) TokenLiteral() string { return "" }
func (i *ImplicitRest) String() string       { return "" }

// ExpressionList represents a list of expressions within the AST divided by commas
type ExpressionList []Expression

func (el ExpressionList) expressionNode() {}
func (el ExpressionList) literalNode()    {}

// Pos returns the position of first character from the first expression
func (el ExpressionList) Pos() int {
	if len(el) == 0 {
		return 0
	}
	return el[0].End()
}

// End returns End of the last element
func (el ExpressionList) End() int {
	if len(el) == 0 {
		return 0
	}
	return el[len(el)-1].End()
}

// TokenLiteral returns the literal of the first element
func (el ExpressionList) TokenLiteral() string {
	if len(el) == 0 {
		return ""
	}
	return el[0].TokenLiteral()
}
func (el ExpressionList) String() string {
	var out bytes.Buffer
	elements := []string{}
	trailingRest := false
	for _, e := range el {
		if _, ok := e.(*ImplicitRest); ok {
			trailingRest = true
			continue
		}
		elements = append(elements, e.String())
	}
	out.WriteString(strings.Join(elements, ", "))
	if trailingRest || len(elements) == 1 {
		out.WriteString(",")
	}
	return out.String()
}

// ArrayLiteral represents an Array literal within the AST
type ArrayLiteral struct {
	Token    token.Token // the '['
	Rbracket token.Token // the ']'
	Elements []Expression
}

func (al *ArrayLiteral) expressionNode() {}
func (al *ArrayLiteral) literalNode()    {}

// Pos returns the position of first character belonging to the node
func (al *ArrayLiteral) Pos() int { return al.Token.Pos }

// End returns the position of first character immediately after the node
func (al *ArrayLiteral) End() int {
	return al.Rbracket.Pos
}

// TokenLiteral returns the literal of the token token.LBRACKET
func (al *ArrayLiteral) TokenLiteral() string { return al.Token.Literal }
func (al *ArrayLiteral) String() string {
	// Preserve percent-literal arrays (`%w[a b]` / `%i[foo bar]` / `%W[...]`
	// / `%I[...]`) on roundtrip. MRI assigns these symbols/strings a
	// `forced_us_ascii_encoding` SymbolFlag when the literal source bytes
	// are ASCII -- a flag that diverges if we re-emit as a bracketed array
	// of explicit `:foo` / `"a"` elements.
	if al.Token.Type == token.STRING_BEG {
		if s, ok := al.percentArrayString(); ok {
			return s
		}
	}
	var out bytes.Buffer
	elements := []string{}
	for _, el := range al.Elements {
		elements = append(elements, el.String())
	}
	out.WriteString("[")
	out.WriteString(strings.Join(elements, ", "))
	out.WriteString("]")
	return out.String()
}

// percentArrayString re-emits a `%w` / `%W` / `%i` / `%I` array. Returns
// false if the array element shape doesn't fit the simple word-list form
// (e.g. an interpolated symbol with embedded spaces) -- the caller falls
// back to `[...]` form.
func (al *ArrayLiteral) percentArrayString() (string, bool) {
	typ := al.Token.Literal
	switch typ {
	case "w", "W", "i", "I":
	default:
		return "", false
	}
	words := make([]string, 0, len(al.Elements))
	for _, el := range al.Elements {
		var w string
		switch e := el.(type) {
		case *StringLiteral:
			if e.Parts != nil || strings.ContainsAny(e.Value, " \t\n[]") {
				return "", false
			}
			w = e.Value
		case *SymbolLiteral:
			switch v := e.Value.(type) {
			case *Identifier:
				w = v.Value
			case *StringLiteral:
				if v.Parts != nil || strings.ContainsAny(v.Value, " \t\n[]") {
					return "", false
				}
				w = v.Value
			default:
				return "", false
			}
		default:
			return "", false
		}
		words = append(words, w)
	}
	return "%" + typ + "[" + strings.Join(words, " ") + "]", true
}

// HashLiteral represents an Hash literal within the AST
type HashLiteral struct {
	Token    token.Token // the '{'
	Rbrace   token.Token // the '}'
	Map      *OrderedExprMap
	Splats   []Expression // **expr keyword-splat entries
	Implicit bool         // true for implicit hash arg (no braces in source)
}

func (hl *HashLiteral) expressionNode() {}
func (hl *HashLiteral) literalNode()    {}

// Pos returns the position of the left brace
func (hl *HashLiteral) Pos() int { return hl.Token.Pos }

// End returns the position of the right brace
func (hl *HashLiteral) End() int { return hl.Rbrace.Pos }

// TokenLiteral returns the literal of the token token.LBRACE
func (hl *HashLiteral) TokenLiteral() string { return hl.Token.Literal }
func (hl *HashLiteral) hashElements() []string {
	type posStr struct {
		pos int
		s   string
	}
	items := []posStr{}
	if hl.Map != nil {
		for _, kv := range hl.Map.Entries() {
			var s string
			if kv.Value != nil {
				if sym, ok := kv.Key.(*SymbolLiteral); ok && sym.Token.Type == token.LABEL {
					if kv.Omitted {
						s = sym.Token.Literal
					} else {
						s = sym.Token.Literal + " " + kv.Value.String()
					}
				} else if sym, ok := kv.Key.(*SymbolLiteral); ok {
					if _, isStr := sym.Value.(*StringLiteral); isStr {
						// String-label key: source was `"a": val`. Preserve label
						// form -- emitting `:"a" => val` would re-parse with a
						// different shape (and is rejected in pattern contexts).
						s = sym.Value.String() + ": " + kv.Value.String()
					} else {
						s = kv.Key.String() + " => " + kv.Value.String()
					}
				} else {
					s = kv.Key.String() + " => " + kv.Value.String()
				}
			} else {
				if sym, ok := kv.Key.(*SymbolLiteral); ok {
					s = sym.LabelString() + ":"
				} else if pe, ok := kv.Key.(*PrefixExpression); ok && pe.Operator == "**" {
					s = doubleSplatPatternKey(pe)
				} else {
					s = kv.Key.String()
				}
			}
			items = append(items, posStr{pos: kv.Key.Pos(), s: s})
		}
	}
	for _, s := range hl.Splats {
		rendered := s.String()
		// PrefixExpression / SplatExpression with `**` already emit `**`
		// (or `**X`) -- don't double-prepend. Other operand shapes are
		// bare and need the `**` prefix.
		switch e := s.(type) {
		case *PrefixExpression:
			if e.Operator == "**" {
				items = append(items, posStr{pos: s.Pos(), s: rendered})
				continue
			}
		case *SplatExpression:
			if e.Operator == "**" {
				items = append(items, posStr{pos: s.Pos(), s: rendered})
				continue
			}
		}
		items = append(items, posStr{pos: s.Pos(), s: "**" + rendered})
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].pos < items[j].pos })
	elements := make([]string, len(items))
	for i, it := range items {
		elements[i] = it.s
	}
	return elements
}

// doubleSplatPatternKey renders a `**rest` / `**nil` / bare `**` hash-pattern key
// without the parens that PrefixExpression.String() normally wraps around prefix
// operators. Wrapping breaks re-parse inside hash patterns.
func doubleSplatPatternKey(pe *PrefixExpression) string {
	if pe.Right == nil {
		return "**"
	}
	return "**" + pe.Right.String()
}

func (hl *HashLiteral) String() string {
	if hl.Implicit {
		return strings.Join(hl.hashElements(), ", ")
	}
	return "{" + strings.Join(hl.hashElements(), ", ") + "}"
}

// StringNoBraces returns the hash content without surrounding braces,
// for use in pattern matching in-clauses where implicit hash is needed.
// Uses label syntax (key: val) instead of hashrocket (key => val) for
// symbol keys so the output re-parses as an implicit hash pattern.
func (hl *HashLiteral) StringNoBraces() string {
	elements := []string{}
	if hl.Map != nil {
		for _, kv := range hl.Map.Entries() {
			if kv.Value != nil {
				if sym, ok := kv.Key.(*SymbolLiteral); ok {
					label := sym.LabelString()
					if !strings.HasSuffix(label, ":") {
						label += ":"
					}
					if kv.Omitted {
						elements = append(elements, label)
					} else {
						elements = append(elements, label+" "+kv.Value.String())
					}
				} else if sl, ok := kv.Key.(*StringLiteral); ok && sl.HeredocTag == "" {
					elements = append(elements, sl.String()+": "+kv.Value.String())
				} else {
					elements = append(elements, kv.Key.String()+" => "+kv.Value.String())
				}
			} else {
				if sym, ok := kv.Key.(*SymbolLiteral); ok {
					label := sym.LabelString()
					if !strings.HasSuffix(label, ":") {
						label += ":"
					}
					elements = append(elements, label)
				} else if pe, ok := kv.Key.(*PrefixExpression); ok && pe.Operator == "**" {
					elements = append(elements, doubleSplatPatternKey(pe))
				} else {
					elements = append(elements, kv.Key.String())
				}
			}
		}
	}
	for _, s := range hl.Splats {
		elements = append(elements, s.String())
	}
	return strings.Join(elements, ", ")
}

// A BlockCapture represents a function scoped variable capturing a block
type BlockCapture struct {
	Token token.Token // the `&`
	Name  *Identifier
	Expr  Expression // non-identifier capture: &->(x){} or &method(:f)
}

func (b *BlockCapture) expressionNode() {}
func (b *BlockCapture) literalNode()    {}

// Pos returns the position of the ampersand
func (b *BlockCapture) Pos() int { return b.Token.Pos }

// End returns the position of the last character of Name
func (b *BlockCapture) End() int {
	if b.Expr != nil {
		return b.Expr.End()
	}
	if b.Name != nil {
		return b.Name.End()
	}
	return b.Token.Pos + 1
}
func (b *BlockCapture) String() string {
	if b.Expr != nil {
		return "&" + b.Expr.String()
	}
	if b.Name != nil {
		return "&" + b.Name.Value
	}
	return "&"
}

// TokenLiteral returns the literal of the token
func (b *BlockCapture) TokenLiteral() string { return b.Token.Literal }

// A FunctionLiteral represents a function definition in the AST
type FunctionLiteral struct {
	Token         token.Token // The 'def' or '->' token
	EndToken      token.Token // the 'end' or '}' token
	Receiver      *Identifier
	Name          *Identifier
	Parameters    []*FunctionParameter
	CapturedBlock *BlockCapture
	Body          *BlockStatement
	Rescues       []*RescueBlock
	ElseBody      *BlockStatement
	EnsureBody    *BlockStatement
	IsLambda      bool // true for -> lambda literals
	// ExplicitParens marks lambdas written as ->() with an explicit (possibly
	// empty) parameter list, distinct from bare -> (no parens).
	ExplicitParens bool
}

func (fl *FunctionLiteral) expressionNode() {}
func (fl *FunctionLiteral) literalNode()    {}

// Pos returns the position of the `def` keyword
func (fl *FunctionLiteral) Pos() int { return fl.Token.Pos }

// End returns the position of the `end` keyword, or the end of the body
// for endless methods (def foo = expr).
func (fl *FunctionLiteral) End() int {
	if fl.EndToken.Type == token.ILLEGAL {
		if fl.Body != nil && len(fl.Body.Statements) > 0 {
			return fl.Body.End()
		}
		return fl.Token.Pos + 3
	}
	return fl.EndToken.Pos
}

// TokenLiteral returns the literal from token.DEF
func (fl *FunctionLiteral) TokenLiteral() string { return fl.Token.Literal }
func (fl *FunctionLiteral) String() string {
	var out bytes.Buffer
	params := []string{}
	for _, p := range fl.Parameters {
		params = append(params, p.String())
	}
	if fl.CapturedBlock != nil {
		params = append(params, fl.CapturedBlock.String())
	}
	if fl.IsLambda {
		out.WriteString("->")
	} else {
		out.WriteString("def ")
		if fl.Receiver != nil {
			needsParens := fl.Receiver.Token.Type == token.RPAREN
			if needsParens {
				out.WriteString("(")
			}
			out.WriteString(fl.Receiver.String())
			if needsParens {
				out.WriteString(")")
			}
			out.WriteString(".")
		}
		out.WriteString(fl.Name.String())
	}
	if !fl.IsLambda && fl.EndToken.Type != token.END && fl.EndToken.Type != token.ILLEGAL {
		if len(params) > 0 || fl.ExplicitParens {
			out.WriteString("(")
			out.WriteString(strings.Join(params, ", "))
			out.WriteString(")")
		}
		out.WriteString(" = ")
		if fl.Body != nil {
			out.WriteString(fl.Body.String())
		}
		return out.String()
	}
	if !fl.IsLambda || len(params) > 0 || fl.ExplicitParens {
		out.WriteString("(")
		out.WriteString(strings.Join(params, ", "))
		out.WriteString(")")
	}
	if fl.IsLambda {
		out.WriteString(" {")
	}
	body := ""
	if fl.Body != nil {
		body = fl.Body.String()
	}
	if body != "" {
		out.WriteString("\n")
		out.WriteString(body)
	}
	out.WriteString("\n")
	for _, r := range fl.Rescues {
		out.WriteString(r.String())
	}
	if fl.ElseBody != nil {
		out.WriteString("else\n")
		out.WriteString(fl.ElseBody.String())
		out.WriteString("\n")
	}
	if fl.EnsureBody != nil {
		out.WriteString("ensure\n")
		out.WriteString(fl.EnsureBody.String())
		out.WriteString("\n")
	}
	if fl.IsLambda {
		out.WriteString("}")
	} else {
		out.WriteString("end")
	}
	return out.String()
}

// A FunctionParameter represents a parameter in a function literal
type FunctionParameter struct {
	Name           *Identifier
	Default        Expression
	IsSplat        bool
	IsKeyword      bool
	IsKeywordRest  bool // **kwargs
	IsNoKeywords   bool // **nil (ruby 2.7+)
	IsForwarding   bool // ... argument forwarding
	IsImplicitRest bool // sentinel for `{|a,|}` trailing comma (MRI's ImplicitRestNode)
}

func (f *FunctionParameter) expressionNode() {}

// Pos returns the position of first character belonging to the node
func (f *FunctionParameter) Pos() int {
	if f.Name != nil {
		return f.Name.Pos()
	}
	return 0
}

// End returns the position of the default end if it exists, otherwise the end position of Name
func (f *FunctionParameter) End() int {
	if f.Default != nil {
		return f.Default.End()
	}
	if f.Name != nil {
		return f.Name.End()
	}
	return 0
}

// TokenLiteral returns the token of the parameter name
func (f *FunctionParameter) TokenLiteral() string {
	if f.Name != nil {
		return f.Name.TokenLiteral()
	}
	return ""
}
func (f *FunctionParameter) String() string {
	var out bytes.Buffer
	if f.IsImplicitRest {
		// Trailing-comma sentinel: caller's join with ", " renders this
		// as the bare comma after the last real param.
		return ""
	}
	if f.IsForwarding {
		out.WriteString("...")
		return out.String()
	}
	if f.IsSplat {
		out.WriteString("*")
	}
	if f.IsNoKeywords {
		out.WriteString("**nil")
		return out.String()
	}
	if f.IsKeywordRest {
		out.WriteString("**")
	}
	if f.Name != nil && !(f.IsKeywordRest && f.Name.Value == "**") && !(f.IsSplat && f.Name.Value == "*") {
		out.WriteString(f.Name.String())
	}
	if f.IsKeyword {
		out.WriteString(": ")
		if f.Default != nil {
			out.WriteString(encloseInParensIfNeeded(f.Default))
		}
	} else if f.Default != nil {
		out.WriteString(" = ")
		out.WriteString(encloseInParensIfNeeded(f.Default))
	}
	return out.String()
}

// An IndexExpression represents an array or hash access in the AST
type IndexExpression struct {
	Token     token.Token // The [ token
	Left      Expression
	Arguments []Expression
}

func (ie *IndexExpression) expressionNode() {}

// Pos returns the position of first character belonging to the node
func (ie *IndexExpression) Pos() int { return ie.Token.Pos }

// End returns the position of the last character belonging to the node
func (ie *IndexExpression) End() int {
	if n := len(ie.Arguments); n > 0 {
		return ie.Arguments[n-1].End()
	}
	return ie.Token.Pos
}

// TokenLiteral returns the literal from token.LBRACKET
func (ie *IndexExpression) TokenLiteral() string { return ie.Token.Literal }
func (ie *IndexExpression) String() string {
	var out bytes.Buffer
	out.WriteString(ie.Left.String())
	out.WriteString("[")
	for i, a := range ie.Arguments {
		if i > 0 {
			out.WriteString(", ")
		}
		out.WriteString(a.String())
	}
	out.WriteString("]")
	return out.String()
}

// A ContextCallExpression represents a method call on a given Context
type ContextCallExpression struct {
	Token          token.Token      // The '.' token
	Context        Expression       // The lefthandside expression
	Function       *Identifier      // The function to call
	Arguments      []Expression     // The function arguments
	Block          *BlockExpression // The function block
	ExplicitParens bool             // true when source had explicit ( ) -- preserves obj.foo() vs obj.foo and foo() vs foo (vcall)
}

func (ce *ContextCallExpression) expressionNode() {}

// Pos returns the position of first character belonging to the node
func (ce *ContextCallExpression) Pos() int {
	if ce.Context != nil {
		return ce.Context.Pos()
	}
	return ce.Function.Pos()
}

// End returns the end position of the block if it exists. If not, it returns
// the end position of the last argument if any. Otherwise it returns the end
// of the function identifier
func (ce *ContextCallExpression) End() int {
	if ce.Block != nil {
		return ce.Block.End()
	}
	if len(ce.Arguments) == 0 {
		return ce.Function.End()
	}
	return ce.Arguments[len(ce.Arguments)-1].End()
}

// TokenLiteral returns the literal from token.DOT
func (ce *ContextCallExpression) TokenLiteral() string { return ce.Token.Literal }
func (ce *ContextCallExpression) String() string {
	var out bytes.Buffer
	if ce.Context != nil {
		out.WriteString(ce.Context.String())
		switch ce.Token.Type {
		case token.LONELY:
			out.WriteString("&.")
		case token.SCOPE:
			out.WriteString("::")
		default:
			out.WriteString(".")
		}
	}
	if ce.Function != nil {
		// Setter call obj.x = 5 -- output as assignment, not obj.x=(5)
		name := ce.Function.Value
		if isSetterName(name) && len(ce.Arguments) == 1 {
			out.WriteString(strings.TrimSuffix(name, "="))
			out.WriteString(" = ")
			out.WriteString(ce.Arguments[0].String())
			return out.String()
		}
		out.WriteString(ce.Function.String())
	}
	args := []string{}
	for _, a := range ce.Arguments {
		if a != nil {
			args = append(args, a.String())
		}
	}
	if len(args) > 0 || ce.ExplicitParens {
		out.WriteString("(")
		out.WriteString(strings.Join(args, ", "))
		out.WriteString(")")
	}
	if ce.Block != nil {
		if ce.Block.Token.Type == token.LBRACE {
			out.WriteString(" ")
		}
		out.WriteString(ce.Block.String())
	}
	return out.String()
}

// A BlockExpression represents a Ruby block
type BlockExpression struct {
	Token         token.Token          // token.DO or token.LBRACE
	EndToken      token.Token          // token.END or token.RBRACE
	Parameters    []*FunctionParameter // the block parameters
	BlockLocals   []*Identifier        // block-local variables (after ; in |x; y|)
	CapturedBlock *BlockCapture        // block capture: |..., &blk|
	Body          *BlockStatement      // the block body
	Rescues       []*RescueBlock       // rescue clauses (ruby 2.5+ in do/end)
	ElseBody      *BlockStatement      // else clause (ruby 2.5+ in do/end)
	EnsureBody    *BlockStatement      // ensure clause (ruby 2.5+ in do/end)
	// HasParameterBars: source had `|...|` (possibly empty `||`). MRI Prism
	// distinguishes `{ || ... }` from `{ ... }` -- empty bars produce a
	// BlockParametersNode wrapper; absent bars omit the wrapper entirely.
	HasParameterBars bool
}

func (b *BlockExpression) expressionNode() {}

// Pos returns the position of first character belonging to the node
func (b *BlockExpression) Pos() int { return b.Token.Pos }

// End returns the position of the end token
func (b *BlockExpression) End() int { return b.EndToken.Pos }

// TokenLiteral returns the literal from the Token
func (b *BlockExpression) TokenLiteral() string { return b.Token.Literal }

// String returns a string representation of the block statement
func (b *BlockExpression) String() string {
	var out bytes.Buffer
	if b.Token.Type == token.LBRACE {
		out.WriteString("{")
	} else {
		out.WriteString(" do")
	}
	if b.HasParameterBars || len(b.Parameters) != 0 || len(b.BlockLocals) != 0 || b.CapturedBlock != nil {
		args := []string{}
		for _, a := range b.Parameters {
			args = append(args, a.String())
		}
		if b.CapturedBlock != nil {
			args = append(args, b.CapturedBlock.String())
		}
		out.WriteString(" |")
		out.WriteString(strings.Join(args, ", "))
		if len(b.BlockLocals) != 0 {
			out.WriteString("; ")
			locals := []string{}
			for _, l := range b.BlockLocals {
				locals = append(locals, l.String())
			}
			out.WriteString(strings.Join(locals, ", "))
		}
		out.WriteString("|")
	}
	out.WriteString("\n")
	out.WriteString(b.Body.String())
	for _, r := range b.Rescues {
		out.WriteString("\n")
		out.WriteString(r.String())
	}
	if b.ElseBody != nil {
		out.WriteString("\nelse\n")
		out.WriteString(b.ElseBody.String())
	}
	if b.EnsureBody != nil {
		out.WriteString("\nensure\n")
		out.WriteString(b.EnsureBody.String())
	}
	out.WriteString("\n")
	if b.Token.Type == token.LBRACE {
		out.WriteString("}")
	} else {
		out.WriteString("end")
	}
	return out.String()
}

// ModuleExpression represents a module definition
type ModuleExpression struct {
	Token    token.Token // The module keyword
	EndToken token.Token // The end token
	Name     *Identifier // The module name, will always be a const
	Body     *BlockStatement
	Rescues  []*RescueBlock
}

func (m *ModuleExpression) expressionNode() {}

// Pos returns the position of first character belonging to the node
func (m *ModuleExpression) Pos() int { return m.Token.Pos }

// End returns the position of the `end` token
func (m *ModuleExpression) End() int { return m.EndToken.Pos }

// TokenLiteral returns the literal from token.MODULE
func (m *ModuleExpression) TokenLiteral() string { return m.Token.Literal }
func (m *ModuleExpression) String() string {
	var out bytes.Buffer
	out.WriteString(m.TokenLiteral())
	out.WriteString(" ")
	out.WriteString(m.Name.String())
	out.WriteString("\n")
	out.WriteString(m.Body.String())
	for _, r := range m.Rescues {
		out.WriteString("\n")
		out.WriteString(r.String())
	}
	out.WriteString("\n")
	out.WriteString("end")
	return out.String()
}

// ClassExpression represents a module definition
type ClassExpression struct {
	Token      token.Token // The class keyword
	EndToken   token.Token // The end token
	Name       *Identifier // The class name, will always be a const
	SuperClass Expression  // The superclass, if any
	Body       *BlockStatement
	Rescues    []*RescueBlock
}

func (m *ClassExpression) expressionNode() {}

// Pos returns the position of first character belonging to the node
func (m *ClassExpression) Pos() int { return m.Token.Pos }

// End returns the position of the `end` token
func (m *ClassExpression) End() int { return m.EndToken.Pos }

// TokenLiteral returns the literal from token.CLASS
func (m *ClassExpression) TokenLiteral() string { return m.Token.Literal }
func (m *ClassExpression) String() string {
	var out bytes.Buffer
	out.WriteString(m.TokenLiteral())
	out.WriteString(" ")
	out.WriteString(m.Name.String())
	if m.SuperClass != nil {
		out.WriteString(" ")
		out.WriteString("<")
		out.WriteString(" ")
		out.WriteString(m.SuperClass.String())
	}
	out.WriteString("\n")
	out.WriteString(m.Body.String())
	for _, r := range m.Rescues {
		out.WriteString("\n")
		out.WriteString(r.String())
	}
	out.WriteString("\nend")
	return out.String()
}

// SingletonClassExpression represents a singleton class definition: class << self; ...; end
type SingletonClassExpression struct {
	Token    token.Token // the 'class' token
	EndToken token.Token // the 'end' token
	Expr     Expression  // the expression after << (e.g. self)
	Body     *BlockStatement
	Rescues  []*RescueBlock
}

func (s *SingletonClassExpression) expressionNode() {}

// Pos returns the position of first character belonging to the node
func (s *SingletonClassExpression) Pos() int { return s.Token.Pos }

// End returns the position of the 'end' token
func (s *SingletonClassExpression) End() int { return s.EndToken.Pos }

// TokenLiteral returns the literal from token.CLASS
func (s *SingletonClassExpression) TokenLiteral() string { return s.Token.Literal }
func (s *SingletonClassExpression) String() string {
	var out bytes.Buffer
	out.WriteString(s.TokenLiteral())
	out.WriteString(" << ")
	out.WriteString(s.Expr.String())
	out.WriteString("\n")
	out.WriteString(s.Body.String())
	for _, r := range s.Rescues {
		out.WriteString("\n")
		out.WriteString(r.String())
	}
	out.WriteString("\nend")
	return out.String()
}

// A SplatExpression represents a splat expression (*expr, **expr)
type SplatExpression struct {
	Token    token.Token // the * or ** token
	Operator string      // "*" or "**"
	Right    Expression
}

func (s *SplatExpression) String() string {
	var out bytes.Buffer
	out.WriteString(s.Operator)
	if s.Right != nil {
		// Splat absorbs the whole arg expression (parser uses precAssignment
		// for the operand), so no defensive wrap needed -- adding parens
		// would just produce a ParenthesesNode on re-parse.
		out.WriteString(s.Right.String())
	}
	return out.String()
}
func (s *SplatExpression) expressionNode() {}

// Pos returns the position of first character belonging to the node
func (s *SplatExpression) Pos() int { return s.Token.Pos }

// End returns the position of first character immediately after the node
func (s *SplatExpression) End() int {
	if s.Right != nil {
		return s.Right.End()
	}
	return s.Token.Pos
}

// TokenLiteral returns the literal from the * token
func (s *SplatExpression) TokenLiteral() string { return s.Token.Literal }

// ArgumentForwarding represents `...` in a call argument context: foo(...)
type ArgumentForwarding struct {
	Token token.Token // the ... token
}

func (af *ArgumentForwarding) expressionNode()      {}
func (af *ArgumentForwarding) Pos() int             { return af.Token.Pos }
func (af *ArgumentForwarding) End() int             { return af.Token.Pos + 3 }
func (af *ArgumentForwarding) TokenLiteral() string { return af.Token.Literal }
func (af *ArgumentForwarding) String() string       { return "..." }

// A CaseExpression represents a case/when or case/in expression
type CaseExpression struct {
	Token       token.Token // case
	EndToken    token.Token // end
	Condition   Expression  // optional, nil for case without expr
	WhenClauses []*WhenClause
	InClauses   []*WhenClause // pattern-matching in clauses (reuse WhenClause for now)
	ElseBody    *BlockStatement
}

func (c *CaseExpression) String() string {
	var out bytes.Buffer
	out.WriteString("case")
	if c.Condition != nil {
		out.WriteString(" ")
		out.WriteString(c.Condition.String())
	}
	out.WriteString("\n")
	for _, w := range c.WhenClauses {
		out.WriteString(w.String())
	}
	for _, in := range c.InClauses {
		out.WriteString(in.String())
	}
	if c.ElseBody != nil {
		out.WriteString("else\n")
		out.WriteString(c.ElseBody.String())
		out.WriteString("\n")
	}
	out.WriteString("end")
	return out.String()
}
func (c *CaseExpression) expressionNode() {}

func (c *CaseExpression) Pos() int             { return c.Token.Pos }
func (c *CaseExpression) End() int             { return c.EndToken.Pos }
func (c *CaseExpression) TokenLiteral() string { return c.Token.Literal }

// A WhenClause represents a single when branch in a case expression
type WhenClause struct {
	Token      token.Token // when
	Conditions []Expression
	Body       *BlockStatement
}

func (w *WhenClause) String() string {
	var out bytes.Buffer
	keyword := "when"
	if w.Token.Literal != "" {
		keyword = w.Token.Literal
	}
	out.WriteString(keyword)
	out.WriteString(" ")
	for i, cond := range w.Conditions {
		if i > 0 {
			out.WriteString(", ")
		}
		out.WriteString(cond.String())
	}
	// Pattern-matching `in` clauses need an explicit terminator between
	// pattern and body. Without it, MRI tries to continue parsing the
	// pattern when it ends with `*` (greedy match) and trips on the body.
	// `then` works for all patterns; plain `\n` only for unambiguous ones.
	if keyword == "in" {
		out.WriteString(" then")
	}
	out.WriteString("\n")
	body := w.Body.String()
	if body != "" {
		out.WriteString(body)
		out.WriteString("\n")
	}
	return out.String()
}
func (w *WhenClause) expressionNode() {}

func (w *WhenClause) Pos() int             { return w.Token.Pos }
func (w *WhenClause) End() int             { return w.Body.End() }
func (w *WhenClause) TokenLiteral() string { return w.Token.Literal }

// A DefinedExpression represents defined?(expr)
type DefinedExpression struct {
	Token token.Token
	Expr  Expression
}

func (d *DefinedExpression) String() string {
	// Space form for InfixExpression so defined? x + y (no paren node)
	// rather than defined?((x + y)) (paren node, changes parse tree).
	if _, isInfix := d.Expr.(*InfixExpression); isInfix {
		return "defined? " + d.Expr.String()
	}
	return "defined?(" + d.Expr.String() + ")"
}
func (d *DefinedExpression) expressionNode() {}

func (d *DefinedExpression) Pos() int             { return d.Token.Pos }
func (d *DefinedExpression) End() int             { return d.Expr.End() }
func (d *DefinedExpression) TokenLiteral() string { return d.Token.Literal }

// A JumpExpression represents break, next, redo, or retry with an optional value
type JumpExpression struct {
	Token token.Token // break, next, redo, or retry
	Value Expression  // optional value (nil for bare break/next/redo/retry)
}

func (j *JumpExpression) String() string {
	if j.Value != nil {
		if al, ok := j.Value.(*ArrayLiteral); ok && al.Token.Type != token.LBRACKET {
			elems := make([]string, len(al.Elements))
			for i, e := range al.Elements {
				elems[i] = e.String()
			}
			return j.Token.Literal + " " + strings.Join(elems, ", ")
		}
		return j.Token.Literal + " " + j.Value.String()
	}
	return j.Token.Literal
}
func (j *JumpExpression) expressionNode() {}

// Pos returns the position of first character belonging to the node
func (j *JumpExpression) Pos() int { return j.Token.Pos }

// End returns the position of first character immediately after the node
func (j *JumpExpression) End() int {
	if j.Value != nil {
		return j.Value.End()
	}
	return j.Token.Pos + len(j.Token.Literal)
}

// TokenLiteral returns the literal from the keyword token
func (j *JumpExpression) TokenLiteral() string { return j.Token.Literal }

// AliasExpression represents an `alias new_name old_name` statement
type AliasExpression struct {
	Token   token.Token // the alias keyword
	NewName *Identifier
	OldName *Identifier
}

func (a *AliasExpression) String() string {
	return "alias " + a.NewName.Value + " " + a.OldName.Value
}
func (a *AliasExpression) expressionNode() {}

func (a *AliasExpression) Pos() int             { return a.Token.Pos }
func (a *AliasExpression) End() int             { return a.OldName.End() }
func (a *AliasExpression) TokenLiteral() string { return a.Token.Literal }

// UndefExpression represents an `undef method1, method2, ...` statement
type UndefExpression struct {
	Token token.Token // the undef keyword
	Names []*Identifier
}

func (u *UndefExpression) String() string {
	var out bytes.Buffer
	out.WriteString("undef ")
	names := []string{}
	for _, n := range u.Names {
		names = append(names, n.Value)
	}
	out.WriteString(strings.Join(names, ", "))
	return out.String()
}
func (u *UndefExpression) expressionNode() {}

func (u *UndefExpression) Pos() int             { return u.Token.Pos }
func (u *UndefExpression) End() int             { return u.Names[len(u.Names)-1].End() }
func (u *UndefExpression) TokenLiteral() string { return u.Token.Literal }

// PrefixExpression represents a prefix operator
type PrefixExpression struct {
	Token    token.Token // The prefix token, e.g. !
	Operator string
	Right    Expression
}

func (pe *PrefixExpression) expressionNode() {}

// Pos returns the position of first character belonging to the node
func (pe *PrefixExpression) Pos() int { return pe.Token.Pos }

// End returns the end of the right expression
func (pe *PrefixExpression) End() int { return pe.Right.End() }

// TokenLiteral returns the literal from the prefix operator token
func (pe *PrefixExpression) TokenLiteral() string { return pe.Token.Literal }
func pinNeedsParens(right Expression) bool {
	switch right.(type) {
	case *Identifier, *InstanceVariable, *ClassVariable, *Global,
		*IntegerLiteral, *FloatLiteral, *StringLiteral, *SymbolLiteral,
		*Boolean, *Nil, *ParenExpression,
		*PrefixExpression:
		return false
	}
	// InfixExpression no longer self-wraps in parens after the
	// precedence-aware printer; pin operator (^) needs to wrap them
	// explicitly so ^n * 2 doesn't bind as (^n) * 2.
	return true
}

// operandHasLeadingSpace reports whether the operand's source-side
// first token had whitespace before it -- used to decide if a unary
// `-` / `+` should emit a space separator on re-print (to preserve
// MRI's CallNode shape for the spaced form).
func operandHasLeadingSpace(e Expression) bool {
	switch n := e.(type) {
	case *IntegerLiteral:
		return n.Token.HadWhitespace
	case *FloatLiteral:
		return n.Token.HadWhitespace
	}
	return false
}

func (pe *PrefixExpression) String() string {
	// Drop the outer parens when the operand is "atomic" enough that
	// MRI wouldn't add a ParenthesesNode on re-parse. The exact rules
	// differ per operator: - / + fold a numeric literal into a single
	// NODE_LIT; ! / ~ are calls that bind tightly so chaining (!!x,
	// ~~x) and applying to simple variables / calls doesn't need
	// parens around the operator.
	atomic := false
	switch pe.Operator {
	case "-", "+":
		switch r := pe.Right.(type) {
		case *IntegerLiteral, *FloatLiteral,
			*ContextCallExpression, *IndexExpression,
			*Identifier, *InstanceVariable, *ClassVariable, *Global,
			*Self, *Nil, *Boolean, *ScopedIdentifier,
			*ParenExpression, *StringLiteral:
			// MRI folds leading negative numeric via tUMINUS_NUM; for
			// other terminals / call / index chains / explicit-paren
			// groups / string literals (frozen-string `-"str"` form)
			// the unary binds to the result either way so no
			// outer parens needed.
			atomic = true
		case *InfixExpression:
			// `**` binds tighter than unary -/+: `-x**y` parses as
			// `-(x**y)` directly. Skip the defensive wrap.
			if r.Operator == "**" {
				atomic = true
			}
		}
	case "!", "~":
		switch pe.Right.(type) {
		case *Identifier, *InstanceVariable, *ClassVariable, *Global,
			*Boolean, *Nil, *Self, *IntegerLiteral, *FloatLiteral,
			*ParenExpression, *PrefixExpression, *ContextCallExpression,
			*IndexExpression, *ScopedIdentifier,
			*DefinedExpression, *YieldExpression, *SuperExpression,
			*RegexLiteral, *StringLiteral, *SymbolLiteral,
			*ArrayLiteral, *HashLiteral:
			atomic = true
		}
	}
	// `not` is the lowest-precedence unary; any embedding context (assign rhs,
	// arg list, infix operand, if/while head) accepts `not X` without an
	// outer ParenthesesNode wrap. `*` / `**` (splat / double-splat) only
	// appear in grammar-delimited positions and don't need a wrap either.
	wrap := pe.Operator != "^" && pe.Operator != "not" &&
		pe.Operator != "*" && pe.Operator != "**" && !atomic

	var out bytes.Buffer
	if wrap {
		out.WriteString("(")
	}
	out.WriteString(pe.Operator)
	if pe.Right != nil {
		// `not` form selection:
		// - pre-2.0 MRI rejects `not X` for non-atomic X (require parens).
		// - on modern MRI, `not(X)` and `not X` normalise to the same
		//   CallNode (opening_loc / closing_loc are stripped).
		// - empty grouped expr `not ()` -> ParenExpression{Expr:nil}.
		//   Emit with a space to keep the source-level form (`not()` is a
		//   different MRI shape -- call-paren with no args, NODE_BEGIN(nil)
		//   recv vs receiver=nil).
		if pe.Operator == "not" {
			needWrap := true
			switch r := pe.Right.(type) {
			case *Identifier, *InstanceVariable, *ClassVariable, *Global,
				*Boolean, *Nil, *Self, *IntegerLiteral, *FloatLiteral,
				*ScopedIdentifier, *RegexLiteral, *StringLiteral,
				*SymbolLiteral, *ArrayLiteral, *HashLiteral:
				// Atomic-enough that pre-2.0 MRI accepts `not X` without
				// requiring parens. ContextCallExpression / IndexExpression
				// are NOT here -- pre-2.0 rejects `not x.foo` / `not x[0]`.
				needWrap = false
			case *ParenExpression:
				if r.Expr == nil && len(r.Stmts) == 0 {
					// `not ()` -- preserve with space + inner empty parens
					out.WriteString(" ")
					out.WriteString(r.String())
					if wrap {
						out.WriteString(")")
					}
					return out.String()
				}
				// `not (X)` already provides outer parens via ParenExpression
				needWrap = false
			}
			if needWrap {
				out.WriteString("(")
				out.WriteString(pe.Right.String())
				out.WriteString(")")
			} else {
				out.WriteString(" ")
				out.WriteString(pe.Right.String())
			}
			if wrap {
				out.WriteString(")")
			}
			return out.String()
		}
		if pe.Operator == "defined?" {
			out.WriteString(" ")
		}
		// For unary `-` / `+`: if the source had whitespace between the
		// operator and the operand, MRI preserves the unary as a CallNode
		// (e.g. `[- 1]` -> `[-@(1)]`); the no-space form `-1` folds to a
		// negative literal. Emit a separating space to keep the call form
		// on re-parse.
		if (pe.Operator == "-" || pe.Operator == "+") && operandHasLeadingSpace(pe.Right) {
			out.WriteString(" ")
		}
		needsParens := pe.Operator == "^" && pinNeedsParens(pe.Right)
		if needsParens {
			out.WriteString("(")
		}
		out.WriteString(pe.Right.String())
		if needsParens {
			out.WriteString(")")
		}
	}
	if wrap {
		out.WriteString(")")
	}
	return out.String()
}

// An InfixExpression represents an infix operator in the AST
type InfixExpression struct {
	Token    token.Token // The operator token, e.g. +
	Left     Expression
	Operator string
	Right    Expression
}

// MustEvaluateRight returns true if it is mandatory to evaluate the right side
// of the operator, false otherwise
func (oe *InfixExpression) MustEvaluateRight() bool {
	return oe.Token.Type != token.LOGICALOR
}

// IsControlExpression returns true if the infix is used for control flow,
// false otherwise
func (oe *InfixExpression) IsControlExpression() bool {
	return oe.Token.Type == token.LOGICALOR || oe.Token.Type == token.LOGICALAND
}

func (oe *InfixExpression) expressionNode() {}

// Pos returns the position of first character belonging to the left node
func (oe *InfixExpression) Pos() int {
	if oe.Left != nil {
		return oe.Left.Pos()
	}
	return oe.Token.Pos
}

// End returns the position of last character belonging to the right node
func (oe *InfixExpression) End() int {
	if oe.Right != nil {
		return oe.Right.End()
	}
	return oe.Token.Pos + len(oe.Token.Literal)
}

// TokenLiteral returns the literal from the infix operator token
func (oe *InfixExpression) TokenLiteral() string { return oe.Token.Literal }
// rubyInfixPrec returns Ruby operator precedence (higher = binds tighter).
// Returns 0 for unknown operators, which falls back to the conservative
// always-wrap behaviour in InfixExpression.String().
func rubyInfixPrec(op string) int {
	switch op {
	case "**":
		return 15
	case "*", "/", "%":
		return 13
	case "+", "-":
		return 12
	case "<<", ">>":
		return 11
	case "&":
		return 10
	case "|", "^":
		return 9
	case "<", "<=", ">", ">=":
		return 8
	case "==", "!=", "===", "=~", "!~", "<=>":
		return 7
	case "&&":
		return 6
	case "||":
		return 5
	case "..", "...":
		return 4
	case "?:":
		return 3
	case "rescue":
		// Modifier rescue binds tighter than assignment but looser than ternary
		// so `a = b rescue c` parses as `a = (b rescue c)` and `a ? b : c rescue d`
		// parses as `(a ? b : c) rescue d`.
		return 3
	case "=", "+=", "-=", "*=", "/=", "%=", "**=", "<<=", ">>=", "&=", "|=", "^=", "&&=", "||=":
		return 2
	case "and", "or":
		return 1
	case "in":
		// `in` only appears as part of `for X in Y` and pattern-match
		// `case ... in ...`. Treat as low-precedence non-wrapping so the
		// printer doesn't add ParenthesesNode around `i in 0..10`.
		return 1
	case "if", "unless":
		// Pattern-guard form: `in <pattern> if <cond>` -- stored as
		// InfixExpression by the pattern parser. The grammar already
		// delimits the guard, so no defensive wrap is needed.
		return 1
	case "=>":
		// Pattern capture (`in Integer => n`) -- stored as InfixExpression
		// by the pattern parser. Grammar-delimited, no wrap needed.
		return 1
	}
	return 0
}

// rubyInfixRightAssoc reports whether the operator is right-associative.
// Only ** and the assignment operators are right-associative in Ruby.
func rubyInfixRightAssoc(op string) bool {
	switch op {
	case "**", "=", "+=", "-=", "*=", "/=", "%=", "**=", "<<=", ">>=", "&=", "|=", "^=", "&&=", "||=":
		return true
	}
	return false
}

func (oe *InfixExpression) String() string {
	if oe.Operator == ":" {
		if sym, ok := oe.Left.(*SymbolLiteral); ok && sym.Token.Type == token.LABEL {
			if oe.Right == nil {
				// Hash-value-omission shorthand (Ruby 3.1+): `foo:` with
				// implicit value. Preserve the omitted form on re-emit.
				return sym.Token.Literal
			}
			return sym.Token.Literal + " " + oe.Right.String()
		}
	}
	parentPrec := rubyInfixPrec(oe.Operator)
	rightAssoc := rubyInfixRightAssoc(oe.Operator)
	// parentPrec == 0 means unknown operator -- fall back to conservative
	// always-wrap so unfamiliar ops can't change parse on re-read.
	wrapAll := parentPrec == 0

	render := func(child Expression, isLeft bool) string {
		if child == nil {
			return ""
		}
		s := child.String()
		inf, ok := child.(*InfixExpression)
		if !ok {
			return s
		}
		// Modifier `rescue` parses its RHS via the `expr` grammar (not `arg`),
		// so anything down to `and`/`or` is absorbed without parens. Skipping
		// the defensive wrap matches MRI's parsetree (rescue_expression =
		// AndNode directly, not ParenthesesNode(AndNode)).
		if oe.Operator == "rescue" && !isLeft {
			return s
		}
		childPrec := rubyInfixPrec(inf.Operator)
		if childPrec == 0 {
			return s // child already wraps itself
		}
		var need bool
		switch {
		case childPrec < parentPrec:
			need = true
		case childPrec == parentPrec:
			if isLeft {
				need = rightAssoc
			} else {
				need = !rightAssoc
			}
		}
		if !need {
			// strip the child's own outer wrap if it added one
			return s
		}
		return "(" + s + ")"
	}

	var out bytes.Buffer
	if wrapAll {
		out.WriteString("(")
	}
	out.WriteString(render(oe.Left, true))
	out.WriteString(" " + oe.Operator + " ")
	out.WriteString(render(oe.Right, false))
	if wrapAll {
		out.WriteString(")")
	}
	return out.String()
}

// RightwardAssignment represents a rightward assignment / one-line pattern match (expr => target)
type RightwardAssignment struct {
	Token token.Token // the => token
	Left  Expression  // the value expression
	Right Expression  // the pattern / target
}

func (ra *RightwardAssignment) expressionNode() {}

func (ra *RightwardAssignment) Pos() int             { return ra.Left.Pos() }
func (ra *RightwardAssignment) End() int             { return ra.Right.End() }
func (ra *RightwardAssignment) TokenLiteral() string { return ra.Token.Literal }
func (ra *RightwardAssignment) String() string {
	var out bytes.Buffer
	out.WriteString(ra.Left.String())
	out.WriteString(" => ")
	out.WriteString(ra.Right.String())
	return out.String()
}

// ParenExpression wraps a parenthesised expression, preserving the parens
// for source roundtrip fidelity. (a; b; c) groups multiple statements,
// stored in Stmts; the single-expression case uses Expr.
type ParenExpression struct {
	Token  token.Token // the '(' token
	Rparen token.Token // the ')' token
	Expr   Expression
	Stmts  []Expression // multi-statement form: (a; b; c). nil for single-expr.
}

func (pe *ParenExpression) expressionNode()      {}
func (pe *ParenExpression) Pos() int             { return pe.Token.Pos }
func (pe *ParenExpression) End() int             { return pe.Rparen.Pos }
func (pe *ParenExpression) TokenLiteral() string { return pe.Token.Literal }
func (pe *ParenExpression) String() string {
	if len(pe.Stmts) > 0 {
		parts := make([]string, len(pe.Stmts))
		for i, s := range pe.Stmts {
			parts[i] = s.String()
		}
		return "(" + strings.Join(parts, "; ") + ")"
	}
	if pe.Expr == nil {
		return "()"
	}
	// Skip the wrap when Expr already emits its own outer parens.
	switch e := pe.Expr.(type) {
	case *ParenExpression:
		return pe.Expr.String()
	case *PrefixExpression:
		// PrefixExpression never self-wraps `not`, and self-wraps -/+/!/~
		// only when operand is non-atomic; otherwise it emits bare and
		// ParenExpression must wrap to preserve grouping (otherwise MRI
		// doesn't see a ParenthesesNode on re-parse).
		s := e.String()
		if !(strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")")) {
			return "(" + s + ")"
		}
		return s
	case *InfixExpression:
		// Unknown-operator fallback in InfixExpression.String() wraps in ().
		if rubyInfixPrec(e.Operator) == 0 {
			return e.String()
		}
	}
	return "(" + pe.Expr.String() + ")"
}

func escapeRegexSlash(s string) string {
	if !strings.Contains(s, "/") || strings.Contains(s, "\\/") {
		return s
	}
	return strings.ReplaceAll(s, "/", "\\/")
}

// isSetterName reports whether name is an identifier-style setter (foo=),
// not an operator method that happens to end with = (==, !=, <=, >=, ===, =~).
func isSetterName(name string) bool {
	if !strings.HasSuffix(name, "=") || len(name) < 2 {
		return false
	}
	// The character before the trailing = must be a letter, digit, or underscore.
	prev := name[len(name)-2]
	return (prev >= 'a' && prev <= 'z') || (prev >= 'A' && prev <= 'Z') ||
		(prev >= '0' && prev <= '9') || prev == '_'
}

func encloseInParensIfNeeded(expr Expression) string {
	val := expr.String()
	hasParens := strings.HasPrefix(val, "(") && strings.HasSuffix(val, ")")
	_, isLiteral := expr.(literal)
	// Treat - / + on a numeric literal as a (negative/positive) literal --
	// PrefixExpression.String() emits -1 without wrapping, but it's still
	// safely an atomic default-value form.
	if pe, ok := expr.(*PrefixExpression); ok && (pe.Operator == "-" || pe.Operator == "+") {
		switch pe.Right.(type) {
		case *IntegerLiteral, *FloatLiteral:
			isLiteral = true
		}
	}
	// Atomic enough to not need a defensive wrap: identifiers, instance /
	// class / global vars, scoped names, method calls (including chained
	// .new), index access, self/nil/booleans -- MRI parses these as the
	// default value directly without a ParenthesesNode.
	switch e := expr.(type) {
	case *Identifier, *InstanceVariable, *ClassVariable, *Global,
		*ScopedIdentifier, *ContextCallExpression, *IndexExpression,
		*Self, *Nil, *Boolean, *Keyword__FILE__, *Keyword__DIR__,
		*Keyword__ENCODING__, *Keyword__CALLEE__, *Keyword__METHOD__,
		*SymbolLiteral:
		isLiteral = true
	case *InfixExpression:
		// Most binary operators bind tighter than `,` and `)` so they parse
		// fine as a default. `and`/`or` are below assignment and would be
		// ambiguous; everything else is safe.
		if e.Operator != "and" && e.Operator != "or" {
			isLiteral = true
		}
	}
	if !isLiteral && !hasParens {
		val = "(" + val + ")"
	}
	return val
}
