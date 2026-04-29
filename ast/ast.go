package ast

import (
	"fmt"
	"strings"

	"github.com/lczyk/goruby/ast/infix"
)

// Node represents a node within the AST
//
// All node types implement the Node interface.
type Node interface {
	node() // marks this as a node
	Coder
	String() string
}

type Statement interface {
	Node
	statementNode()
}

type Expression interface {
	Node
	expressionNode()
}

type Coder interface {
	Code() string
}

////////////////////////////////////////////////////////////////////////////////

// A Program node is the root node within the AST.
type Program struct {
	Statements []Statement
}

func (p *Program) node()          {}
func (p *Program) String() string { return "<<<Program>>>" }

func (p *Program) Code() string {
	stmts := make([]string, len(p.Statements))
	for i, s := range p.Statements {
		if s != nil {
			stmts[i] = s.Code()
		}
	}
	return strings.Join(stmts, "; ")
}

var (
	_ Node = &Program{}
)

// A ReturnStatement represents a return node which yields another Expression.
type ReturnStatement struct {
	ReturnValue Expression
}

func (rs *ReturnStatement) node()          {}
func (rs *ReturnStatement) statementNode() {}
func (rs *ReturnStatement) String() string { return "<<<ReturnStatement>>>" }

func (rs *ReturnStatement) Code() string {
	var out strings.Builder
	out.WriteString("return ")
	if rs.ReturnValue != nil {
		out.WriteString(rs.ReturnValue.Code())
	}
	return out.String()
}

var (
	_ Node      = &ReturnStatement{}
	_ Statement = &ReturnStatement{}
)

// An ExpressionStatement is a Statement wrapping an Expression
type ExpressionStatement struct {
	Expression Expression
}

func (es *ExpressionStatement) node()          {}
func (es *ExpressionStatement) statementNode() {}
func (es *ExpressionStatement) String() string { return "<<<ExpressionStatement>>>" }

func (es *ExpressionStatement) Code() string {
	if es.Expression == nil {
		return ""
	}
	return es.Expression.Code()
}

var (
	_ Node      = &ExpressionStatement{}
	_ Statement = &ExpressionStatement{}
)

// BlockStatement represents a list of statements
type BlockStatement struct {
	Statements []Statement
}

func (bs *BlockStatement) node()          {}
func (bs *BlockStatement) statementNode() {}
func (bs *BlockStatement) String() string { return "<<<BlockStatement>>>" }

// Code returns the code representation of the block
func (bs *BlockStatement) Code() string {
	var out strings.Builder
	statement_strings := make([]string, 0)
	for _, s := range bs.Statements {
		if s == nil {
			continue
		}
		statement_string := s.Code()
		statement_strings = append(statement_strings, statement_string)
	}

	if len(statement_strings) == 0 {
		return ""
	}

	out.WriteString(strings.Join(statement_strings, ";"))
	return out.String()
}

var (
	_ Node      = &BlockStatement{}
	_ Statement = &BlockStatement{}
)

// A BreakStatement represents a break statement
type BreakStatement struct {
	Condition Expression
	Unless    bool
}

func (bs *BreakStatement) node()          {}
func (bs *BreakStatement) statementNode() {}
func (bs *BreakStatement) String() string { return "<<<Break Statement>>>" }

func (bs *BreakStatement) Code() string {
	var out strings.Builder
	out.WriteString("break ")
	if bs.Unless {
		out.WriteString("unless ")
	} else {
		out.WriteString("if ")
	}
	if bs.Condition != nil {
		out.WriteString(bs.Condition.Code())
	}
	return out.String()
}

var (
	_ Node      = &BreakStatement{}
	_ Statement = &BreakStatement{}
)

// Assignment represents a generic assignment
type Assignment struct {
	Left  Expression
	Right Expression
}

func (a *Assignment) node()           {}
func (a *Assignment) expressionNode() {}
func (a *Assignment) String() string  { return "<<<Assignment>>>" }

func (a *Assignment) Code() string {
	var out strings.Builder
	out.WriteString(maybeParenthesize(a.Left.Code(), needsParens(a.Left)))
	out.WriteString(" = ")
	out.WriteString(maybeParenthesize(a.Right.Code(), needsParens(a.Right)))
	return out.String()
}

