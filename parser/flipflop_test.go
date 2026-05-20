package parser

import (
	"fmt"
	"strings"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
	"github.com/lczyk/goruby/ast"
)

// TestFlipFlopASTDistinction asserts that `expr1..expr2` (and `...`) inside
// conditional position (if / unless / while / until / ternary / modifier)
// produces an AST node distinguishable from a plain Range. MRI 1.9-4.0 all
// parse this construct as a stateful flip-flop predicate, not a Range
// literal.
//
// Today goruby collapses both into the same `*ast.InfixExpression{Operator:
// ".."}`, losing the distinction. This test fails until the parser emits a
// dedicated flip-flop node (e.g. `*ast.FlipFlopExpression`) or otherwise
// tags the InfixExpression so flip-flop can be recovered from the AST.
//
// The check uses a type-name sentinel ("FlipFlop") so the test does not
// depend on a particular new type existing yet -- any node type containing
// "FlipFlop" in its name satisfies the assertion.
func TestFlipFlopASTDistinction(t *testing.T) {
	cases := []struct {
		name string
		src  string
		// extractCond pulls the predicate expression that should be a flip-flop.
		extractCond func(t *testing.T, prog *ast.Program) ast.Expression
	}{
		{
			name: "if-block inclusive",
			src:  "if (i == 1)..(i == 5)\nputs i\nend",
			extractCond: func(t *testing.T, p *ast.Program) ast.Expression {
				return condOfFirstIf(t, p)
			},
		},
		{
			name: "if-block exclusive",
			src:  "if (i == 1)...(i == 5)\nputs i\nend",
			extractCond: func(t *testing.T, p *ast.Program) ast.Expression {
				return condOfFirstIf(t, p)
			},
		},
		{
			name: "modifier-if",
			src:  "puts i if (i == 1)..(i == 5)",
			extractCond: func(t *testing.T, p *ast.Program) ast.Expression {
				return condOfFirstIf(t, p)
			},
		},
		{
			name: "unless-block",
			src:  "unless (i == 1)..(i == 5)\nputs i\nend",
			extractCond: func(t *testing.T, p *ast.Program) ast.Expression {
				return condOfFirstIf(t, p)
			},
		},
		{
			name: "while-predicate",
			src:  "while (i == 0)..(i == 3)\ni += 1\nend",
			extractCond: func(t *testing.T, p *ast.Program) ast.Expression {
				return condOfFirstLoop(t, p)
			},
		},
		{
			name: "until-predicate",
			src:  "until (i == 0)...(i == 3)\ni += 1\nend",
			extractCond: func(t *testing.T, p *ast.Program) ast.Expression {
				return condOfFirstLoop(t, p)
			},
		},
		{
			name: "ternary",
			src:  "((i == 1)..(i == 5)) ? :in : :out",
			extractCond: func(t *testing.T, p *ast.Program) ast.Expression {
				return condOfFirstIf(t, p)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prog, err := parseSource(tc.src)
			checkParserErrors(t, err)
			cond := tc.extractCond(t, prog)
			require.NotNil(t, cond, "flip-flop predicate missing")

			typeName := fmt.Sprintf("%T", cond)
			assert.That(t,
				strings.Contains(typeName, "FlipFlop"),
				"flip-flop predicate must produce a FlipFlop-named AST node, got %s (op `..`/`...` in conditional position must be distinguishable from a Range literal). source: %q",
				typeName, tc.src,
			)
		})
	}
}

// TestFlipFlopVsRangeDistinct asserts that the same `..` operator produces
// distinct AST node types depending on context: a flip-flop predicate
// (conditional position) vs a Range literal (everywhere else). Today both
// collapse to the same `*ast.InfixExpression`, so this fails.
func TestFlipFlopVsRangeDistinct(t *testing.T) {
	// Range literal: not in conditional position. Same `..` operator,
	// same parenthesised endpoints -- only the surrounding context
	// differs.
	rangeProg, err := parseSource("r = (i == 1)..(i == 5)")
	checkParserErrors(t, err)
	require.That(t, len(rangeProg.Statements) >= 1, "no statements parsed")

	// Flip-flop: same `..` operator but inside an if-condition.
	flipProg, err := parseSource("if (i == 1)..(i == 5)\nputs i\nend")
	checkParserErrors(t, err)

	flipCond := condOfFirstIf(t, flipProg)
	require.NotNil(t, flipCond, "flip-flop predicate missing")

	rangeAssign, ok := rangeProg.Statements[0].(*ast.ExpressionStatement)
	require.That(t, ok, "expected ExpressionStatement, got %T", rangeProg.Statements[0])
	assignExpr, ok := rangeAssign.Expression.(*ast.Assignment)
	require.That(t, ok, "expected Assignment, got %T", rangeAssign.Expression)

	rangeType := fmt.Sprintf("%T", assignExpr.Right)
	flipType := fmt.Sprintf("%T", flipCond)
	assert.That(t, rangeType != flipType,
		"Range literal and flip-flop must have distinct AST node types, both got %s",
		rangeType,
	)
}

// TestFlipFlopExclusiveDistinct asserts that inclusive (`..`) and exclusive
// (`...`) flip-flops are distinguishable in the AST -- MRI flags them
// separately and the semantics differ (one-tick-off behaviour).
func TestFlipFlopExclusiveDistinct(t *testing.T) {
	incl, err := parseSource("if (i == 1)..(i == 5)\nputs i\nend")
	checkParserErrors(t, err)
	excl, err := parseSource("if (i == 1)...(i == 5)\nputs i\nend")
	checkParserErrors(t, err)

	inclCond := condOfFirstIf(t, incl)
	exclCond := condOfFirstIf(t, excl)
	require.NotNil(t, inclCond, "incl cond missing")
	require.NotNil(t, exclCond, "excl cond missing")

	// Either distinct types, OR same type w/ a distinguishing field.
	// Today both are *ast.InfixExpression with the operator being the
	// only differentiator -- but neither carries a "flip-flop" tag, so
	// this test fails on the FlipFlop-name check first.
	inclStr := inclCond.String()
	exclStr := exclCond.String()
	assert.That(t, inclStr != exclStr,
		"inclusive (`..`) and exclusive (`...`) flip-flops must render differently; both got %q",
		inclStr,
	)
}

// --- helpers ---------------------------------------------------------------

// condOfFirstIf returns the predicate expression of the first if/unless/ternary
// statement in the program, with outer ParenExpression wraps peeled away.
// Returns nil if the program shape doesn't match.
func condOfFirstIf(t *testing.T, prog *ast.Program) ast.Expression {
	t.Helper()
	for _, s := range prog.Statements {
		es, ok := s.(*ast.ExpressionStatement)
		if !ok {
			continue
		}
		if ce, ok := es.Expression.(*ast.ConditionalExpression); ok {
			return unwrapParens(ce.Condition)
		}
	}
	return nil
}

func unwrapParens(e ast.Expression) ast.Expression {
	for {
		pe, ok := e.(*ast.ParenExpression)
		if !ok {
			return e
		}
		e = pe.Expr
	}
}

// condOfFirstLoop returns the predicate of the first while/until loop.
func condOfFirstLoop(t *testing.T, prog *ast.Program) ast.Expression {
	t.Helper()
	for _, s := range prog.Statements {
		es, ok := s.(*ast.ExpressionStatement)
		if !ok {
			continue
		}
		if le, ok := es.Expression.(*ast.LoopExpression); ok {
			return le.Condition
		}
	}
	return nil
}
