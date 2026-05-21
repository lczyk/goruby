package ast

import (
	"bytes"
	"math/big"
	"sort"
	"strconv"
	"strings"

	"github.com/lczyk/goruby/token"
)

// isComparableExpr reports whether an Expression's dynamic type is
// comparable with ==. Slice-backed AST types (currently only
// ExpressionList) are not, and would panic on direct interface
// comparison.
func isComparableExpr(e Expression) bool {
	if e == nil {
		return false
	}
	_, isList := e.(ExpressionList)
	return !isList
}

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
	// WriteTo writes the rendering of this node into the shared builder.
	// Equivalent to String() but lets callers avoid the intermediate
	// allocation when assembling parent strings. Always present so the
	// recursive print path is alloc-free.
	WriteTo(b *strings.Builder)
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

// writeTo writes the rendering of n into b. Every Node implements WriteTo
// per the interface contract, so this is a direct method call with no
// type-assert overhead.
func writeTo(n Node, b *strings.Builder) {
	n.WriteTo(b)
}

// Format renders n into a string. Equivalent to n.String() but routes
// through the shared-builder WriteTo path so a node's children render
// straight into the same buffer.
func Format(n Node) string {
	var b strings.Builder
	n.WriteTo(&b)
	return b.String()
}

// A Program node is the root node within the AST.
type Program struct {
	pos        int
	Statements []Statement
	// arena keeps the parser's bump-allocated chunks reachable for the
	// lifetime of the Program. nil for hand-built ASTs (tests, fixtures).
	arena *Arena
	// LitPool carries the per-parse literal pool finalised at parse end.
	// Tokens reference text via LitOff indices into this slice, so the
	// AST keeps its literal text reachable after the parser drops the
	// source bytes. Nil for hand-built ASTs (tests, fixtures) whose
	// tokens carry their text via dedicated AST fields (.Value etc).
	LitPool []string
	// Filename is the source path the parser was invoked with (the
	// `filename` arg to ParseFile). Used by the evaluator to resolve
	// `require_relative` paths and to stamp `__FILE__` literals. Empty
	// for hand-built ASTs.
	Filename string
}

// Arena returns the bump allocator used to construct this Program's AST
// nodes, or nil if the program was built by hand without one.
func (p *Program) Arena() *Arena { return p.arena }

// SetArena attaches an arena to the program so its chunks survive as long
// as the program reference does. Used by the parser at construction time;
// callers normally don't need to invoke it.
func (p *Program) SetArena(a *Arena) { p.arena = a }

// Pos returns the position of first character belonging to the node
func (p *Program) Pos() int { return p.pos }

// End returns the position of first character immediately after the node
func (p *Program) End() int {
	if len(p.Statements) == 0 {
		return p.pos
	}
	return p.Statements[len(p.Statements)-1].End()
}
func (p *Program) WriteTo(b *strings.Builder) {
	// Program's String has post-processing (relocateHeredocBodies + magic
	// encoding header). WriteTo defers to String so the post-processing
	// runs; this means embedding a Program as a child is a one-shot copy.
	// In practice Program is only ever the root, so this is fine.
	b.WriteString(p.String())
}

func (p *Program) String() string {
	var out strings.Builder
	first := true
	for _, s := range p.Statements {
		if s == nil {
			continue
		}
		if !first {
			out.WriteByte('\n')
		}
		writeTo(s, &out)
		first = false
	}
	body := relocateHeredocBodies(out.String())
	// Re-add a magic encoding comment when the body contains non-ASCII
	// bytes: the source likely had one (we drop comments on re-emit) and
	// without it MRI 1.9 / our 1.9-mode lexer rejects the file. Don't
	// prepend when an existing magic encoding/coding comment is already on
	// the first or second line -- otherwise we'd overrule a source that
	// explicitly declared (say) ASCII-8BIT with our utf-8 default.
	if containsNonAscii(body) && !hasLeadingMagicEncoding(body) {
		body = "# encoding: utf-8\n" + body
	}
	return body
}

// hasLeadingMagicEncoding reports whether the first or second line of body
// carries a Ruby magic encoding comment. Mirrors MRI's recognition: the
// line must be a `#` comment containing `coding:` or `coding=` somewhere.
// That covers the explicit `# encoding: X` / `# coding: X` form, the emacs
// modeline `# -*- coding: X -*-`, and the vim modeline
// `# vim: set fileencoding=X` (which MRI accepts because the substring
// `encoding=X` matches the keyword scan).
func hasLeadingMagicEncoding(body string) bool {
	for i, line := 0, 0; line < 2 && i < len(body); line++ {
		end := i
		for end < len(body) && body[end] != '\n' {
			end++
		}
		raw := body[i:end]
		if end >= len(body) {
			i = end
		} else {
			i = end + 1
		}
		trimmed := strings.TrimLeft(raw, " \t")
		if !strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.Contains(trimmed, "coding:") || strings.Contains(trimmed, "coding=") {
			return true
		}
	}
	return false
}

func containsNonAscii(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return true
		}
	}
	return false
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
	var b strings.Builder
	rs.WriteTo(&b)
	return b.String()
}

func (rs *ReturnStatement) WriteTo(b *strings.Builder) {
	b.WriteString(rs.TokenLiteral())
	if rs.ReturnValue != nil {
		b.WriteByte(' ')
		if al, ok := rs.ReturnValue.(*ArrayLiteral); ok && al.Token.Type != token.LBRACKET {
			for i, e := range al.Elements {
				if i > 0 {
					b.WriteString(", ")
				}
				writeTo(e, b)
			}
		} else {
			writeTo(rs.ReturnValue, b)
		}
	}
}
func (rs *ReturnStatement) statementNode() {}

// TokenLiteral returns the 'return' token literal
func (rs *ReturnStatement) TokenLiteral() string { return rs.Token.Type.Literal() }

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
	var b strings.Builder
	es.WriteTo(&b)
	return b.String()
}

func (es *ExpressionStatement) WriteTo(b *strings.Builder) {
	if es.Expression != nil {
		writeTo(es.Expression, b)
	}
}
func (es *ExpressionStatement) statementNode() {}

// Pos returns the position of first character belonging to the node
func (es *ExpressionStatement) Pos() int { return es.Expression.Pos() }

// End returns the position of first character immediately after the node
func (es *ExpressionStatement) End() int { return es.Expression.End() }

// TokenLiteral returns the first token of the Expression
func (es *ExpressionStatement) TokenLiteral() string {
	if es.Expression != nil {
		return es.Expression.TokenLiteral()
	}
	return ""
}

// BlockStatement represents a list of statements
type BlockStatement struct {
	// the { token or the first token from the first statement
	Token      token.Token
	EndPos     int // pos of the closing token (} / end / etc); 0 if unterminated
	Statements []Statement
}

func (bs *BlockStatement) statementNode() {}

// Pos returns the position of first character belonging to the node
func (bs *BlockStatement) Pos() int { return bs.Token.Pos }

// End returns the position of first character immediately after the node
func (bs *BlockStatement) End() int { return bs.EndPos }

// TokenLiteral returns '{' or the first token from the first statement
func (bs *BlockStatement) TokenLiteral() string {
	if len(bs.Statements) > 0 && bs.Statements[0] != nil {
		return bs.Statements[0].TokenLiteral()
	}
	return bs.Token.Type.Literal()
}
func (bs *BlockStatement) String() string {
	var b strings.Builder
	bs.WriteTo(&b)
	return b.String()
}

func (bs *BlockStatement) WriteTo(b *strings.Builder) {
	first := true
	for _, s := range bs.Statements {
		if s == nil {
			continue
		}
		if !first {
			b.WriteByte('\n')
		}
		writeTo(s, b)
		first = false
	}
}

// ExceptionHandlingBlock represents a begin/end block where exceptions are rescued
type ExceptionHandlingBlock struct {
	BeginToken token.Token
	EndPos     int // pos of the closing `end`
	TryBody    *BlockStatement
	Rescues    []*RescueBlock
	ElseBody   *BlockStatement
	EnsureBody *BlockStatement
}

func (eh *ExceptionHandlingBlock) expressionNode() {}

// Pos returns the position of first character belonging to the node
func (eh *ExceptionHandlingBlock) Pos() int { return eh.BeginToken.Pos }

// End returns the position of first character immediately after the node
func (eh *ExceptionHandlingBlock) End() int { return eh.EndPos }

// TokenLiteral returns the token literal from 'begin'
func (eh *ExceptionHandlingBlock) TokenLiteral() string { return eh.BeginToken.Type.Literal() }
func (eh *ExceptionHandlingBlock) String() string {
	var b strings.Builder
	eh.WriteTo(&b)
	return b.String()
}