var (
	_ Node       = &Assignment{}
	_ Expression = &Assignment{}
)

// MultiAssignment represents multiple variables on the left-hand side
type MultiAssignment struct {
	Variables []*Identifier
	Values    []Expression
}

func (m *MultiAssignment) node()           {}
func (m *MultiAssignment) expressionNode() {}

func (m *MultiAssignment) String() string { return "<<<MultiAssignment>>>" }
func (m *MultiAssignment) Code() string {
	var out strings.Builder
	vars := make([]string, len(m.Variables))
	for i, v := range m.Variables {
		vars[i] = v.Value
	}
	out.WriteString(strings.Join(vars, ", "))
	out.WriteString(" = ")
	values := make([]string, len(m.Values))
	for i, v := range m.Values {
		values[i] = v.Code()
	}
	out.WriteString(strings.Join(values, ", "))
	return out.String()
}

var (
	_ Node       = &MultiAssignment{}
	_ Expression = &MultiAssignment{}
)

type Identifier struct {
	Value string
}

func (i *Identifier) node()           {}
func (i *Identifier) expressionNode() {}
func (i *Identifier) String() string  { return "<<<Identifier>>>: " }
func (i *Identifier) Code() string    { return i.Value }

var (
	_ Node       = &Identifier{}
	_ Expression = &Identifier{}
)

func (i *Identifier) IsConstant() bool {
	if len(i.Value) == 0 {
		return false
	}
	return i.Value[0] >= 'A' && i.Value[0] <= 'Z'
}

func (i *Identifier) IsGlobal() bool {
	if len(i.Value) == 0 {
		return false
	}
	return i.Value[0] == '$'
}

// IntegerLiteral represents an integer in the AST
type IntegerLiteral struct {
	Value int64
}

func (il *IntegerLiteral) node()           {}
func (il *IntegerLiteral) expressionNode() {}
func (il *IntegerLiteral) String() string  { return "<<<IntegerLiteral>>>" }
func (il *IntegerLiteral) Code() string {
	var out strings.Builder
	out.WriteString(fmt.Sprintf("%d", il.Value))
	return out.String()
}

var (
	_ Node       = &IntegerLiteral{}
	_ Expression = &IntegerLiteral{}
)

// FloatLiteral represents a float in the AST
type FloatLiteral struct {
	Value float64
}

func (fl *FloatLiteral) node()           {}
func (fl *FloatLiteral) expressionNode() {}
func (fl *FloatLiteral) String() string  { return "<<<FloatLiteral>>>" }
func (fl *FloatLiteral) Code() string    { return fmt.Sprintf("%f", fl.Value) }

var (
	_ Node       = &FloatLiteral{}
	_ Expression = &FloatLiteral{}
)

// StringLiteral represents a double quoted string in the AST
type StringLiteral struct {
	Value string
}

func (sl *StringLiteral) node()           {}
func (sl *StringLiteral) expressionNode() {}
func (sl *StringLiteral) String() string  { return "<<<StringLiteral>>>" }
func (sl *StringLiteral) Code() string {
	var out strings.Builder
	out.WriteString("\"")
	out.WriteString(sl.Value)
	out.WriteString("\"")
	return out.String()
}

var (
	_ Node       = &StringLiteral{}
	_ Expression = &StringLiteral{}
)

// Comment represents a double quoted string in the AST
type Comment struct {
	Value string
}

func (c *Comment) node()          {}
func (c *Comment) statementNode() {}
func (c *Comment) String() string { return "<<<Comment>>>" }
func (c *Comment) Code() string {
	if strings.HasPrefix(c.Value, "#") {
		return c.Value
	}
	return "# " + strings.TrimLeft(c.Value, " ")
}

var (
	_ Node      = &Comment{}
	_ Statement = &Comment{}
)

// SymbolLiteral represents a symbol within the AST
type SymbolLiteral struct {
	Value string
}

func (s *SymbolLiteral) node()           {}
func (s *SymbolLiteral) expressionNode() {}
func (s *SymbolLiteral) String() string  { return "<<<SymbolLiteral>>>" }
func (s *SymbolLiteral) Code() string    { return ":" + s.Value }

var (
	_ Node       = &SymbolLiteral{}
	_ Expression = &SymbolLiteral{}
)

// ConditionalExpression represents an if expression within the AST
type ConditionalExpression struct {
	Unless      bool // true = unless, false = if
	Condition   Expression
	Consequence *BlockStatement
	Alternative *BlockStatement
}

func (ce *ConditionalExpression) node()           {}
func (ce *ConditionalExpression) expressionNode() {}
func (ce *ConditionalExpression) String() string  { return "<<<ConditionalExpression>>>" }

func (ce *ConditionalExpression) Code() string {
	var out strings.Builder
	if ce.Unless {
		out.WriteString("unless ")
	} else {
		out.WriteString("if ")
	}
	out.WriteString(ce.Condition.Code())
	out.WriteString("; ")
	out.WriteString(ce.Consequence.Code())
	if ce.Alternative != nil {
		out.WriteString("else ")
		out.WriteString(ce.Alternative.Code())
	}
	out.WriteString(" end")
	return out.String()
}

var (
	_ Node       = &ConditionalExpression{}
	_ Expression = &ConditionalExpression{}
)

// A LoopExpression represents an infinite loop (with breaks)
type LoopExpression struct {
	Block *BlockStatement
}

func (ce *LoopExpression) node()           {}
func (ce *LoopExpression) expressionNode() {}
func (ce *LoopExpression) String() string  { return "<<<LoopExpression>>>" }

func (ce *LoopExpression) Code() string {
	var out strings.Builder
	out.WriteString("loop {")
	out.WriteString(ce.Block.Code())
	out.WriteString("}")
	return out.String()
}

var (
	_ Node       = &LoopExpression{}
	_ Expression = &LoopExpression{}
)

// ExpressionList represents a list of expressions within the AST divided by commas
type ExpressionList []Expression

func (el ExpressionList) node()           {}
func (el ExpressionList) expressionNode() {}

func (el ExpressionList) String() string {
	var out strings.Builder
	elements := []string{}
	for _, e := range el {
		elements = append(elements, e.Code())
	}
	out.WriteString(strings.Join(elements, ", "))
	return out.String()
}

func (el ExpressionList) Code() string {
	var out strings.Builder
	elements := []string{}
	for _, e := range el {
		e_str := e.Code()
		e_str = maybeParenthesize(e_str, needsParens(e))
		elements = append(elements, e_str)
	}
	out.WriteString(strings.Join(elements, ", "))
	return out.String()
}

var (
	_ Node       = ExpressionList{}
	_ Expression = ExpressionList{}
)

// ArrayLiteral represents an Array literal within the AST
type ArrayLiteral struct {
	Elements []Expression
}

func (al *ArrayLiteral) node()           {}
func (al *ArrayLiteral) expressionNode() {}
func (al *ArrayLiteral) String() string  { return "<<<ArrayLiteral>>>" }
func (al *ArrayLiteral) Code() string {
	var out strings.Builder
	elements := []string{}
	for _, el := range al.Elements {
		elements = append(elements, el.Code())
	}
	out.WriteString("[")
	out.WriteString(strings.Join(elements, ", "))
	out.WriteString("]")
	return out.String()
}

var (
	_ Node       = &ArrayLiteral{}
	_ Expression = &ArrayLiteral{}
)

// HashLiteral represents an Hash literal within the AST
type HashLiteral struct {
	Map map[Expression]Expression
}

func (hl *HashLiteral) node()           {}
func (hl *HashLiteral) expressionNode() {}
func (hl *HashLiteral) String() string  { return "<<<HashLiteral>>>" }
func (hl *HashLiteral) Code() string {
	var out strings.Builder
	elements := []string{}
	for key, val := range hl.Map {
		elements = append(elements, fmt.Sprintf("%q => %q", key.Code(), val.Code()))
	}
	out.WriteString("{")
	out.WriteString(strings.Join(elements, ", "))
	out.WriteString("}")
	return out.String()
}

var (
	_ Node       = &HashLiteral{}
	_ Expression = &HashLiteral{}
)

// RangeLiteral represents a range literal within the AST
type RangeLiteral struct {
	Left      Expression
	Right     Expression
	Inclusive bool
}