func (eh *ExceptionHandlingBlock) WriteTo(b *strings.Builder) {
	b.WriteString(eh.BeginToken.Type.Literal())
	b.WriteByte('\n')
	writeTo(eh.TryBody, b)
	b.WriteByte('\n')
	for _, r := range eh.Rescues {
		writeTo(r, b)
	}
	if eh.ElseBody != nil {
		b.WriteString("else\n")
		writeTo(eh.ElseBody, b)
		b.WriteByte('\n')
	}
	if eh.EnsureBody != nil {
		b.WriteString("ensure\n")
		writeTo(eh.EnsureBody, b)
		b.WriteByte('\n')
	}
	b.WriteString("end")
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
func (rb *RescueBlock) TokenLiteral() string { return rb.Token.Type.Literal() }
func (rb *RescueBlock) String() string {
	var b strings.Builder
	rb.WriteTo(&b)
	return b.String()
}

func (rb *RescueBlock) WriteTo(b *strings.Builder) {
	b.WriteString("rescue")
	if len(rb.ExceptionClasses) != 0 {
		b.WriteByte(' ')
		for i, c := range rb.ExceptionClasses {
			if i > 0 {
				b.WriteString(", ")
			}
			writeTo(c, b)
		}
	}
	if rb.Exception != nil {
		b.WriteString(" => ")
		writeTo(rb.Exception, b)
	}
	b.WriteByte('\n')
	writeTo(rb.Body, b)
	b.WriteByte('\n')
}

// Assignment represents a generic assignment
type Assignment struct {
	Token token.Token
	Left  Expression
	Right Expression
}

func (a *Assignment) String() string {
	var b strings.Builder
	a.WriteTo(&b)
	return b.String()
}

func (a *Assignment) WriteTo(b *strings.Builder) {
	writeTo(a.Left, b)
	op := a.Token.Type.Literal()
	if op == "" {
		op = "="
	}
	b.WriteByte(' ')
	b.WriteString(op)
	b.WriteByte(' ')
	rhs := a.Right
	if op != "=" {
		// Compound op-assignments (`a += 1`) are parsed as
		// `a = a + 1` where the inner Left aliases the outer Left
		// (same pointer, set in parseAssignmentOperator). Detect that
		// alias to suppress the redundant lhs in the RHS. Pointer
		// compare -- not String() -- because String() comparison on
		// nested compound assignments is exponential. Skip the check
		// when Left is a non-comparable type (e.g. ExpressionList, a
		// slice) -- those cannot be the LHS of an op-assignment desugar
		// anyway.
		if inf, ok := rhs.(*InfixExpression); ok && isComparableExpr(a.Left) && isComparableExpr(inf.Left) && inf.Left == a.Left {
			rhs = inf.Right
		}
	}
	writeTo(rhs, b)
}
func (a *Assignment) expressionNode() {}

// Pos returns the position of first character belonging to the node
func (a *Assignment) Pos() int { return a.Left.Pos() }

// End returns the position of first character immediately after the node
func (a *Assignment) End() int { return a.Right.End() }

// TokenLiteral returns the literal of the ASSIGN token
func (a *Assignment) TokenLiteral() string { return a.Token.Type.Literal() }

// An InstanceVariable represents an instance variable in the AST
type InstanceVariable struct {
	Token token.Token
	Name  *Identifier
}

func (i *InstanceVariable) String() string {
	var b strings.Builder
	i.WriteTo(&b)
	return b.String()
}

func (i *InstanceVariable) WriteTo(b *strings.Builder) {
	b.WriteString(i.Token.Type.Literal())
	writeTo(i.Name, b)
}
func (i *InstanceVariable) literalNode()    {}
func (i *InstanceVariable) expressionNode() {}

// Pos returns the position of first character belonging to the node
func (i *InstanceVariable) Pos() int { return i.Token.Pos }

// End returns the position of first character immediately after the node
func (i *InstanceVariable) End() int { return i.Name.End() }

// TokenLiteral returns the literal of the AT token
func (i *InstanceVariable) TokenLiteral() string { return i.Token.Type.Literal() }

// A ClassVariable represents a class variable in the AST
type ClassVariable struct {
	Token token.Token
	Name  *Identifier
}

func (c *ClassVariable) String() string {
	var b strings.Builder
	c.WriteTo(&b)
	return b.String()
}

func (c *ClassVariable) WriteTo(b *strings.Builder) {
	b.WriteString(c.Token.Type.Literal())
	writeTo(c.Name, b)
}
func (c *ClassVariable) literalNode()    {}
func (c *ClassVariable) expressionNode() {}

// Pos returns the position of first character belonging to the node
func (c *ClassVariable) Pos() int { return c.Token.Pos }

// End returns the position of first character immediately after the node
func (c *ClassVariable) End() int { return c.Name.End() }

// TokenLiteral returns the literal of the CLASS_VAR token
func (c *ClassVariable) TokenLiteral() string { return c.Token.Type.Literal() }

// MultiAssignment represents multiple variables on the lefthand side
type MultiAssignment struct {
	Variables []*Identifier
	Values    []Expression
}

func (m *MultiAssignment) String() string {
	var b strings.Builder
	m.WriteTo(&b)
	return b.String()
}

func (m *MultiAssignment) WriteTo(b *strings.Builder) {
	for i, v := range m.Variables {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(v.Value)
	}
	b.WriteString(" = ")
	for i, v := range m.Values {
		if i > 0 {
			b.WriteString(", ")
		}
		writeTo(v, b)
	}
}
func (m *MultiAssignment) literalNode() {}

// Pos returns the position of first character belonging to the node
func (m *MultiAssignment) Pos() int { return m.Variables[0].Pos() }

// End returns the position of first character immediately after the node
func (m *MultiAssignment) End() int        { return m.Values[len(m.Values)-1].End() }
func (m *MultiAssignment) expressionNode() {}

// TokenLiteral returns the literal of the first variable token
func (m *MultiAssignment) TokenLiteral() string { return m.Variables[0].TokenLiteral() }

// Self represents self in the current context in the program.
// Pointer-free (just a Pos) so arena chunks of Self are noscan-eligible.
type Self struct {
	PosOff int
}

func (s *Self) String() string             { return "self" }
func (s *Self) WriteTo(b *strings.Builder) { b.WriteString("self") }
func (s *Self) expressionNode() {}
func (s *Self) literalNode()    {}

// Pos returns the position of first character belonging to the node
func (s *Self) Pos() int { return s.PosOff }

// End returns the position of first character immediately after the node
func (s *Self) End() int { return s.PosOff + 4 }

// TokenLiteral returns the literal of the token.SELF token
func (s *Self) TokenLiteral() string { return "self" }

// YieldExpression represents self in the current context in the program
type YieldExpression struct {
	Token     token.Token      // the token.YIELD token
	Arguments []Expression     // The arguments to yield
	Block     *BlockExpression // optional block passed to yield
}

func (y *YieldExpression) String() string {
	var b strings.Builder
	y.WriteTo(&b)
	return b.String()
}

func (y *YieldExpression) WriteTo(b *strings.Builder) {
	b.WriteString(y.Token.Type.Literal())
	if len(y.Arguments) != 0 {
		b.WriteByte('(')
		for i, a := range y.Arguments {
			if i > 0 {
				b.WriteString(", ")
			}
			writeTo(a, b)
		}
		b.WriteByte(')')
	}
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
func (y *YieldExpression) TokenLiteral() string { return y.Token.Type.Literal() }

// SuperExpression represents a `super` call with optional arguments
type SuperExpression struct {
	Token     token.Token      // the token.KW_SUPER token
	Arguments []Expression     // optional explicit arguments; nil means implicit forwarding
	Block     *BlockExpression // optional block passed to super
}

func (s *SuperExpression) String() string {
	var b strings.Builder
	s.WriteTo(&b)
	return b.String()
}

func (s *SuperExpression) WriteTo(b *strings.Builder) {
	b.WriteString(s.Token.Type.Literal())
	if s.Arguments != nil {
		b.WriteByte('(')
		for i, a := range s.Arguments {
			if i > 0 {
				b.WriteString(", ")
			}
			writeTo(a, b)
		}
		b.WriteByte(')')
	}
	if s.Block != nil {
		if s.Block.Token.Type == token.LBRACE {
			b.WriteByte(' ')
		}
		writeTo(s.Block, b)
	}
}
func (s *SuperExpression) expressionNode() {}

func (s *SuperExpression) Pos() int { return s.Token.Pos }
func (s *SuperExpression) End() int {
	if len(s.Arguments) == 0 {
		return s.Token.EndPos()
	}
	return s.Arguments[len(s.Arguments)-1].End()
}
func (s *SuperExpression) TokenLiteral() string { return s.Token.Type.Literal() }

// BeginBlock represents a top-level BEGIN { ... } block
type BeginBlock struct {
	Token token.Token // the BEGIN token
	Body  *BlockStatement
}

func (b *BeginBlock) expressionNode()      {}
func (b *BeginBlock) Pos() int             { return b.Token.Pos }
func (b *BeginBlock) End() int             { return b.Body.End() }
func (b *BeginBlock) TokenLiteral() string { return b.Token.Type.Literal() }
func (b *BeginBlock) String() string {
	var sb strings.Builder
	b.WriteTo(&sb)
	return sb.String()
}

func (b *BeginBlock) WriteTo(sb *strings.Builder) {
	sb.WriteString("BEGIN {\n")
	writeTo(b.Body, sb)
	sb.WriteString("\n}")
}

// EndBlock represents a top-level END { ... } block
type EndBlock struct {
	Token token.Token // the END token
	Body  *BlockStatement
}

func (e *EndBlock) expressionNode()      {}
func (e *EndBlock) Pos() int             { return e.Token.Pos }
func (e *EndBlock) End() int             { return e.Body.End() }
func (e *EndBlock) TokenLiteral() string { return e.Token.Type.Literal() }
func (e *EndBlock) String() string {
	var b strings.Builder
	e.WriteTo(&b)
	return b.String()
}

func (e *EndBlock) WriteTo(b *strings.Builder) {
	b.WriteString("END {\n")
	writeTo(e.Body, b)
	b.WriteString("\n}")
}

// Keyword__FILE__ represents __FILE__ in the AST
type Keyword__FILE__ struct {
	Token    token.Token // the token.FILE__ token
	Filename string
}

func (f *Keyword__FILE__) String() string             { return f.Token.Type.Literal() }
func (f *Keyword__FILE__) WriteTo(b *strings.Builder) { b.WriteString(f.Token.Type.Literal()) }
func (f *Keyword__FILE__) expressionNode() {}
func (f *Keyword__FILE__) literalNode()    {}

// Pos returns the position of first character belonging to the node
func (f *Keyword__FILE__) Pos() int { return f.Token.Pos }

// End returns the position of first character immediately after the node
func (f *Keyword__FILE__) End() int { return f.Token.Pos + 8 }

// TokenLiteral returns the literal of the token.FILE__ token
func (f *Keyword__FILE__) TokenLiteral() string { return f.Token.Type.Literal() }

// Keyword__LINE__ represents __LINE__ in the AST.
// Pointer-free so arena chunks are noscan-eligible.
type Keyword__LINE__ struct {
	PosOff int
}

func (l *Keyword__LINE__) String() string             { return "__LINE__" }
func (l *Keyword__LINE__) WriteTo(b *strings.Builder) { b.WriteString("__LINE__") }
func (l *Keyword__LINE__) expressionNode()      {}
func (l *Keyword__LINE__) literalNode()         {}
func (l *Keyword__LINE__) Pos() int             { return l.PosOff }
func (l *Keyword__LINE__) End() int             { return l.PosOff + 8 }
func (l *Keyword__LINE__) TokenLiteral() string { return "__LINE__" }

// Keyword__DIR__ represents __dir__ in the AST.
// Pointer-free so arena chunks are noscan-eligible.
type Keyword__DIR__ struct {
	PosOff int
}

func (d *Keyword__DIR__) String() string             { return "__dir__" }
func (d *Keyword__DIR__) WriteTo(b *strings.Builder) { b.WriteString("__dir__") }
func (d *Keyword__DIR__) expressionNode()      {}
func (d *Keyword__DIR__) literalNode()         {}
func (d *Keyword__DIR__) Pos() int             { return d.PosOff }
func (d *Keyword__DIR__) End() int             { return d.PosOff + 7 }
func (d *Keyword__DIR__) TokenLiteral() string { return "__dir__" }

// Keyword__CALLEE__ represents __callee__ in the AST.
// Pointer-free so arena chunks are noscan-eligible.
type Keyword__CALLEE__ struct {
	PosOff int
}

func (c *Keyword__CALLEE__) String() string             { return "__callee__" }
func (c *Keyword__CALLEE__) WriteTo(b *strings.Builder) { b.WriteString("__callee__") }
func (c *Keyword__CALLEE__) expressionNode()      {}
func (c *Keyword__CALLEE__) literalNode()         {}
func (c *Keyword__CALLEE__) Pos() int             { return c.PosOff }
func (c *Keyword__CALLEE__) End() int             { return c.PosOff + 10 }
func (c *Keyword__CALLEE__) TokenLiteral() string { return "__callee__" }

// Keyword__METHOD__ represents __method__ in the AST.
// Pointer-free so arena chunks are noscan-eligible.
type Keyword__METHOD__ struct {
	PosOff int
}

func (m *Keyword__METHOD__) String() string             { return "__method__" }
func (m *Keyword__METHOD__) WriteTo(b *strings.Builder) { b.WriteString("__method__") }
func (m *Keyword__METHOD__) expressionNode()      {}
func (m *Keyword__METHOD__) literalNode()         {}
func (m *Keyword__METHOD__) Pos() int             { return m.PosOff }
func (m *Keyword__METHOD__) End() int             { return m.PosOff + 10 }
func (m *Keyword__METHOD__) TokenLiteral() string { return "__method__" }

// Keyword__ENCODING__ represents __ENCODING__ in the AST.
// Pointer-free so arena chunks are noscan-eligible.
type Keyword__ENCODING__ struct {
	PosOff int
}

func (e *Keyword__ENCODING__) String() string             { return "__ENCODING__" }
func (e *Keyword__ENCODING__) WriteTo(b *strings.Builder) { b.WriteString("__ENCODING__") }
func (e *Keyword__ENCODING__) expressionNode()      {}
func (e *Keyword__ENCODING__) literalNode()         {}
func (e *Keyword__ENCODING__) Pos() int             { return e.PosOff }
func (e *Keyword__ENCODING__) End() int             { return e.PosOff + 12 }
func (e *Keyword__ENCODING__) TokenLiteral() string { return "__ENCODING__" }

// UsingExpression represents a `using Module` statement
type UsingExpression struct {
	Token token.Token // the using keyword
	Expr  Expression  // the module/refinement
}

func (u *UsingExpression) expressionNode()      {}
func (u *UsingExpression) Pos() int             { return u.Token.Pos }
func (u *UsingExpression) End() int             { return u.Expr.End() }
func (u *UsingExpression) TokenLiteral() string { return u.Token.Type.Literal() }
func (u *UsingExpression) String() string {
	var b strings.Builder
	u.WriteTo(&b)
	return b.String()
}

func (u *UsingExpression) WriteTo(b *strings.Builder) {
	b.WriteString("using ")
	writeTo(u.Expr, b)
}

// RefineExpression represents a `refine Class do ... end` block
type RefineExpression struct {
	Token  token.Token // the refine keyword
	EndPos int         // pos of the closing `end`
	Expr   Expression  // the target class
	Body   *BlockStatement
}

func (r *RefineExpression) expressionNode()      {}
func (r *RefineExpression) Pos() int             { return r.Token.Pos }
func (r *RefineExpression) End() int             { return r.EndPos }
func (r *RefineExpression) TokenLiteral() string { return r.Token.Type.Literal() }
func (r *RefineExpression) String() string {
	var b strings.Builder
	r.WriteTo(&b)
	return b.String()
}

func (r *RefineExpression) WriteTo(b *strings.Builder) {
	b.WriteString("refine ")
	writeTo(r.Expr, b)
	if r.Body == nil {
		return
	}
	b.WriteString(" do\n")
	writeTo(r.Body, b)
	b.WriteString("\nend")
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

func (i *Identifier) WriteTo(b *strings.Builder) {
	if i == nil {
		return
	}
	b.WriteString(i.Value)
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
func (i *Identifier) TokenLiteral() string { return i.Value }

// Global represents a global in the AST
type Global struct {
	Token token.Token // the token.GLOBAL token
	Value string
}

func (g *Global) String() string             { return g.Value }
func (g *Global) WriteTo(b *strings.Builder) { b.WriteString(g.Value) }
func (g *Global) expressionNode() {}

// Pos returns the position of first character belonging to the node
func (g *Global) Pos() int { return g.Token.Pos }

// End returns the position of first character immediately after the node
func (g *Global) End() int     { return g.Token.Pos + len(g.Value) }
func (g *Global) literalNode() {}

// TokenLiteral returns the literal of the token.GLOBAL token
func (g *Global) TokenLiteral() string { return g.Value }

// ScopedIdentifier represents a scoped Constant declaration
type ScopedIdentifier struct {
	Token token.Token // the token.SCOPE
	// Outer is the LHS of the `::` operator. Usually an *Identifier
	// (`Foo::Bar`) or a nested *ScopedIdentifier (`Foo::Bar::Baz`), but
	// any expression that resolves to a Class / Module value at runtime
	// is permitted -- e.g. `self.class::OPERATORS`, where Outer is a
	// *ContextCallExpression.
	Outer Expression
	Inner Expression
}

func (i *ScopedIdentifier) String() string {
	var b strings.Builder
	i.WriteTo(&b)
	return b.String()
}

func (i *ScopedIdentifier) WriteTo(b *strings.Builder) {
	if i.Outer != nil {
		writeTo(i.Outer, b)
	}
	b.WriteString(i.Token.Type.Literal())
	if i.Inner != nil {
		writeTo(i.Inner, b)
	}
}
func (i *ScopedIdentifier) expressionNode() {}
func (i *ScopedIdentifier) literalNode()    {}

// Pos returns the position of first character belonging to the node
func (i *ScopedIdentifier) Pos() int {
	if i.Outer != nil {
		return i.Outer.Pos()
	}
	return i.Token.Pos
}

// End returns the position of first character immediately after the node
func (i *ScopedIdentifier) End() int {
	if i.Inner != nil {
		return i.Inner.End()
	}
	return i.Token.Pos + len(i.Token.Type.Literal())
}

// TokenLiteral returns the literal of the token.SCOPE token
func (i *ScopedIdentifier) TokenLiteral() string { return i.Token.Type.Literal() }

// IntegerLiteral represents an integer in the AST. Base preserves the
// source-form prefix (2 / 8 / 16) so MRI re-reads the same IntegerBaseFlags
// on roundtrip; 0 / 10 both mean plain decimal. Digit-group underscores
// and prefix-case (`0X`, `0d`, etc) are dropped -- cosmetic, MRI collapses
// on parse and parsetreenorm matches across forms. HadWhitespace tracks
// the source-side leading-space bit needed for unary `f -1` vs `f-1`
// disambiguation on re-emit (semantic: spaced form is a call with a
// negative arg, tight form is subtraction). Rational / Imaginary flags
// preserve the `r` / `i` suffix so MRI re-classifies the literal as
// RationalNode / ImaginaryNode on roundtrip (without these, `1r` would
// re-parse as a plain `1` IntegerNode -- different tree).
type IntegerLiteral struct {
	PosOff        int
	Value         int64
	BigInt        *big.Int // set for values that overflow int64
	Base          uint8    // 2, 8, 16; 0 or 10 = decimal (no prefix on re-emit)
	HadWhitespace bool
	Rational      bool
	Imaginary     bool
}

func (il *IntegerLiteral) expressionNode() {}
func (il *IntegerLiteral) literalNode()    {}

// Pos returns the position of first character belonging to the node
func (il *IntegerLiteral) Pos() int { return il.PosOff }

// End returns the position of first character immediately after the node
func (il *IntegerLiteral) End() int { return il.PosOff + len(il.String()) }

// TokenLiteral returns the literal form (same as String).
func (il *IntegerLiteral) TokenLiteral() string { return il.String() }
func (il *IntegerLiteral) String() string {
	var b strings.Builder
	il.WriteTo(&b)
	return b.String()
}

func (il *IntegerLiteral) WriteTo(b *strings.Builder) {
	base := int(il.Base)
	if base == 0 {
		base = 10
	}
	switch base {
	case 2:
		b.WriteString("0b")
	case 8:
		b.WriteString("0o")
	case 16:
		b.WriteString("0x")
	}
	if il.BigInt != nil {
		b.WriteString(il.BigInt.Text(base))
	} else {
		b.WriteString(strconv.FormatInt(il.Value, base))
	}
	if il.Rational {
		b.WriteByte('r')
	}
	if il.Imaginary {
		b.WriteByte('i')
	}
}

// FloatLiteral represents a floating-point number in the AST. Underscore
// grouping and exponent case (`E` vs `e`) from the source are dropped --
// both cosmetic, MRI collapses on parse. HadWhitespace tracks the
// source-side leading-space bit needed for unary `f -1.0` vs `f-1.0`
// disambiguation (same semantic split as IntegerLiteral). Rational /
// Imaginary preserve the `r` / `i` suffix so MRI re-classifies as
// RationalNode / ImaginaryNode on roundtrip.
type FloatLiteral struct {
	PosOff        int
	Value         float64
	HadWhitespace bool
	Rational      bool
	Imaginary     bool
}

func (fl *FloatLiteral) expressionNode() {}
func (fl *FloatLiteral) literalNode()    {}

// Pos returns the position of first character belonging to the node
func (fl *FloatLiteral) Pos() int { return fl.PosOff }

// End returns the position of first character immediately after the node
func (fl *FloatLiteral) End() int { return fl.PosOff + len(fl.String()) }

// TokenLiteral returns the literal form (same as String).
func (fl *FloatLiteral) TokenLiteral() string { return fl.String() }
func (fl *FloatLiteral) String() string {
	var b strings.Builder
	fl.WriteTo(&b)
	return b.String()
}

func (fl *FloatLiteral) WriteTo(b *strings.Builder) {
	verb := byte('g')
	if fl.Rational || fl.Imaginary {
		verb = 'f'
	}
	s := strconv.FormatFloat(fl.Value, verb, -1, 64)
	b.WriteString(s)
	if !strings.ContainsAny(s, ".eE") {
		b.WriteString(".0")
	}
	if fl.Rational {
		b.WriteByte('r')
	}
	if fl.Imaginary {
		b.WriteByte('i')
	}
}

// Nil represents the 'nil' keyword.
// Pointer-free (just a Pos) so arena chunks of Nil are noscan-eligible.
type Nil struct {
	PosOff int
}

func (n *Nil) expressionNode() {}
func (n *Nil) literalNode()    {}

// Pos returns the position of first character belonging to the node
func (n *Nil) Pos() int { return n.PosOff }

// End returns the position of first character immediately after the node
func (n *Nil) End() int { return n.PosOff + 3 }

// TokenLiteral returns the literal from the token token.NIL
func (n *Nil) TokenLiteral() string         { return "nil" }
func (n *Nil) String() string               { return "nil" }
func (n *Nil) WriteTo(b *strings.Builder)   { b.WriteString("nil") }

// Boolean represents a boolean in the AST.
// Pointer-free (Pos + bool) so arena chunks of Boolean are noscan-eligible.
type Boolean struct {
	PosOff int
	Value  bool
}

func (b *Boolean) expressionNode() {}
func (b *Boolean) literalNode()    {}

// Pos returns the position of first character belonging to the node
func (b *Boolean) Pos() int { return b.PosOff }

// End returns the position of first character immediately after the node
func (b *Boolean) End() int {
	if b.Value {
		return b.PosOff + 4 // "true"
	}
	return b.PosOff + 5 // "false"
}

// TokenLiteral returns the literal "true" or "false" depending on Value.
func (b *Boolean) TokenLiteral() string {
	if b.Value {
		return "true"
	}
	return "false"
}
func (b *Boolean) String() string {
	if b.Value {
		return "true"
	}
	return "false"
}

func (b *Boolean) WriteTo(sb *strings.Builder) {
	if b.Value {
		sb.WriteString("true")
	} else {
		sb.WriteString("false")
	}
}

// StringLiteral represents a string in the AST. For non-interpolated strings,
// Value holds the content and Parts is nil. For interpolated strings, Parts
// holds StringContent and expression nodes.
type StringLiteral struct {
	Token            token.Token      // STRING_BEG or STRING
	Value            string           // for non-interpolated strings
	Parts            []Expression     // for interpolated strings
	HeredocTagSource string           // source-form heredoc tag (e.g. "<<EOS", "<<~'DOC'") set at parse time for heredoc-origin literals; "" for non-heredocs
	HeredocStripped  bool             // true on `<<~` heredocs whose source had a positive common indent that was stripped -- preserves MRI's NODE_DSTR (vs NODE_STR) classification on roundtrip
	Adjacent         []*StringLiteral // adjacent string literals: `"a" "b"` -- MRI parses each separately and wraps in an outer InterpolatedString
}

// HeredocTag returns the heredoc tag (e.g. "<<EOS", "<<~'DOC'") for heredoc-
// origin literals, or "" for non-heredocs. Populated at parse time from the
// STRING_BEG / XSTR_BEG token literal; carries the tag info that used to
// live on Token.Literal so the AST stays self-contained.
func (sl *StringLiteral) HeredocTag() string {
	switch sl.Token.Type {
	case token.STRING_BEG, token.XSTR_BEG:
		return sl.HeredocTagSource
	}
	return ""
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
func (sl *StringLiteral) TokenLiteral() string { return sl.Value }
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
	var b strings.Builder
	sl.WriteTo(&b)
	return b.String()
}

// WriteTo on StringLiteral keeps the existing stringOnce / adjacent
// pipeline -- the body has heredoc-aware buffer juggling that would be
// invasive to refactor; this layer just routes the final string into the
// shared builder. Same allocation profile as the prior String, but lets
// upstream callers (InfixExpression, ContextCallExpression) avoid the
// intermediate they used to do via child.String().
func (sl *StringLiteral) WriteTo(b *strings.Builder) {
	b.WriteString(sl.stringOnce())
	for _, a := range sl.Adjacent {
		b.WriteByte(' ')
		writeTo(a, b)
	}
}

func (sl *StringLiteral) stringOnce() string {
	if sl.HeredocTag() != "" {
		delim := heredocDelimFromTag(sl.HeredocTag())
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
		style := heredocStyle(sl.HeredocTag())
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
		out.WriteString(sl.HeredocTag())
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
				if pe, ok := p.(*ParenExpression); ok {
					if pe.Expr == nil && len(pe.Stmts) == 0 {
						out.WriteString("#{}")
						break
					}
					if len(pe.Stmts) > 0 {
						// Multi-statement interp `#{a; b; c}` -- emit the
						// statements bare inside `#{...}` (no outer parens)
						// so MRI re-parses as EmbeddedStatementsNode with
						// multiple body entries, not a wrapping
						// ParenthesesNode.
						parts := make([]string, len(pe.Stmts))
						for i, s := range pe.Stmts {
							parts[i] = s.String()
						}
						out.WriteString("#{")
						out.WriteString(strings.Join(parts, "; "))
						out.WriteString("}")
						break
					}
				}
				out.WriteString("#{")
				out.WriteString(p.String())
				out.WriteString("}")
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
func (sc *StringContent) TokenLiteral() string         { return sc.Value }
func (sc *StringContent) String() string               { return sc.Value }
func (sc *StringContent) WriteTo(b *strings.Builder)   { b.WriteString(sc.Value) }

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
func (ev *EmbeddedVariable) String() string {
	var b strings.Builder
	ev.WriteTo(&b)
	return b.String()
}

func (ev *EmbeddedVariable) WriteTo(b *strings.Builder) {
	b.WriteByte('#')
	writeTo(ev.Variable, b)
}

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
func (rl *RegexLiteral) TokenLiteral() string { return rl.Value }

// WriteTo is a thin delegation to String for RegexLiteral. The String body
// has delimiter-pick + interpolation handling that's tedious to inline; the
// gain from sharing the parent's builder is marginal because regexes don't
// deeply recurse.
func (rl *RegexLiteral) WriteTo(b *strings.Builder) { b.WriteString(rl.String()) }

func (rl *RegexLiteral) String() string {
	// RegexLiteral's body builds its own buffer with several layers of
	// delimiter / interpolation logic. Keeping the body intact and stubbing
	// WriteTo as a delegation; the perf gain from sharing the parent's
	// builder isn't material here -- regex bodies don't deeply recurse.
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
				if pe, ok := p.(*ParenExpression); ok {
					if pe.Expr == nil && len(pe.Stmts) == 0 {
						out.WriteString("#{}")
						break
					}
					if len(pe.Stmts) > 0 {
						parts := make([]string, len(pe.Stmts))
						for i, s := range pe.Stmts {
							parts[i] = s.String()
						}
						out.WriteString("#{")
						out.WriteString(strings.Join(parts, "; "))
						out.WriteString("}")
						break
					}
				}
				out.WriteString("#{")
				out.WriteString(p.String())
				out.WriteString("}")
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
func (c *Comment) TokenLiteral() string { return c.Value }
func (c *Comment) String() string { return "#" + c.Value }
func (c *Comment) WriteTo(b *strings.Builder) {
	b.WriteByte('#')
	b.WriteString(c.Value)
}

// SymbolLiteral represents a symbol within the AST
type SymbolLiteral struct {
	Token token.Token // the ':' (SYMBEG) or the 'name:' (LABEL)
	Value Expression
	// LabelText is the source-form text of a LABEL-typed symbol (e.g.
	// "foo:"). Populated at parse from Token.Literal so the printer can
	// re-emit the LABEL form without consulting the token's source text.
	// Empty for non-LABEL symbols. Carries the info that Token.Literal
	// held; structurally non-trivial to reconstruct from Value alone
	// because the parser stores Value as either StringLiteral{Value:"name"}
	// or Identifier{Value:"name:"} depending on the parse context.
	LabelText string
}

func (s *SymbolLiteral) expressionNode() {}
func (s *SymbolLiteral) literalNode()    {}

// Pos returns the position of first character belonging to the node
func (s *SymbolLiteral) Pos() int { return s.Token.Pos }

// End returns the position of first character immediately after the node
func (s *SymbolLiteral) End() int { return s.Value.End() }

// TokenLiteral returns the literal from token token.SYMBOL. For LABEL
// tokens, returns the source-form `name:`; for plain symbols, returns
// the `:` sigil.
func (s *SymbolLiteral) TokenLiteral() string {
	if s.Token.Type == token.LABEL {
		return s.LabelText
	}
	return s.Token.Type.Literal()
}
func (s *SymbolLiteral) String() string {
	var b strings.Builder
	s.WriteTo(&b)
	return b.String()
}

func (s *SymbolLiteral) WriteTo(b *strings.Builder) {
	b.WriteByte(':')
	if s.Value != nil {
		writeTo(s.Value, b)
	}
}

// LabelString returns the symbol in label form (without leading colon).
func (s *SymbolLiteral) LabelString() string { return s.Value.String() }

// ConditionalExpression represents an if expression within the AST
type ConditionalExpression struct {
	Token       token.Token // The 'if' or 'unless' token
	EndPos      int         // pos of the closing `end`; 0 for ternary / modifier (no closer in source)
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
	if ce.EndPos == 0 {
		return ce.Consequence.Pos()
	}
	return ce.Token.Pos
}

// End returns the position of first character immediately after the node
func (ce *ConditionalExpression) End() int {
	if ce.EndPos == 0 {
		return ce.Consequence.Pos()
	}
	return ce.EndPos
}

// TokenLiteral returns the literal from token token.IF or token.UNLESS
func (ce *ConditionalExpression) TokenLiteral() string { return ce.Token.Type.Literal() }
func (ce *ConditionalExpression) String() string {
	var b strings.Builder
	ce.WriteTo(&b)
	return b.String()
}

func (ce *ConditionalExpression) WriteTo(b *strings.Builder) {
	if ce.Token.Type == token.QMARK {
		writeTo(ce.Condition, b)
		b.WriteString(" ? ")
		writeTo(ce.Consequence, b)
		b.WriteString(" : ")
		if ce.Alternative != nil {
			writeTo(ce.Alternative, b)
		}
		return
	}
	if ce.EndPos == 0 && ce.Token.Type != token.KW_ELSIF && ce.Alternative == nil {
		writeTo(ce.Consequence, b)
		b.WriteByte(' ')
		b.WriteString(ce.Token.Type.Literal())
		b.WriteByte(' ')
		writeTo(ce.Condition, b)
		return
	}
	b.WriteString(ce.Token.Type.Literal())
	b.WriteByte(' ')
	writeTo(ce.Condition, b)
	b.WriteByte('\n')
	writeTo(ce.Consequence, b)
	b.WriteByte('\n')
	if ce.Alternative != nil {
		if nested := extractElsif(ce.Alternative); nested != nil {
			writeTo(nested, b)
			return
		}
		b.WriteString("else\n")
		writeTo(ce.Alternative, b)
		b.WriteByte('\n')
	}
	b.WriteString("end")
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
	EndPos    int         // pos of the closing `end`; 0 for modifier-form (no closer in source)
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
	return ce.EndPos
}

// TokenLiteral returns the literal from token token.WHILE
func (ce *LoopExpression) TokenLiteral() string { return ce.Token.Type.Literal() }
func (ce *LoopExpression) String() string {
	var b strings.Builder
	ce.WriteTo(&b)
	return b.String()
}

func (ce *LoopExpression) WriteTo(b *strings.Builder) {
	if ce.PostTest && ce.Block != nil && len(ce.Block.Statements) == 1 {
		if es, ok := ce.Block.Statements[0].(*ExpressionStatement); ok {
			if bb, ok := es.Expression.(*ExceptionHandlingBlock); ok {
				writeTo(bb, b)
				b.WriteByte(' ')
				b.WriteString(ce.Token.Type.Literal())
				b.WriteByte(' ')
				writeTo(ce.Condition, b)
				return
			}
		}
	}
	b.WriteString(ce.Token.Type.Literal())
	b.WriteByte(' ')
	writeTo(ce.Condition, b)
	if ce.Block != nil {
		b.WriteByte('\n')
		writeTo(ce.Block, b)
		b.WriteByte('\n')
	}
	b.WriteString("end")
}

// ImplicitRest is a sentinel for the trailing comma on multi-assign LHS
// (`a, b, = X`). MRI represents this as ImplicitRestNode in the parsetree;
// we mark it in the ExpressionList tail so the printer keeps the trailing
// comma on re-emit. Pointer-free so arena chunks are noscan-eligible.
type ImplicitRest struct {
	PosOff int
}

func (i *ImplicitRest) expressionNode()      {}
func (i *ImplicitRest) literalNode()         {}
func (i *ImplicitRest) Pos() int             { return i.PosOff }
func (i *ImplicitRest) End() int             { return i.PosOff }
func (i *ImplicitRest) TokenLiteral() string { return "" }
func (i *ImplicitRest) String() string             { return "" }
func (i *ImplicitRest) WriteTo(b *strings.Builder) {}

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
	var b strings.Builder
	el.WriteTo(&b)
	return b.String()
}

func (el ExpressionList) WriteTo(b *strings.Builder) {
	trailingRest := false
	count := 0
	for _, e := range el {
		if _, ok := e.(*ImplicitRest); ok {
			trailingRest = true
			continue
		}
		if count > 0 {
			b.WriteString(", ")
		}
		writeTo(e, b)
		count++
	}
	if trailingRest || count == 1 {
		b.WriteByte(',')
	}
}

// ArrayLiteral represents an Array literal within the AST
type ArrayLiteral struct {
	Token    token.Token // the '['
	EndPos   int         // pos of the closing `]`
	Elements []Expression
	// Multiline marks a %w/%W/%i/%I array whose source body spanned more
	// than one line. Prism's encoding-inference for the surrounding
	// program (not just elements inside the array) depends on whether a
	// `\u` escape sits alone on its line versus shares one with other
	// content: a single-line layout propagates `forced_utf8_encoding`
	// onto subsequent symbols/strings outside the array body, while a
	// multi-line layout contains the propagation to the line of the
	// escape. Layout has to be preserved on re-emit -- multi-line stays
	// multi-line, single-line stays so -- or downstream SymbolFlags /
	// StringFlags diverge.
	Multiline bool
	// PercentChar marks the percent-array type for `%w`/`%W`/`%i`/`%I`
	// arrays (matching value 'w'/'W'/'i'/'I'). Zero for plain bracketed
	// arrays. Carries the info that Token.Literal used to hold, so the
	// printer can re-emit the percent-array form without consulting the
	// Token's source text.
	PercentChar byte
}

func (al *ArrayLiteral) expressionNode() {}
func (al *ArrayLiteral) literalNode()    {}

// Pos returns the position of first character belonging to the node
func (al *ArrayLiteral) Pos() int { return al.Token.Pos }

// End returns the position of first character immediately after the node
func (al *ArrayLiteral) End() int {
	return al.EndPos
}

// TokenLiteral returns the literal of the token token.LBRACKET, or the
// percent-array type char for `%w`/`%W`/`%i`/`%I` arrays.
func (al *ArrayLiteral) TokenLiteral() string {
	if al.PercentChar != 0 {
		return string(al.PercentChar)
	}
	return al.Token.Type.Literal()
}
func (al *ArrayLiteral) String() string {
	var b strings.Builder
	al.WriteTo(&b)
	return b.String()
}

func (al *ArrayLiteral) WriteTo(b *strings.Builder) {
	if al.Token.Type == token.STRING_BEG {
		if s, ok := al.percentArrayString(); ok {
			b.WriteString(s)
			return
		}
	}
	b.WriteByte('[')
	for i, el := range al.Elements {
		if i > 0 {
			b.WriteString(", ")
		}
		writeTo(el, b)
	}
	b.WriteByte(']')
}

// percentArrayString re-emits a `%w` / `%W` / `%i` / `%I` array. Returns
// false if the array element shape doesn't fit the simple word-list form
// (e.g. an interpolated symbol with embedded spaces) -- the caller falls
// back to `[...]` form.
func (al *ArrayLiteral) percentArrayString() (string, bool) {
	switch al.PercentChar {
	case 'w', 'W', 'i', 'I':
	default:
		return "", false
	}
	typ := string(al.PercentChar)
	// Lowercase %w / %i resolve `\<delim>` -> `<delim>` and `\\` -> `\` in
	// content, so a Value containing a backslash can't be safely re-emitted
	// without knowing the original delim. Uppercase %W / %I store source
	// bytes verbatim (escapes are preserved raw), so backslash content is
	// fine to re-emit.
	allowBackslash := al.PercentChar == 'W' || al.PercentChar == 'I'
	badChars := " \t\n[]"
	if !allowBackslash {
		badChars += "\\"
	}
	words := make([]string, 0, len(al.Elements))
	for _, el := range al.Elements {
		var w string
		switch e := el.(type) {
		case *StringLiteral:
			if e.Parts != nil || strings.ContainsAny(e.Value, badChars) {
				return "", false
			}
			w = e.Value
		case *SymbolLiteral:
			switch v := e.Value.(type) {
			case *Identifier:
				w = v.Value
			case *StringLiteral:
				if v.Parts != nil || strings.ContainsAny(v.Value, badChars) {
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
	if al.Multiline {
		return "%" + typ + "[\n" + strings.Join(words, "\n") + "\n]", true
	}
	return "%" + typ + "[" + strings.Join(words, " ") + "]", true
}

// HashLiteral represents an Hash literal within the AST
type HashLiteral struct {
	Token    token.Token // the '{'
	EndPos   int         // pos of the closing `}`
	Map      OrderedExprMap
	Splats   []Expression // **expr keyword-splat entries
	Implicit bool         // true for implicit hash arg (no braces in source)
}

func (hl *HashLiteral) expressionNode() {}
func (hl *HashLiteral) literalNode()    {}

// Pos returns the position of the left brace
func (hl *HashLiteral) Pos() int { return hl.Token.Pos }

// End returns the position of the right brace
func (hl *HashLiteral) End() int { return hl.EndPos }

// TokenLiteral returns the literal of the token token.LBRACE
func (hl *HashLiteral) TokenLiteral() string { return hl.Token.Type.Literal() }
func (hl *HashLiteral) hashElements() []string {
	type posStr struct {
		pos int
		s   string
	}
	items := []posStr{}
	if hl.Map.Len() > 0 {
		for _, kv := range hl.Map.Entries() {
			var s string
			if kv.Value != nil {
				if sym, ok := kv.Key.(*SymbolLiteral); ok && sym.Token.Type == token.LABEL {
					if kv.Omitted {
						s = sym.LabelText
					} else {
						s = sym.LabelText + " " + kv.Value.String()
					}
				} else if sym, ok := kv.Key.(*SymbolLiteral); ok {
					if slv, isStr := sym.Value.(*StringLiteral); isStr {
						// String-label key: source was `"a": val`. Preserve label
						// form -- emitting `:"a" => val` would re-parse with a
						// different shape (and is rejected in pattern contexts).
						if kv.Omitted {
							s = slv.String() + ":"
						} else {
							s = slv.String() + ": " + kv.Value.String()
						}
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
	var b strings.Builder
	hl.WriteTo(&b)
	return b.String()
}

func (hl *HashLiteral) WriteTo(b *strings.Builder) {
	if !hl.Implicit {
		b.WriteByte('{')
	}
	for i, e := range hl.hashElements() {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(e)
	}
	if !hl.Implicit {
		b.WriteByte('}')
	}
}

// StringNoBraces returns the hash content without surrounding braces,
// for use in pattern matching in-clauses where implicit hash is needed.
// Uses label syntax (key: val) instead of hashrocket (key => val) for
// symbol keys so the output re-parses as an implicit hash pattern.
func (hl *HashLiteral) StringNoBraces() string {
	elements := []string{}
	if hl.Map.Len() > 0 {
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
				} else if sl, ok := kv.Key.(*StringLiteral); ok && sl.HeredocTag() == "" {
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
	var sb strings.Builder
	b.WriteTo(&sb)
	return sb.String()
}

func (b *BlockCapture) WriteTo(sb *strings.Builder) {
	sb.WriteByte('&')
	if b.Expr != nil {
		writeTo(b.Expr, sb)
		return
	}
	if b.Name != nil {
		sb.WriteString(b.Name.Value)
	}
}

// TokenLiteral returns the literal of the token
func (b *BlockCapture) TokenLiteral() string { return b.Token.Type.Literal() }

// A FunctionLiteral represents a function definition in the AST
type FunctionLiteral struct {
	Token         token.Token // The 'def' or '->' token
	EndPos        int         // pos of the closing `end` / `}` / endless-body last token; 0 if unterminated
	Receiver      *Identifier
	Name          *Identifier
	Parameters    []*FunctionParameter
	CapturedBlock *BlockCapture
	Body          *BlockStatement
	Rescues       []*RescueBlock
	ElseBody      *BlockStatement
	EnsureBody    *BlockStatement
	IsLambda      bool // true for -> lambda literals
	IsEndless     bool // true for endless methods: `def foo = expr` (ruby 3.0+)
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
	if fl.EndPos == 0 {
		if fl.Body != nil && len(fl.Body.Statements) > 0 {
			return fl.Body.End()
		}
		return fl.Token.Pos + 3
	}
	return fl.EndPos
}

// TokenLiteral returns the literal from token.DEF
func (fl *FunctionLiteral) TokenLiteral() string { return fl.Token.Type.Literal() }
func (fl *FunctionLiteral) String() string {
	var b strings.Builder
	fl.WriteTo(&b)
	return b.String()
}

func (fl *FunctionLiteral) WriteTo(b *strings.Builder) {
	writeParams := func() {
		paramCount := len(fl.Parameters)
		hasCap := fl.CapturedBlock != nil
		if paramCount == 0 && !hasCap {
			return
		}
		for i, p := range fl.Parameters {
			if i > 0 {
				b.WriteString(", ")
			}
			writeTo(p, b)
		}
		if hasCap {
			if paramCount > 0 {
				b.WriteString(", ")
			}
			writeTo(fl.CapturedBlock, b)
		}
	}
	hasAnyParam := len(fl.Parameters) > 0 || fl.CapturedBlock != nil
	if fl.IsLambda {
		b.WriteString("->")
	} else {
		b.WriteString("def ")
		if fl.Receiver != nil {
			needsParens := fl.Receiver.Token.Type == token.RPAREN
			if needsParens {
				b.WriteByte('(')
			}
			writeTo(fl.Receiver, b)
			if needsParens {
				b.WriteByte(')')
			}
			b.WriteByte('.')
		}
		writeTo(fl.Name, b)
	}
	if !fl.IsLambda && fl.IsEndless {
		if hasAnyParam || fl.ExplicitParens {
			b.WriteByte('(')
			writeParams()
			b.WriteByte(')')
		}
		b.WriteString(" = ")
		if fl.Body != nil {
			writeTo(fl.Body, b)
		}
		return
	}
	if !fl.IsLambda || hasAnyParam || fl.ExplicitParens {
		b.WriteByte('(')
		writeParams()
		b.WriteByte(')')
	}
	if fl.IsLambda {
		b.WriteString(" {")
	}
	if fl.Body != nil && len(fl.Body.Statements) > 0 {
		b.WriteByte('\n')
		writeTo(fl.Body, b)
	}
	b.WriteByte('\n')
	for _, r := range fl.Rescues {
		writeTo(r, b)
	}
	if fl.ElseBody != nil {
		b.WriteString("else\n")
		writeTo(fl.ElseBody, b)
		b.WriteByte('\n')
	}
	if fl.EnsureBody != nil {
		b.WriteString("ensure\n")
		writeTo(fl.EnsureBody, b)
		b.WriteByte('\n')
	}
	if fl.IsLambda {
		b.WriteByte('}')
	} else {
		b.WriteString("end")
	}
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
	var b strings.Builder
	f.WriteTo(&b)
	return b.String()
}

func (f *FunctionParameter) WriteTo(b *strings.Builder) {
	if f.IsImplicitRest {
		return
	}
	if f.IsForwarding {
		b.WriteString("...")
		return
	}
	if f.IsSplat {
		b.WriteByte('*')
	}
	if f.IsNoKeywords {
		b.WriteString("**nil")
		return
	}
	if f.IsKeywordRest {
		b.WriteString("**")
	}
	if f.Name != nil && !(f.IsKeywordRest && f.Name.Value == "**") && !(f.IsSplat && f.Name.Value == "*") {
		writeTo(f.Name, b)
	}
	if f.IsKeyword {
		b.WriteString(": ")
		if f.Default != nil {
			b.WriteString(encloseInParensIfNeeded(f.Default))
		}
	} else if f.Default != nil {
		b.WriteString(" = ")
		b.WriteString(encloseInParensIfNeeded(f.Default))
	}
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
func (ie *IndexExpression) TokenLiteral() string { return ie.Token.Type.Literal() }
func (ie *IndexExpression) String() string {
	var b strings.Builder
	ie.WriteTo(&b)
	return b.String()
}

func (ie *IndexExpression) WriteTo(b *strings.Builder) {
	writeTo(ie.Left, b)
	b.WriteByte('[')
	for i, a := range ie.Arguments {
		if i > 0 {
			b.WriteString(", ")
		}
		writeTo(a, b)
	}
	b.WriteByte(']')
}

// A ContextCallExpression represents a method call on a given Context.
//
// PERF: full token.Token here was 32B and only Type was ever actually read
// (the .  / :: / &. dispatch in String). Replaced with OpType + packed bool
// to shrink the node from 96B to 64B; saves ~32B x ~6M nodes on the
// real-files bench. See PERF_IDEAS.md.
type ContextCallExpression struct {
	OpType         token.Type       // Type of the call operator (DOT, SCOPE, LONELY) or the function IDENT for paren-less / paramless calls
	ExplicitParens bool             // true when source had explicit ( ) -- preserves obj.foo() vs obj.foo and foo() vs foo (vcall)
	Context        Expression       // The lefthandside expression
	Function       *Identifier      // The function to call
	Block          *BlockExpression // The function block
	Arguments      []Expression     // The function arguments
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

// TokenLiteral returns the operator literal (".", "::", "&.") derived from
// OpType. Falls back to the function identifier's literal for paren-less /
// paramless calls whose OpType is an IDENT.
func (ce *ContextCallExpression) TokenLiteral() string {
	switch ce.OpType {
	case token.DOT:
		return "."
	case token.SCOPE:
		return "::"
	case token.LONELY:
		return "&."
	}
	if ce.Function != nil {
		return ce.Function.TokenLiteral()
	}
	return ""
}
func (ce *ContextCallExpression) String() string {
	var b strings.Builder
	ce.WriteTo(&b)
	return b.String()
}

func (ce *ContextCallExpression) WriteTo(b *strings.Builder) {
	if ce.Context != nil {
		writeTo(ce.Context, b)
		switch ce.OpType {
		case token.LONELY:
			b.WriteString("&.")
		case token.SCOPE:
			b.WriteString("::")
		default:
			b.WriteByte('.')
		}
	}
	if ce.Function != nil {
		name := ce.Function.Value
		if isSetterName(name) && len(ce.Arguments) == 1 {
			b.WriteString(strings.TrimSuffix(name, "="))
			b.WriteString(" = ")
			writeTo(ce.Arguments[0], b)
			return
		}
		writeTo(ce.Function, b)
	}
	writeArgs := func(open, close string) {
		if open != "" {
			b.WriteString(open)
		}
		first := true
		for _, a := range ce.Arguments {
			if a == nil {
				continue
			}
			if !first {
				b.WriteString(", ")
			}
			writeTo(a, b)
			first = false
		}
		if close != "" {
			b.WriteString(close)
		}
	}
	if ce.ExplicitParens {
		writeArgs("(", ")")
	} else if hasArg(ce.Arguments) {
		if containsHeredocArg(ce.Arguments) {
			b.WriteByte(' ')
			writeArgs("", "")
		} else {
			writeArgs("(", ")")
		}
	}
	if ce.Block != nil {
		if ce.Block.Token.Type == token.LBRACE {
			b.WriteByte(' ')
		}
		writeTo(ce.Block, b)
	}
}

func hasArg(args []Expression) bool {
	for _, a := range args {
		if a != nil {
			return true
		}
	}
	return false
}

// bareSplatPattern reports whether p is a top-level bare splat
// (`*` or `**` with no operand) used as an `in` pattern. MRI rejects
// such patterns without an explicit `then` delimiter.
func bareSplatPattern(p Expression) bool {
	pe, ok := p.(*PrefixExpression)
	if !ok {
		return false
	}
	if pe.Operator != "*" && pe.Operator != "**" {
		return false
	}
	return pe.Right == nil
}

func containsHeredocArg(args []Expression) bool {
	for _, a := range args {
		if sl, ok := a.(*StringLiteral); ok && sl.HeredocTag() != "" {
			return true
		}
	}
	return false
}

// A BlockExpression represents a Ruby block
type BlockExpression struct {
	Token         token.Token          // token.DO or token.LBRACE
	EndPos        int                  // pos of the closing `end` / `}`
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
func (b *BlockExpression) End() int { return b.EndPos }

// TokenLiteral returns the literal from the Token
func (b *BlockExpression) TokenLiteral() string { return b.Token.Type.Literal() }

// String returns a string representation of the block statement
func (b *BlockExpression) String() string {
	var sb strings.Builder
	b.WriteTo(&sb)
	return sb.String()
}

func (b *BlockExpression) WriteTo(sb *strings.Builder) {
	if b.Token.Type == token.LBRACE {
		sb.WriteByte('{')
	} else {
		sb.WriteString(" do")
	}
	if b.HasParameterBars || len(b.Parameters) != 0 || len(b.BlockLocals) != 0 || b.CapturedBlock != nil {
		sb.WriteString(" |")
		first := true
		for _, a := range b.Parameters {
			if !first {
				sb.WriteString(", ")
			}
			writeTo(a, sb)
			first = false
		}
		if b.CapturedBlock != nil {
			if !first {
				sb.WriteString(", ")
			}
			writeTo(b.CapturedBlock, sb)
		}
		if len(b.BlockLocals) != 0 {
			sb.WriteString("; ")
			for i, l := range b.BlockLocals {
				if i > 0 {
					sb.WriteString(", ")
				}
				writeTo(l, sb)
			}
		}
		sb.WriteByte('|')
	}
	sb.WriteByte('\n')
	writeTo(b.Body, sb)
	for _, r := range b.Rescues {
		sb.WriteByte('\n')
		writeTo(r, sb)
	}
	if b.ElseBody != nil {
		sb.WriteString("\nelse\n")
		writeTo(b.ElseBody, sb)
	}
	if b.EnsureBody != nil {
		sb.WriteString("\nensure\n")
		writeTo(b.EnsureBody, sb)
	}
	sb.WriteByte('\n')
	if b.Token.Type == token.LBRACE {
		sb.WriteByte('}')
	} else {
		sb.WriteString("end")
	}
}

// ModuleExpression represents a module definition
type ModuleExpression struct {
	Token   token.Token // The module keyword
	EndPos  int         // pos of the closing `end`
	Name    *Identifier // The module name, will always be a const
	Body    *BlockStatement
	Rescues []*RescueBlock
}

func (m *ModuleExpression) expressionNode() {}

// Pos returns the position of first character belonging to the node
func (m *ModuleExpression) Pos() int { return m.Token.Pos }

// End returns the position of the `end` token
func (m *ModuleExpression) End() int { return m.EndPos }

// TokenLiteral returns the literal from token.MODULE
func (m *ModuleExpression) TokenLiteral() string { return m.Token.Type.Literal() }
func (m *ModuleExpression) String() string {
	var b strings.Builder
	m.WriteTo(&b)
	return b.String()
}

func (m *ModuleExpression) WriteTo(b *strings.Builder) {
	b.WriteString(m.TokenLiteral())
	b.WriteByte(' ')
	writeTo(m.Name, b)
	b.WriteByte('\n')
	writeTo(m.Body, b)
	for _, r := range m.Rescues {
		b.WriteByte('\n')
		writeTo(r, b)
	}
	b.WriteString("\nend")
}

// ClassExpression represents a module definition
type ClassExpression struct {
	Token      token.Token // The class keyword
	EndPos     int         // pos of the closing `end`
	Name       *Identifier // The class name, will always be a const
	SuperClass Expression  // The superclass, if any
	Body       *BlockStatement
	Rescues    []*RescueBlock
}

func (m *ClassExpression) expressionNode() {}

// Pos returns the position of first character belonging to the node
func (m *ClassExpression) Pos() int { return m.Token.Pos }

// End returns the position of the `end` token
func (m *ClassExpression) End() int { return m.EndPos }

// TokenLiteral returns the literal from token.CLASS
func (m *ClassExpression) TokenLiteral() string { return m.Token.Type.Literal() }
func (m *ClassExpression) String() string {
	var b strings.Builder
	m.WriteTo(&b)
	return b.String()
}

func (m *ClassExpression) WriteTo(b *strings.Builder) {
	b.WriteString(m.TokenLiteral())
	b.WriteByte(' ')
	writeTo(m.Name, b)
	if m.SuperClass != nil {
		b.WriteString(" < ")
		writeTo(m.SuperClass, b)
	}
	b.WriteByte('\n')
	writeTo(m.Body, b)
	for _, r := range m.Rescues {
		b.WriteByte('\n')
		writeTo(r, b)
	}
	b.WriteString("\nend")
}

// SingletonClassExpression represents a singleton class definition: class << self; ...; end
type SingletonClassExpression struct {
	Token   token.Token // the 'class' token
	EndPos  int         // pos of the closing `end`
	Expr    Expression  // the expression after << (e.g. self)
	Body    *BlockStatement
	Rescues []*RescueBlock
}

func (s *SingletonClassExpression) expressionNode() {}

// Pos returns the position of first character belonging to the node
func (s *SingletonClassExpression) Pos() int { return s.Token.Pos }

// End returns the position of the 'end' token
func (s *SingletonClassExpression) End() int { return s.EndPos }

// TokenLiteral returns the literal from token.CLASS
func (s *SingletonClassExpression) TokenLiteral() string { return s.Token.Type.Literal() }
func (s *SingletonClassExpression) String() string {
	var b strings.Builder
	s.WriteTo(&b)
	return b.String()
}

func (s *SingletonClassExpression) WriteTo(b *strings.Builder) {
	b.WriteString(s.TokenLiteral())
	b.WriteString(" << ")
	writeTo(s.Expr, b)
	b.WriteByte('\n')
	writeTo(s.Body, b)
	for _, r := range s.Rescues {
		b.WriteByte('\n')
		writeTo(r, b)
	}
	b.WriteString("\nend")
}

// A SplatExpression represents a splat expression (*expr, **expr)
type SplatExpression struct {
	Token    token.Token // the * or ** token
	Operator string      // "*" or "**"
	Right    Expression
}

func (s *SplatExpression) String() string {
	var b strings.Builder
	s.WriteTo(&b)
	return b.String()
}

func (s *SplatExpression) WriteTo(b *strings.Builder) {
	b.WriteString(s.Operator)
	if s.Right != nil {
		writeTo(s.Right, b)
	}
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
func (s *SplatExpression) TokenLiteral() string { return s.Token.Type.Literal() }

// ArgumentForwarding represents `...` in a call argument context: foo(...)
type ArgumentForwarding struct {
	PosOff int // pos of the '...' token; pointer-free so arena chunks are noscan-eligible
}

func (af *ArgumentForwarding) expressionNode()      {}
func (af *ArgumentForwarding) Pos() int             { return af.PosOff }
func (af *ArgumentForwarding) End() int             { return af.PosOff + 3 }
func (af *ArgumentForwarding) TokenLiteral() string { return "..." }
func (af *ArgumentForwarding) String() string               { return "..." }
func (af *ArgumentForwarding) WriteTo(b *strings.Builder)   { b.WriteString("...") }

// A CaseExpression represents a case/when or case/in expression
type CaseExpression struct {
	Token       token.Token // case
	EndPos      int         // pos of the closing `end`
	Condition   Expression  // optional, nil for case without expr
	WhenClauses []*WhenClause
	InClauses   []*WhenClause // pattern-matching in clauses (reuse WhenClause for now)
	ElseBody    *BlockStatement
}

func (c *CaseExpression) String() string {
	var b strings.Builder
	c.WriteTo(&b)
	return b.String()
}

func (c *CaseExpression) WriteTo(b *strings.Builder) {
	b.WriteString("case")
	if c.Condition != nil {
		b.WriteByte(' ')
		writeTo(c.Condition, b)
	}
	b.WriteByte('\n')
	for _, w := range c.WhenClauses {
		writeTo(w, b)
	}
	for _, in := range c.InClauses {
		writeTo(in, b)
	}
	if c.ElseBody != nil {
		b.WriteString("else\n")
		writeTo(c.ElseBody, b)
		b.WriteByte('\n')
	}
	b.WriteString("end")
}
func (c *CaseExpression) expressionNode() {}

func (c *CaseExpression) Pos() int             { return c.Token.Pos }
func (c *CaseExpression) End() int             { return c.EndPos }
func (c *CaseExpression) TokenLiteral() string { return c.Token.Type.Literal() }

// A WhenClause represents a single when branch in a case expression
type WhenClause struct {
	Token      token.Token // when
	Conditions []Expression
	Body       *BlockStatement
}

func (w *WhenClause) String() string {
	var b strings.Builder
	w.WriteTo(&b)
	return b.String()
}

func (w *WhenClause) WriteTo(b *strings.Builder) {
	keyword := "when"
	if lit := w.Token.Type.Literal(); lit != "" {
		keyword = lit
	}
	b.WriteString(keyword)
	b.WriteByte(' ')
	for i, cond := range w.Conditions {
		if i > 0 {
			b.WriteString(", ")
		}
		writeTo(cond, b)
	}
	if keyword == "in" && len(w.Conditions) == 1 && bareSplatPattern(w.Conditions[0]) {
		b.WriteString(" then")
	}
	b.WriteByte('\n')
	if w.Body != nil && len(w.Body.Statements) > 0 {
		writeTo(w.Body, b)
		b.WriteByte('\n')
	}
}
func (w *WhenClause) expressionNode() {}

func (w *WhenClause) Pos() int             { return w.Token.Pos }
func (w *WhenClause) End() int             { return w.Body.End() }
func (w *WhenClause) TokenLiteral() string { return w.Token.Type.Literal() }

// A DefinedExpression represents defined?(expr)
type DefinedExpression struct {
	Token token.Token
	Expr  Expression
}

func (d *DefinedExpression) String() string {
	var b strings.Builder
	d.WriteTo(&b)
	return b.String()
}

func (d *DefinedExpression) WriteTo(b *strings.Builder) {
	if _, isInfix := d.Expr.(*InfixExpression); isInfix {
		b.WriteString("defined? ")
		writeTo(d.Expr, b)
		return
	}
	b.WriteString("defined?(")
	writeTo(d.Expr, b)
	b.WriteByte(')')
}
func (d *DefinedExpression) expressionNode() {}

func (d *DefinedExpression) Pos() int             { return d.Token.Pos }
func (d *DefinedExpression) End() int             { return d.Expr.End() }
func (d *DefinedExpression) TokenLiteral() string { return d.Token.Type.Literal() }

// A JumpExpression represents break, next, redo, or retry with an optional value
type JumpExpression struct {
	Token token.Token // break, next, redo, or retry
	Value Expression  // optional value (nil for bare break/next/redo/retry)
}

func (j *JumpExpression) String() string {
	var b strings.Builder
	j.WriteTo(&b)
	return b.String()
}

func (j *JumpExpression) WriteTo(b *strings.Builder) {
	b.WriteString(j.Token.Type.Literal())
	if j.Value == nil {
		return
	}
	b.WriteByte(' ')
	if al, ok := j.Value.(*ArrayLiteral); ok && al.Token.Type != token.LBRACKET {
		for i, e := range al.Elements {
			if i > 0 {
				b.WriteString(", ")
			}
			writeTo(e, b)
		}
		return
	}
	writeTo(j.Value, b)
}
func (j *JumpExpression) expressionNode() {}

// Pos returns the position of first character belonging to the node
func (j *JumpExpression) Pos() int { return j.Token.Pos }

// End returns the position of first character immediately after the node
func (j *JumpExpression) End() int {
	if j.Value != nil {
		return j.Value.End()
	}
	return j.Token.EndPos()
}

// TokenLiteral returns the literal from the keyword token
func (j *JumpExpression) TokenLiteral() string { return j.Token.Type.Literal() }

// AliasExpression represents an `alias new_name old_name` statement
type AliasExpression struct {
	Token   token.Token // the alias keyword
	NewName *Identifier
	OldName *Identifier
}

func (a *AliasExpression) String() string {
	var b strings.Builder
	a.WriteTo(&b)
	return b.String()
}

func (a *AliasExpression) WriteTo(b *strings.Builder) {
	b.WriteString("alias ")
	b.WriteString(a.NewName.Value)
	b.WriteByte(' ')
	b.WriteString(a.OldName.Value)
}
func (a *AliasExpression) expressionNode() {}

func (a *AliasExpression) Pos() int             { return a.Token.Pos }
func (a *AliasExpression) End() int             { return a.OldName.End() }
func (a *AliasExpression) TokenLiteral() string { return a.Token.Type.Literal() }

// UndefExpression represents an `undef method1, method2, ...` statement
type UndefExpression struct {
	Token token.Token // the undef keyword
	Names []*Identifier
}

func (u *UndefExpression) String() string {
	var b strings.Builder
	u.WriteTo(&b)
	return b.String()
}

func (u *UndefExpression) WriteTo(b *strings.Builder) {
	b.WriteString("undef ")
	for i, n := range u.Names {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(n.Value)
	}
}
func (u *UndefExpression) expressionNode() {}

func (u *UndefExpression) Pos() int             { return u.Token.Pos }
func (u *UndefExpression) End() int             { return u.Names[len(u.Names)-1].End() }
func (u *UndefExpression) TokenLiteral() string { return u.Token.Type.Literal() }

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
func (pe *PrefixExpression) TokenLiteral() string { return pe.Token.Type.Literal() }
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
		return n.HadWhitespace
	case *FloatLiteral:
		return n.HadWhitespace
	}
	return false
}

func (pe *PrefixExpression) String() string {
	var b strings.Builder
	pe.WriteTo(&b)
	return b.String()
}

func (pe *PrefixExpression) WriteTo(b *strings.Builder) {
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
			atomic = true
		case *InfixExpression:
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
	wrap := pe.Operator != "^" && pe.Operator != "not" &&
		pe.Operator != "*" && pe.Operator != "**" && !atomic

	if wrap {
		b.WriteByte('(')
	}
	b.WriteString(pe.Operator)
	if pe.Right != nil {
		if pe.Operator == "not" {
			needWrap := true
			switch r := pe.Right.(type) {
			case *Identifier, *InstanceVariable, *ClassVariable, *Global,
				*Boolean, *Nil, *Self, *IntegerLiteral, *FloatLiteral,
				*ScopedIdentifier, *RegexLiteral, *StringLiteral,
				*SymbolLiteral, *ArrayLiteral, *HashLiteral:
				needWrap = false
			case *ParenExpression:
				if r.Expr == nil && len(r.Stmts) == 0 {
					b.WriteByte(' ')
					writeTo(r, b)
					if wrap {
						b.WriteByte(')')
					}
					return
				}
				needWrap = false
			}
			if needWrap {
				b.WriteByte('(')
				writeTo(pe.Right, b)
				b.WriteByte(')')
			} else {
				b.WriteByte(' ')
				writeTo(pe.Right, b)
			}
			if wrap {
				b.WriteByte(')')
			}
			return
		}
		if pe.Operator == "defined?" {
			b.WriteByte(' ')
		}
		if (pe.Operator == "-" || pe.Operator == "+") && operandHasLeadingSpace(pe.Right) {
			b.WriteByte(' ')
		}
		needsParens := pe.Operator == "^" && pinNeedsParens(pe.Right)
		if needsParens {
			b.WriteByte('(')
		}
		writeTo(pe.Right, b)
		if needsParens {
			b.WriteByte(')')
		}
	}
	if wrap {
		b.WriteByte(')')
	}
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
	return oe.Token.EndPos()
}

// TokenLiteral returns the literal from the infix operator token
func (oe *InfixExpression) TokenLiteral() string { return oe.Token.Type.Literal() }
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
	var b strings.Builder
	oe.WriteTo(&b)
	return b.String()
}

func (oe *InfixExpression) WriteTo(b *strings.Builder) {
	if oe.Operator == ":" {
		if sym, ok := oe.Left.(*SymbolLiteral); ok && sym.Token.Type == token.LABEL {
			if oe.Right == nil {
				// Hash-value-omission shorthand (Ruby 3.1+): `foo:` with
				// implicit value. Preserve the omitted form on re-emit.
				b.WriteString(sym.LabelText)
				return
			}
			b.WriteString(sym.LabelText)
			b.WriteByte(' ')
			writeTo(oe.Right, b)
			return
		}
	}
	parentPrec := rubyInfixPrec(oe.Operator)
	rightAssoc := rubyInfixRightAssoc(oe.Operator)
	// parentPrec == 0 means unknown operator -- fall back to conservative
	// always-wrap so unfamiliar ops can't change parse on re-read.
	wrapAll := parentPrec == 0

	render := func(child Expression, isLeft bool) {
		if child == nil {
			return
		}
		inf, ok := child.(*InfixExpression)
		if !ok {
			writeTo(child, b)
			return
		}
		// Modifier `rescue` parses its RHS via the `expr` grammar (not `arg`),
		// so anything down to `and`/`or` is absorbed without parens. Skipping
		// the defensive wrap matches MRI's parsetree (rescue_expression =
		// AndNode directly, not ParenthesesNode(AndNode)).
		if oe.Operator == "rescue" && !isLeft {
			writeTo(child, b)
			return
		}
		childPrec := rubyInfixPrec(inf.Operator)
		if childPrec == 0 {
			writeTo(child, b) // child already wraps itself
			return
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
			writeTo(child, b)
			return
		}
		b.WriteByte('(')
		writeTo(child, b)
		b.WriteByte(')')
	}

	if wrapAll {
		b.WriteByte('(')
	}
	render(oe.Left, true)
	b.WriteByte(' ')
	b.WriteString(oe.Operator)
	b.WriteByte(' ')
	render(oe.Right, false)
	if wrapAll {
		b.WriteByte(')')
	}
}

// RightwardAssignment represents a rightward assignment / one-line pattern match (expr => target)
type RightwardAssignment struct {
	Token token.Token // the => token
	Left  Expression  // the value expression
	Right Expression  // the pattern / target
}

func (ra *RightwardAssignment) expressionNode() {}

func (ra *RightwardAssignment) Pos() int {
	if ra.Left == nil {
		return ra.Token.Pos
	}
	return ra.Left.Pos()
}
func (ra *RightwardAssignment) End() int {
	if ra.Right == nil {
		if ra.Left != nil {
			return ra.Left.End()
		}
		return ra.Token.Pos + len(ra.Token.Type.Literal())
	}
	return ra.Right.End()
}
func (ra *RightwardAssignment) TokenLiteral() string { return ra.Token.Type.Literal() }
func (ra *RightwardAssignment) String() string {
	var b strings.Builder
	ra.WriteTo(&b)
	return b.String()
}

func (ra *RightwardAssignment) WriteTo(b *strings.Builder) {
	if ra.Left != nil {
		writeTo(ra.Left, b)
	}
	b.WriteString(" => ")
	if ra.Right != nil {
		writeTo(ra.Right, b)
	}
}

// ParenExpression wraps a parenthesised expression, preserving the parens
// for source roundtrip fidelity. (a; b; c) groups multiple statements,
// stored in Stmts; the single-expression case uses Expr.
type ParenExpression struct {
	Token         token.Token // the '(' token
	EndPos        int         // pos of the closing `)`
	Expr          Expression
	Stmts         []Expression // multi-statement form: (a; b; c). nil for single-expr.
	MultipleStmts bool         // true when source had leading/trailing void `;` (e.g. `(;x)` / `(x;)`) -- MRI tags this ParenthesesNodeFlags=multiple_statements
}

func (pe *ParenExpression) expressionNode()      {}
func (pe *ParenExpression) Pos() int             { return pe.Token.Pos }
func (pe *ParenExpression) End() int             { return pe.EndPos }
func (pe *ParenExpression) TokenLiteral() string { return pe.Token.Type.Literal() }
func (pe *ParenExpression) String() string {
	var b strings.Builder
	pe.WriteTo(&b)
	return b.String()
}

func (pe *ParenExpression) WriteTo(b *strings.Builder) {
	if len(pe.Stmts) > 0 {
		b.WriteByte('(')
		for i, s := range pe.Stmts {
			if i > 0 {
				b.WriteString("; ")
			}
			writeTo(s, b)
		}
		b.WriteByte(')')
		return
	}
	if pe.MultipleStmts && pe.Expr != nil {
		// Source had a leading/trailing void `;` -- emit `(;expr)` so MRI
		// re-parses with ParenthesesNodeFlags=multiple_statements.
		b.WriteString("(;")
		writeTo(pe.Expr, b)
		b.WriteByte(')')
		return
	}
	if pe.Expr == nil {
		b.WriteString("()")
		return
	}
	// Skip the wrap when Expr already emits its own outer parens.
	switch e := pe.Expr.(type) {
	case *ParenExpression:
		writeTo(pe.Expr, b)
		return
	case *PrefixExpression:
		// PrefixExpression never self-wraps `not`, and self-wraps -/+/!/~
		// only when operand is non-atomic; otherwise it emits bare and
		// ParenExpression must wrap to preserve grouping (otherwise MRI
		// doesn't see a ParenthesesNode on re-parse).
		s := e.String()
		if !(strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")")) {
			b.WriteByte('(')
			b.WriteString(s)
			b.WriteByte(')')
			return
		}
		b.WriteString(s)
		return
	case *InfixExpression:
		// Unknown-operator fallback in InfixExpression.String() wraps in ().
		if rubyInfixPrec(e.Operator) == 0 {
			writeTo(e, b)
			return
		}
	}
	b.WriteByte('(')
	writeTo(pe.Expr, b)
	b.WriteByte(')')
}

// FlipFlop is the stateful `..` / `...` predicate that Ruby produces when
// a Range-shape expression appears in conditional position
// (if / unless / while / until / ternary / modifier). Semantically it is
// a state machine -- true from the first time Left evaluates truthy
// until Right evaluates truthy (inclusive `..`) or flips off on the same
// tick (exclusive `...`). The parser converts Range-syntax in cond
// context into this node so the AST distinguishes flip-flop from a
// plain Range literal.
type FlipFlop struct {
	Token     token.Token // RANGE (`..`) or RANGEEX (`...`)
	Left      Expression
	Right     Expression
	Exclusive bool // true for `...`, false for `..`
}

func (ff *FlipFlop) expressionNode() {}

// Pos returns the position of the first character of the left endpoint.
func (ff *FlipFlop) Pos() int {
	if ff.Left != nil {
		return ff.Left.Pos()
	}
	return ff.Token.Pos
}

// End returns the position past the last character of the right endpoint.
func (ff *FlipFlop) End() int {
	if ff.Right != nil {
		return ff.Right.End()
	}
	return ff.Token.EndPos()
}

func (ff *FlipFlop) TokenLiteral() string { return ff.Token.Type.Literal() }

func (ff *FlipFlop) String() string {
	var b strings.Builder
	ff.WriteTo(&b)
	return b.String()
}

func (ff *FlipFlop) WriteTo(b *strings.Builder) {
	if ff.Left != nil {
		writeTo(ff.Left, b)
	}
	if ff.Exclusive {
		b.WriteString(" ... ")
	} else {
		b.WriteString(" .. ")
	}
	if ff.Right != nil {
		writeTo(ff.Right, b)
	}
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