func (rl *RangeLiteral) node()           {}
func (rl *RangeLiteral) expressionNode() {}
func (rl *RangeLiteral) String() string  { return "<<<RangeLiteral>>>" }

func (rl *RangeLiteral) Code() string {
	var out strings.Builder
	out.WriteString(rl.Left.Code())
	out.WriteString(" ")
	if rl.Inclusive {
		out.WriteString("..")
	} else {
		out.WriteString("...")
	}
	out.WriteString(" ")
	out.WriteString(rl.Right.Code())
	return out.String()
}

var (
	_ Node       = &RangeLiteral{}
	_ Expression = &RangeLiteral{}
)

// A FunctionLiteral represents a function definition in the AST
type FunctionLiteral struct {
	Name       string
	Parameters []*FunctionParameter
	Body       *BlockStatement
}

func (fl *FunctionLiteral) node()           {}
func (fl *FunctionLiteral) expressionNode() {}

const BODY_STRING_LIMIT = 30

func (fl *FunctionLiteral) String() string {
	var out strings.Builder
	out.WriteString("<<<FunctionLiteral, name=\"")
	out.WriteString(fl.Name)
	args := []string{}
	for _, a := range fl.Parameters {
		args = append(args, a.Code())
	}
	out.WriteString("\", parameters=[")
	if len(args) == 0 {
		out.WriteString("]")
	} else {
		out.WriteString("\"")
		out.WriteString(strings.Join(args, "\", \""))
		out.WriteString("\"]")
	}
	out.WriteString(", body=\"")
	body_string := fl.Body.Code()
	if len(body_string) > BODY_STRING_LIMIT {
		body_string = body_string[:BODY_STRING_LIMIT] + "..."
	}
	out.WriteString(body_string)
	out.WriteString("\">>>")
	return out.String()
}

func (fl *FunctionLiteral) Code() string {
	var out strings.Builder
	if strings.HasPrefix(fl.Name, "__") {
		out.WriteString("-> ")
		out.WriteString("(")
		args := []string{}
		for _, a := range fl.Parameters {
			args = append(args, a.Code())
		}
		out.WriteString(strings.Join(args, ", "))
		out.WriteString(") {")
		out.WriteString(fl.Body.Code())
		out.WriteString("}")
	} else {
		out.WriteString("def ")
		out.WriteString(fl.Name)
		out.WriteString("(")
		args := []string{}
		for _, a := range fl.Parameters {
			args = append(args, a.Code())
		}
		out.WriteString(strings.Join(args, ", "))
		out.WriteString(")")
		out.WriteString("\n")
		body_string := fl.Body.Code()
		for _, line := range strings.Split(body_string, "\n") {
			out.WriteString("    ")
			out.WriteString(line)
			out.WriteString("\n")
		}
		out.WriteString("end")
	}
	return out.String()
}

var (
	_ Node       = &FunctionLiteral{}
	_ Expression = &FunctionLiteral{}
)

// A FunctionParameter represents a parameter in a function literal
type FunctionParameter struct {
	Name    string
	Default Expression
	Splat   bool
}

func (f *FunctionParameter) node()           {}
func (f *FunctionParameter) expressionNode() {}

func (f *FunctionParameter) String() string {
	return "<<<FunctionParameter, name=\"" + f.Name + "\">>>"
}
func (f *FunctionParameter) Code() string {
	var out strings.Builder
	if f.Splat {
		out.WriteString("*")
	}
	out.WriteString(f.Name)
	if f.Default != nil {
		out.WriteString(" = ")
		out.WriteString(f.Default.Code())
	}
	return out.String()
}

var (
	_ Node       = &FunctionParameter{}
	_ Expression = &FunctionParameter{}
)

// A Splat represents a splat operator in the AST
type Splat struct {
	Value Expression
}

func (s *Splat) node()           {}
func (s *Splat) expressionNode() {}
func (s *Splat) String() string  { return "<<<Splat>>>" }
func (s *Splat) Code() string {
	var out strings.Builder
	out.WriteString("*")
	out.WriteString(s.Value.Code())
	return out.String()
}

var (
	_ Node       = &Splat{}
	_ Expression = &Splat{}
)

// An IndexExpression represents an array or hash access in the AST
type IndexExpression struct {
	Left  Expression
	Index Expression
}

func (ie *IndexExpression) node()           {}
func (ie *IndexExpression) expressionNode() {}

func (ie *IndexExpression) String() string {
	var out strings.Builder
	out.WriteString("<<<IndexExpression>>>")
	return out.String()
}

func (ie *IndexExpression) Code() string {
	var out strings.Builder
	out.WriteString(ie.Left.Code())
	out.WriteString("[")
	out.WriteString(ie.Index.Code())
	out.WriteString("]")
	return out.String()
}

var (
	_ Node       = &IndexExpression{}
	_ Expression = &IndexExpression{}
)

// A ContextCallExpression represents a method call on a given Context
type ContextCallExpression struct {
	Context   Expression   // The left-hand side expression
	Function  string       // The function to call
	Arguments []Expression // Normal arguments
	Block     Expression   // Block argument, if any
}

func (ce *ContextCallExpression) node()           {}
func (ce *ContextCallExpression) expressionNode() {}
func (ce *ContextCallExpression) String() string  { return "<<<ContextCallExpression>>>" }

func (ce *ContextCallExpression) Code() string {
	var out strings.Builder
	if ce.Context != nil {
		ce_str := ce.Context.Code()
		ce_str = maybeParenthesize(ce_str, needsParens(ce.Context))
		out.WriteString(ce_str)
		out.WriteString(".")
	}
	out.WriteString(ce.Function)
	args := []string{}
	for _, a := range ce.Arguments {
		args = append(args, a.Code())
	}
	if len(args) > 0 {
		out.WriteString("(")
		out.WriteString(strings.Join(args, ", "))
		out.WriteString(")")
	}
	if ce.Block != nil {
		out.WriteString(" ")
		out.WriteString(ce.Block.Code())
	}
	return out.String()
}

var (
	_ Node       = &ContextCallExpression{}
	_ Expression = &ContextCallExpression{}
)

// PrefixExpression represents a prefix operator
type PrefixExpression struct {
	Operator string
	Right    Expression
}

func (pe *PrefixExpression) node()           {}
func (pe *PrefixExpression) expressionNode() {}
func (pe *PrefixExpression) String() string  { return "<<<PrefixExpression>>>" }
func (pe *PrefixExpression) Code() string {
	var out strings.Builder
	out.WriteString(pe.Operator)
	out.WriteString(maybeParenthesize(pe.Right.Code(), needsParens(pe.Right)))
	return out.String()
}

var (
	_ Node       = &PrefixExpression{}
	_ Expression = &PrefixExpression{}
)

// An InfixExpression represents an infix operator in the AST
type InfixExpression struct {
	Left     Expression
	Operator infix.Infix
	Right    Expression
}

func (oe *InfixExpression) node()           {}
func (oe *InfixExpression) expressionNode() {}

func (oe *InfixExpression) String() string {
	var out strings.Builder
	out.WriteString("<<<InfixExpression>>>")
	return out.String()
}

func (oe *InfixExpression) Code() string {
	var out strings.Builder
	out.WriteString(maybeParenthesize(oe.Left.Code(), needsParens(oe.Left)))
	out.WriteString(" ")
	out.WriteString(oe.Operator.String())
	out.WriteString(" ")
	out.WriteString(maybeParenthesize(oe.Right.Code(), needsParens(oe.Right)))
	return out.String()
}

var (
	_ Node       = &InfixExpression{}
	_ Expression = &InfixExpression{}
)

func needsParens(e Expression) bool {
	// This function checks if the expression needs parentheses
	// For now, we put everything except Identifiers in parentheses
	switch e := e.(type) {
	case *Identifier:
	case *StringLiteral:
	case *IntegerLiteral:
	case *SymbolLiteral:
	case *FloatLiteral:
	case *ContextCallExpression:
		if e.Context == nil {
			// we're calling a function without a context, aka just calling a function
			// we should not need to parenthesize this, i don't think... -MK
			return false
		}
	case *IndexExpression:
		// Again, i don't think we ever need to parenthesize this... -MK
		return false
	default:
		return true
	}
	return false
}

func maybeParenthesize(s string, parens bool) string {
	if parens {
		return "(" + s + ")"
	}
	return s
}
