package parser_test

import (
	"fmt"
	gotoken "go/token"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/MarcinKonowalczyk/goruby/ast"
	"github.com/MarcinKonowalczyk/goruby/ast/infix"
	p "github.com/MarcinKonowalczyk/goruby/parser"
	"github.com/lczyk/assert"
	"github.com/lczyk/assert/compare"
	"github.com/pkg/errors"
)

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func TestAssignment(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		leftType  reflect.Type
		rightType reflect.Type
	}{
		{
			name:      "hash index assignment",
			input:     `x[:foo] = 3`,
			leftType:  reflect.TypeOf(&ast.IndexExpression{}),
			rightType: reflect.TypeOf(&ast.IntegerLiteral{}),
		},
		{
			name:      "local varibale",
			input:     `x = 3`,
			leftType:  reflect.TypeOf(&ast.Identifier{}),
			rightType: reflect.TypeOf(&ast.IntegerLiteral{}),
		},
		{
			name:      "method call with block on rhs",
			input:     `x = foo { |x| }`,
			leftType:  reflect.TypeOf(&ast.Identifier{}),
			rightType: reflect.TypeOf(&ast.ContextCallExpression{}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			program, err := parseSource(tt.input)
			checkParserErrors(t, err)
			assert.Equal(t, len(program.Statements), 1)
			stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
			assert.That(t, ok, "program.Statements[0] is not ast.ExpressionStatement. got=%T", program.Statements[0])

			assign, ok := stmt.Expression.(*ast.Assignment)
			assert.That(t, ok, "stmt.Expression is not *ast.Assignment. got=%T", stmt.Expression)

			{
				actual := reflect.TypeOf(assign.Left)
				assert.Equal(t, actual, tt.leftType)
			}

			{
				actual := reflect.TypeOf(assign.Right)
				assert.Equal(t, actual, tt.rightType)
			}
		})
	}
}

func TestAssignmentOperator(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		leftType      reflect.Type
		rightOperator infix.Infix
	}{
		{
			name:          "-=",
			input:         `x -= 3`,
			leftType:      reflect.TypeOf(&ast.Identifier{}),
			rightOperator: infix.MINUS,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			program, err := parseSource(tt.input)
			checkParserErrors(t, err)

			assert.Equal(t, len(program.Statements), 1)
			stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
			assert.That(t, ok, "program.Statements[0] is not ast.ExpressionStatement. got=%T", program.Statements[0])

			assign, ok := stmt.Expression.(*ast.Assignment)
			assert.That(t, ok, "stmt.Expression is not *ast.Assignment. got=%T", stmt.Expression)

			{
				actual := reflect.TypeOf(assign.Left)
				assert.Equal(t, actual, tt.leftType)
			}

			{
				infix_exp, ok := assign.Right.(*ast.InfixExpression)
				assert.That(t, ok, "expected right assign type to be %T, got %T", infix_exp, assign.Right)
				assert.Equal(t, infix_exp.Operator, tt.rightOperator)
			}
		})
	}
}

func TestVariableExpression(t *testing.T) {
	t.Run("valid variable expressions", func(t *testing.T) {
		tests := []struct {
			input              string
			expectedIdentifier string
			expectedValue      string
		}{
			{"x = 5;", "x", "5"},
			{"x = 5_0;", "x", "50"},
			{"y = true;", "y", ":true"},
			{"foobar = y;", "foobar", "y"},
			{"foobar = (12 + 2 * bar) - x;", "foobar", "(12 + (2 * bar)) - x"},
		}

		for _, tt := range tests {
			program, err := parseSource(tt.input)
			checkParserErrors(t, err)
			assert.Equal(t, len(program.Statements), 1)
			stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
			assert.That(t, ok, "program.Statements[0] is not ast.ExpressionStatement. got=%T", program.Statements[0])

			variable, ok := stmt.Expression.(*ast.Assignment)
			assert.That(t, ok, "stmt.Expression is not *ast.Assignment. got=%T", stmt.Expression)
			testIdentifier(t, variable.Left, tt.expectedIdentifier)
			assert.Equal(t, variable.Right.Code(), tt.expectedValue)
		}
	})
	t.Run("const assignment within function", func(t *testing.T) {
		tests := []struct {
			desc  string
			input string
			err   error
		}{
			{
				desc: "single const assign",
				input: `
				def foo
					Ten = 10
				end`,
				err: fmt.Errorf("dynamic constant assignment"),
			},
			{
				desc: "const assign as multiassign",
				input: `
				def foo
					x, Ten = 10, 20
				end`,
				err: fmt.Errorf("dynamic constant assignment"),
			},
		}

		for _, tt := range tests {
			t.Run(tt.desc, func(t *testing.T) {

				_, errs := parseExpression(tt.input)
				assert.NotEqual(t, errs, nil)

				errors := errs.Errors
				assert.Equal(t, len(errors), 1)
				assert.Error(t, errors[0], tt.err)
			})
		}
	})
}

func TestGlobalAssignment(t *testing.T) {
	input := "$foo = 3"

	program, err := parseSource(input)
	checkParserErrors(t, err)
	assert.Equal(t, len(program.Statements), 1)
	stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
	assert.That(t, ok, "program.Statements[0] is not ast.ExpressionStatement. got=%T", program.Statements[0])

	variable, ok := stmt.Expression.(*ast.Assignment)
	assert.That(t, ok, "stmt.Expression is not *ast.Assignment. got=%T", stmt.Expression)

	testGlobal(t, variable.Left, "$foo")
	assert.Equal(t, variable.Right.Code(), "3")
}

func TestParseMultiAssignment(t *testing.T) {
	tests := []struct {
		input     string
		variables []string
		values    []string
	}{
		{
			input:     "x, y, z = 3, 4, 5;",
			variables: []string{"x", "y", "z"},
			values:    []string{"3", "4", "5"},
		},
		{
			input:     "x, y = 3, 4;",
			variables: []string{"x", "y"},
			values:    []string{"3", "4"},
		},
		{
			input:     "x, y, z = 3, 4;",
			variables: []string{"x", "y", "z"},
			values:    []string{"3", "4"},
		},
		{
			input:     "x, y, z = 3;",
			variables: []string{"x", "y", "z"},
			values:    []string{"3"},
		},
		{
			input:     "x[0], $y, A = 3, 4, 5;",
			variables: []string{"x[0]", "$y", "A"},
			values:    []string{"3", "4", "5"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			expr, err := parseExpression(tt.input)
			checkParserErrors(t, err)

			assign, ok := expr.(*ast.Assignment)
			assert.That(t, ok, "Expected expression to be %T, got %T\n", assign, expr)

			left, ok := assign.Left.(ast.ExpressionList)
			assert.That(t, ok, "Expected left to be %T, got %T\n", left, assign.Left)

			actualVars := make([]string, len(left))
			for i, v := range left {
				actualVars[i] = v.Code()
			}
			assert.EqualCmp(t, tt.variables, actualVars, compare.Arrays)
			assert.Equal(t, strings.Join(tt.values, ", "), assign.Right.Code())
		})
	}
}

func TestReturnStatements(t *testing.T) {
	tests := []struct {
		input         string
		expectedValue interface{}
	}{
		{"return 5;", 5},
		{"return true;", true},
		{"return foobar;", "foobar"},
		{"return 3, 5, 8;", []string{"3", "5", "8"}},
	}

	for _, tt := range tests {
		program, err := parseSource(tt.input)
		checkParserErrors(t, err)
		assert.Equal(t, len(program.Statements), 1)

		stmt := program.Statements[0]
		returnStmt, ok := stmt.(*ast.ReturnStatement)
		assert.That(t, ok, "stmt not *ast.returnStatement. got=%T", stmt)
		testLiteralExpression(t, returnStmt.ReturnValue, tt.expectedValue)
	}
}

func TestParseComment(t *testing.T) {
	t.Run("line comment newline", func(t *testing.T) {
		tests := []struct {
			input        string
			commentValue string
		}{
			{
				input:        "# a comment\n",
				commentValue: "# a comment",
			},
			{
				input:        "# a comment",
				commentValue: "# a comment",
			},
			{
				input:        "# a comment;",
				commentValue: "# a comment;",
			},
		}

		for _, tt := range tests {
			program, err := parseSource(tt.input)
			checkParserErrors(t, err)
			assert.Equal(t, len(program.Statements), 1)

			comment, ok := program.Statements[0].(*ast.Comment)
			assert.That(t, ok, "Expected program.Statements[0] to be %T, got %T\n", comment, program.Statements[0])
			assert.Equal(t, comment.Value, tt.commentValue)
		}
	})
	t.Run("inline comment", func(t *testing.T) {
		tests := []struct {
			input        string
			commentValue string
		}{
			{
				input:        "foo # a comment\n",
				commentValue: "# a comment",
			},
			{
				input:        "foo # a comment",
				commentValue: "# a comment",
			},
			{
				input:        "foo # a comment;",
				commentValue: "# a comment;",
			},
		}

		for _, tt := range tests {
			program, err := parseSource(tt.input)
			checkParserErrors(t, err)
			assert.Equal(t, len(program.Statements), 2)

			comment, ok := program.Statements[1].(*ast.Comment)
			assert.That(t, ok, "Expected program.Statements[1] to be %T, got %T\n", comment, program.Statements[1])
			assert.Equal(t, comment.Value, tt.commentValue)
		}
	})
}

func TestIdentifierExpression(t *testing.T) {
	t.Run("local variable", func(t *testing.T) {
		input := "foobar;"

		program, err := parseSource(input)
		checkParserErrors(t, err)

		assert.Equal(t, len(program.Statements), 1)
		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		assert.That(t, ok, "program.Statements[0] is not ast.ExpressionStatement. got=%T", program.Statements[0])

		ident, ok := stmt.Expression.(*ast.Identifier)
		assert.That(t, ok, "expression not *ast.Identifier. got=%T", stmt.Expression)
		assert.Equal(t, ident.Value, "foobar")
	})
	t.Run("constant", func(t *testing.T) {
		input := "Foobar;"

		program, err := parseSource(input)
		checkParserErrors(t, err)
		assert.Equal(t, len(program.Statements), 1)
		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		assert.That(t, ok, "program.Statements[0] is not ast.ExpressionStatement. got=%T", program.Statements[0])

		ident, ok := stmt.Expression.(*ast.Identifier)
		assert.That(t, ok, "expression not *ast.Identifier. got=%T", stmt.Expression)
		assert.Equal(t, ident.Value, "Foobar")
	})
}

func TestGlobalExpression(t *testing.T) {
	input := "$foobar;"

	program, err := parseSource(input)
	checkParserErrors(t, err)
	assert.Equal(t, len(program.Statements), 1)
	stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
	assert.That(t, ok, "program.Statements[0] is not ast.ExpressionStatement. got=%T", program.Statements[0])

	global, ok := stmt.Expression.(*ast.Identifier)
	assert.That(t, ok, "expression not *ast.Global. got=%T", stmt.Expression)
	assert.Equal(t, global.Value, "$foobar")
	assert.That(t, global.IsGlobal(), "global not set to true")
}

func TestGlobalExpressionWithIndex(t *testing.T) {
	input := "$foobar[1];"
	program, err := parseSource(input)
	checkParserErrors(t, err)

	assert.Equal(t, len(program.Statements), 1)
	stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
	assert.That(t, ok, "program.Statements[0] is not ast.ExpressionStatement. got=%T", program.Statements[0])

	index, ok := stmt.Expression.(*ast.IndexExpression)
	assert.That(t, ok, "expression not *ast.IndexExpression. got=%T", stmt.Expression)

	testGlobal(t, index.Left, "$foobar")
	testLiteralExpression(t, index.Index, 1)
}

func TestIntegerLiteralExpression(t *testing.T) {
	input := "5;"

	program, err := parseSource(input)
	checkParserErrors(t, err)
	assert.Equal(t, len(program.Statements), 1)
	stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
	assert.That(t, ok, "program.Statements[0] is not ast.ExpressionStatement. got=%T", program.Statements[0])

	literal, ok := stmt.Expression.(*ast.IntegerLiteral)
	assert.That(t, ok, "expression not *ast.IntegerLiteral. got=%T", stmt.Expression)
	assert.Equal(t, literal.Value, 5)
}

func TestParsingPrefixExpressions(t *testing.T) {
	prefixTests := []struct {
		input    string
		operator string
		value    interface{}
	}{
		{"!5;", "!", 5},
		{"-15;", "-", 15},
		{"!foobar;", "!", "foobar"},
		{"-foobar;", "-", "foobar"},
		{"!true;", "!", true},
		{"!false;", "!", false},
	}

	for _, tt := range prefixTests {
		program, err := parseSource(tt.input)
		checkParserErrors(t, err)
		assert.Equal(t, len(program.Statements), 1)

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		assert.That(t, ok, "program.Statements[0] is not ast.ExpressionStatement. got=%T", program.Statements[0])

		exp, ok := stmt.Expression.(*ast.PrefixExpression)
		assert.That(t, ok, "stmt.Expression is not ast.PrefixExpression. got=%T", stmt.Expression)
		assert.Equal(t, exp.Operator, tt.operator)
		testLiteralExpression(t, exp.Right, tt.value)
	}
}

func TestParsingInfixExpressions(t *testing.T) {
	t.Run("literal expressions", func(t *testing.T) {
		infixTests := []struct {
			input      string
			leftValue  interface{}
			operator   infix.Infix
			rightValue interface{}
		}{
			{"5 + 5;", 5, infix.PLUS, 5},
			{"5 - 5;", 5, infix.MINUS, 5},
			{"5 * 5;", 5, infix.ASTERISK, 5},
			{"5 / 5;", 5, infix.SLASH, 5},
			{"5 % 5;", 5, infix.MODULO, 5},
			{"5 > 5;", 5, infix.GT, 5},
			{"5 < 5;", 5, infix.LT, 5},
			{"5 >= 5;", 5, infix.GTE, 5},
			{"5 <= 5;", 5, infix.LTE, 5},
			{"5 == 5;", 5, infix.EQ, 5},
			{"5 != 5;", 5, infix.NOTEQ, 5},
			{"5 <=> 5;", 5, infix.SPACESHIP, 5},
			{"foobar + barfoo;", "foobar", infix.PLUS, "barfoo"},
			{"foobar - barfoo;", "foobar", infix.MINUS, "barfoo"},
			{"foobar * barfoo;", "foobar", infix.ASTERISK, "barfoo"},
			{"foobar / barfoo;", "foobar", infix.SLASH, "barfoo"},
			{"foobar > barfoo;", "foobar", infix.GT, "barfoo"},
			{"foobar < barfoo;", "foobar", infix.LT, "barfoo"},
			{"foobar == barfoo;", "foobar", infix.EQ, "barfoo"},
			{"foobar <=> barfoo;", "foobar", infix.SPACESHIP, "barfoo"},
			{"foobar != barfoo;", "foobar", infix.NOTEQ, "barfoo"},
			{"true == true", true, infix.EQ, true},
			{"true != false", true, infix.NOTEQ, false},
			{"false == false", false, infix.EQ, false},
			{"false || false", false, infix.LOGICALOR, false},
			{"false && false", false, infix.LOGICALAND, false},
		}

		for _, tt := range infixTests {
			program, err := parseSource(tt.input)
			checkParserErrors(t, err)
			assert.Equal(t, len(program.Statements), 1)

			stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
			assert.That(t, ok, "program.Statements[0] is not ast.ExpressionStatement. got=%T", program.Statements[0])

			testInfixExpression(t, stmt.Expression, tt.leftValue, tt.operator, tt.rightValue)
		}
	})
	t.Run("symbols expressions", func(t *testing.T) {
		input := ":bar <=> 13"

		program, err := parseSource(input)
		checkParserErrors(t, err)
		assert.Equal(t, len(program.Statements), 1)

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		assert.That(t, ok, "program.Statements[0] is not ast.ExpressionStatement. got=%T", program.Statements[0])

		_, ok = stmt.Expression.(*ast.InfixExpression)
		assert.That(t, ok, "stmt.Expression is not *ast.InfixExpression. got=%T", stmt.Expression)
	})
	t.Run("call expression no args", func(t *testing.T) {
		input := "foo.bar <=> 13"

		program, err := parseSource(input)
		checkParserErrors(t, err)
		assert.Equal(t, len(program.Statements), 1)

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		assert.That(t, ok, "program.Statements[0] is not ast.ExpressionStatement. got=%T", program.Statements[0])

		_, ok = stmt.Expression.(*ast.InfixExpression)
		assert.That(t, ok, "stmt.Expression is not *ast.InfixExpression. got=%T", stmt.Expression)
	})
	t.Run("call expression with one arg", func(t *testing.T) {
		input := "foo.bar 3 <=> 13"

		program, err := parseSource(input)
		checkParserErrors(t, err)
		assert.Equal(t, len(program.Statements), 1)

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		assert.That(t, ok, "program.Statements[0] is not ast.ExpressionStatement. got=%T", program.Statements[0])

		_, ok = stmt.Expression.(*ast.ContextCallExpression)
		assert.That(t, ok, "stmt.Expression is not *ast.ContextCallExpression. got=%T", stmt.Expression)
	})
	t.Run("call expression with two args", func(t *testing.T) {
		input := "foo.bar 3, 5 <=> 13"

		program, err := parseSource(input)
		checkParserErrors(t, err)
		assert.Equal(t, len(program.Statements), 1)

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		assert.That(t, ok, "program.Statements[0] is not ast.ExpressionStatement. got=%T", program.Statements[0])

		_, ok = stmt.Expression.(*ast.ContextCallExpression)
		assert.That(t, ok, "stmt.Expression is not *ast.ContextCallExpression. got=%T", stmt.Expression)
	})
	t.Run("complex infix with call expression with just a block", func(t *testing.T) {
		input := "1 + 21 * 8 - 3 <=> foo { |x| x }"

		expr, err := parseExpression(input)
		checkParserErrors(t, err)

		_, ok := expr.(*ast.InfixExpression)
		assert.That(t, ok, "stmt.Expression is not *ast.InfixExpression. got=%T", expr)
	})
	t.Run("easy infix with call expression with just a block", func(t *testing.T) {
		input := "1 <=> foo { |x| x }"

		expr, err := parseExpression(input)
		checkParserErrors(t, err)

		_, ok := expr.(*ast.InfixExpression)
		assert.That(t, ok, "stmt.Expression is not *ast.InfixExpression. got=%T", expr)
	})
}

func TestOperatorPrecedenceParsing(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			"-a * b",
			"(-a) * b",
		},
		{
			"!-a",
			"!(-a)",
		},
		{
			"a + b + c",
			"(a + b) + c",
		},
		{
			"a + b - c",
			"(a + b) - c",
		},
		{
			"a * b * c",
			"(a * b) * c",
		},
		{
			"a * b / c",
			"(a * b) / c",
		},
		{
			"a + b / c",
			"a + (b / c)",
		},
		{
			"a + b * c + d / e - f",
			"((a + (b * c)) + (d / e)) - f",
		},
		{
			"3 + 4; -5 * 5",
			"3 + 4; (-5) * 5",
		},
		{
			"5 > 4 == 3 < 4",
			"(5 > 4) == (3 < 4)",
		},
		{
			"5 < 4 != 3 > 4",
			"(5 < 4) != (3 > 4)",
		},
		{
			"3 + 4 * 5 == 3 * 1 + 4 * 5",
			"(3 + (4 * 5)) == ((3 * 1) + (4 * 5))",
		},
		{
			"3 + 4 * 5 == 3 * 1 + 4 * 5",
			"(3 + (4 * 5)) == ((3 * 1) + (4 * 5))",
		},
		{
			"true | true",
			":true || :true",
		},
		{
			"true & true",
			":true && :true",
		},
		{
			"3 > 5 == false",
			"(3 > 5) == :false",
		},
		{
			"3 < 5 == true",
			"(3 < 5) == :true",
		},
		{
			"1 + (2 + 3) + 4",
			"(1 + (2 + 3)) + 4",
		},
		{
			"(5 + 5) * 2",
			"(5 + 5) * 2",
		},
		{
			"2 / (5 + 5)",
			"2 / (5 + 5)",
		},
		{
			"(5 + 5) * 2 * (5 + 5)",
			"((5 + 5) * 2) * (5 + 5)",
		},
		{
			"-(5 + 5)",
			"-(5 + 5)",
		},
		{
			"!(true == true)",
			"!(:true == :true)",
		},
		{
			"a + add(b * c) + d",
			"(a + add(b * c)) + d",
		},
		{
			"add(a, b, 1, 2 * 3, 4 + 5, add(6, 7 * 8))",
			"add(a, b, 1, 2 * 3, 4 + 5, add(6, 7 * 8))",
		},
		{
			"add(a + b + c * d / f + g)",
			"add(((a + b) + ((c * d) / f)) + g)",
		},
		{
			"add(a + b + c * d / f + g)",
			"add(((a + b) + ((c * d) / f)) + g)",
		},
		{
			"x = 12 * 3;",
			"x = (12 * 3)",
		},
		{
			"x = 3 + 4 * 3;",
			"x = (3 + (4 * 3))",
		},
		{
			"x = add(4) * 3;",
			"x = (add(4) * 3)",
		},
		{
			"add(x = add(4) * 3);",
			"add(x = (add(4) * 3))",
		},
		{
			"a = b = 0;",
			"a = (b = 0)",
		},
		{
			"a * [1, 2, 3, 4][b * c] * d",
			"(a * [1, 2, 3, 4][b * c]) * d",
		},
		{
			"add(a * b[2], b[1], 2 * [1, 2][1])",
			"add(a * b[2], b[1], 2 * [1, 2][1])",
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			program, err := parseSource(tt.input)
			checkParserErrors(t, err)
			assert.Equal(t, program.Code(), tt.expected)
		})
	}
}

func TestBlockExpression(t *testing.T) {
	tests := []struct {
		input             string
		expectedArguments []*ast.Identifier
		expectedBody      string
	}{
		{
			"method { x }",
			nil,
			"x",
		},
		{
			"method { |x| x }",
			[]*ast.Identifier{{Value: "x"}},
			"x",
		},
	}

	for _, tt := range tests {
		program, err := parseSource(tt.input)
		checkParserErrors(t, err)
		assert.Equal(t, len(program.Statements), 1)

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		assert.That(t, ok, "program.Statements[0] is not ast.ExpressionStatement. got=%T", program.Statements[0])

		call, ok := stmt.Expression.(*ast.ContextCallExpression)
		assert.That(t, ok, "stmt.Expression is not *ast.ContextCallExpression. got=%T", stmt.Expression)

		block := assert.Type[*ast.FunctionLiteral](t, call.Block, "*ast.ContextCallExpression.Block is not *ast.FunctionLiteral. got=%T", call.Block)
		assert.NotEqual(t, block, nil)
		assert.Equal(t, len(block.Parameters), len(tt.expectedArguments))

		for i, arg := range block.Parameters {
			assert.Equal(t, arg.Code(), tt.expectedArguments[i].Code())
		}

		assert.Equal(t, block.Body.Code(), tt.expectedBody)
	}
}

func TestBooleanExpression(t *testing.T) {
	tests := []struct {
		input           string
		expectedBoolean string
	}{
		{"true;", ":true"},
		{"false;", ":false"},
	}

	for _, tt := range tests {
		program, err := parseSource(tt.input)
		checkParserErrors(t, err)
		assert.That(t, len(program.Statements) == 1, "program has not enough statements. got=%d", len(program.Statements))

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		assert.That(t, ok, "program.Statements[0] is not ast.ExpressionStatement. got=%T", program.Statements[0])

		boolean, ok := stmt.Expression.(*ast.SymbolLiteral)
		assert.That(t, ok, "expression not *ast.Boolean. got=%T", stmt.Expression)
		assert.Equal(t, boolean.Code(), tt.expectedBoolean)
	}
}

func TestNilExpression(t *testing.T) {
	input := "nil;"

	program, err := parseSource(input)
	checkParserErrors(t, err)
	assert.Equal(t, len(program.Statements), 1)

	stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
	assert.That(t, ok, "program.Statements[0] is not ast.ExpressionStatement. got=%T", program.Statements[0])
	nil_node, ok := stmt.Expression.(*ast.SymbolLiteral)
	assert.That(t, ok, "expression not *ast.Nil. got=%T", stmt.Expression)
	assert.Equal(t, nil_node.Code(), ":nil")
	assert.Equal(t, nil_node.Value, "nil")
}

func TestConditionalExpression(t *testing.T) {
	t.Run("with operator expression", func(t *testing.T) {
		tests := []struct {
			input                         string
			expectedConditionLeft         string
			expectedConditionOperator     infix.Infix
			expectedConditionRight        string
			expectedConsequenceExpression string
		}{
			{`if x < y
			x
			end`, "x", infix.LT, "y", "x"},
			{`if x < y then
			x
			end`, "x", infix.LT, "y", "x"},
			{`if x < y; x
			end`, "x", infix.LT, "y", "x"},
			{`if x < y
			if x == 3
			y
			end
			x
			end`, "x", infix.LT, "y", "if x == 3; y endx"},
			{`if x < y
			x = Object x
			end`, "x", infix.LT, "y", "x = Object(x)"},
			{"x 3 if x < y", "x", infix.LT, "y", "x(3)"},
			{"x.add 3 if x < y", "x", infix.LT, "y", "x.add(3)"},
			{`unless x < y
			x
			end`, "x", infix.LT, "y", "x"},
			{`unless x < y then
			x
			end`, "x", infix.LT, "y", "x"},
			{`unless x < y; x
			end`, "x", infix.LT, "y", "x"},
			{`unless x < y
			if x == 3
			y
			end
			x
			end`, "x", infix.LT, "y", "if x == 3; y endx"},
			{`unless x < y
			x = Object x
			end`, "x", infix.LT, "y", "x = Object(x)"},
			{"x = 3 if x < y", "x", infix.LT, "y", "x = 3"},
			{"x = 3 unless x < y", "x", infix.LT, "y", "x = 3"},
			{"x 3 unless x < y", "x", infix.LT, "y", "x(3)"},
			{"x.add 3 unless x < y", "x", infix.LT, "y", "x.add(3)"},
		}

		for _, tt := range tests {
			t.Run("expression "+tt.input, func(t *testing.T) {
				program, err := parseSource(tt.input)
				checkParserErrors(t, err)
				assert.That(t, len(program.Statements) == 1, "program has not enough statements. got=%d", len(program.Statements))

				stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
				assert.That(t, ok, "program.Statements[0] is not ast.ExpressionStatement. got=%T", program.Statements[0])

				exp, ok := stmt.Expression.(*ast.ConditionalExpression)
				assert.That(t, ok, "stmt.Expression is not %T. got=%T", stmt.Expression)

				testInfixExpression(
					t,
					exp.Condition,
					tt.expectedConditionLeft,
					tt.expectedConditionOperator,
					tt.expectedConditionRight,
				)

				consequenceBody := ""
				for _, stmt := range exp.Consequence.Statements {
					consequence, ok := stmt.(*ast.ExpressionStatement)
					assert.That(t, ok, "Statements[0] is not ast.ExpressionStatement. got=%T", exp.Consequence.Statements[0])

					consequenceBody += consequence.Expression.Code()
				}

				assert.Equal(t, consequenceBody, tt.expectedConsequenceExpression)
				// assert.Equal(t, exp.Alternative, nil)
			})
		}
	})
	t.Run("with method call expression", func(t *testing.T) {
		tests := []struct {
			input       string
			condContext string
			condMethod  string
			condArg     string
			consequence string
		}{
			{`unless x.exist? :y
			x
			end`, "x", "exist?", ":y", "x"},
			{`unless x.exist? :y
			x = Object x
			end`, "x", "exist?", ":y", "x = Object(x)"},
			{`unless x.exist? :y
			x
			end`, "x", "exist?", ":y", "x"},
			{`unless x.exist? :y
			x = Object x
			end`, "x", "exist?", ":y", "x = Object(x)"},
		}

		for _, tt := range tests {
			program, err := parseSource(tt.input)
			checkParserErrors(t, err)
			assert.Equal(t, len(program.Statements), 1)

			stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
			assert.That(t, ok, "program.Statements[0] is not ast.ExpressionStatement. got=%T", program.Statements[0])

			exp, ok := stmt.Expression.(*ast.ConditionalExpression)
			assert.That(t, ok, "stmt.Expression is not ast.ConditionalExpression. got=%T", stmt.Expression)

			call, ok := exp.Condition.(*ast.ContextCallExpression)
			assert.That(t, ok, "exp.Condition is not ast.ContextCallExpression. got=%T", exp.Condition)
			assert.Equal(t, call.Function, tt.condMethod)

			args := []string{}
			for _, a := range call.Arguments {
				args = append(args, a.Code())
			}
			assert.Equal(t, strings.Join(args, " "), tt.condArg)
			assert.Equal(t, call.Context.Code(), tt.condContext)

			consequenceBody := ""
			for _, stmt := range exp.Consequence.Statements {
				consequence, ok := stmt.(*ast.ExpressionStatement)
				assert.That(t, ok, "Statements[0] is not ast.ExpressionStatement. got=%T", exp.Consequence.Statements[0])

				consequenceBody += consequence.Expression.Code()
			}

			assert.Equal(t, consequenceBody, tt.consequence)
			assert.Equal(t, exp.Alternative, nil)
		}
	})
}

func TestConditionalExpressionWithAlternative(t *testing.T) {
	tests := []struct {
		name               string
		input              string
		condition_left     string
		condition_operator infix.Infix
		condition_right    string
		consequence        string
		alternative        string
	}{
		{
			"regular if else",
			`
			if x < y
				x
			else
				y
			end`,
			"x",
			infix.LT,
			"y",
			"x",
			"y",
		},
		{
			"ternary if",
			"x < y ? x : y;",
			"x",
			infix.LT,
			"y",
			"x",
			"y",
		},
		{
			"ternary if with symbol as consequence",
			"x < y ? :x : y;",
			"x",
			infix.LT,
			"y",
			":x",
			"y",
		},
		{
			"ternary if with symbol as alternative",
			"x < y ? x : :y;",
			"x",
			infix.LT,
			"y",
			"x",
			":y",
		},
	}

	for i, tt := range tests {
		t.Run(fmt.Sprintf("%d/%s", i, tt.name), func(t *testing.T) {
			program, err := parseSource(tt.input)
			checkParserErrors(t, err)
			assert.Equal(t, len(program.Statements), 1)

			stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
			assert.That(t, ok, "program.Statements[0] is not ast.ExpressionStatement. got=%T", program.Statements[0])

			exp, ok := stmt.Expression.(*ast.ConditionalExpression)
			assert.That(t, ok, "stmt.Expression is not ast.ConditionalExpression. got=%T", stmt.Expression)
			testInfixExpression(t, exp.Condition, tt.condition_left, tt.condition_operator, tt.condition_right)
			assert.Equal(t, len(exp.Consequence.Statements), 1)

			consequence, ok := exp.Consequence.Statements[0].(*ast.ExpressionStatement)
			assert.That(t, ok, "Statements[0] is not ast.ExpressionStatement. got=%T", exp.Consequence.Statements[0])
			testLiteralExpression(t, consequence.Expression, tt.consequence)
			assert.Equal(t, len(exp.Alternative.Statements), 1)

			alternative, ok := exp.Alternative.Statements[0].(*ast.ExpressionStatement)
			assert.That(t, ok, "Statements[0] is not ast.ExpressionStatement. got=%T", exp.Alternative.Statements[0])
			testLiteralExpression(t, alternative.Expression, tt.alternative)
		})
	}
	t.Run("ternary if with call as consequence", func(t *testing.T) {
		tt := struct {
			input              string
			condition_left     string
			condition_operator infix.Infix
			condition_right    string
			consequence        string
			alternative        string
		}{
			"x < y ? x.foo : y;",
			"x", infix.LT, "y",
			"x.foo",
			"y",
		}
		program, err := parseSource(tt.input)
		checkParserErrors(t, err)
		assert.Equal(t, len(program.Statements), 1)

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		assert.That(t, ok, "program.Statements[0] is not ast.ExpressionStatement. got=%T", program.Statements[0])

		exp, ok := stmt.Expression.(*ast.ConditionalExpression)
		assert.That(t, ok, "stmt.Expression is not ast.ConditionalExpression. got=%T", stmt.Expression)

		testInfixExpression(t, exp.Condition, tt.condition_left, tt.condition_operator, tt.condition_right)
		assert.Equal(t, len(exp.Consequence.Statements), 1)

		consequence, ok := exp.Consequence.Statements[0].(*ast.ExpressionStatement)
		assert.That(t, ok, "Statements[0] is not ast.ExpressionStatement. got=%T", exp.Consequence.Statements[0])
		assert.Equal(t, consequence.Code(), tt.consequence)
		assert.Equal(t, len(exp.Alternative.Statements), 1)

		alternative, ok := exp.Alternative.Statements[0].(*ast.ExpressionStatement)
		assert.That(t, ok, "Statements[0] is not ast.ExpressionStatement. got=%T", exp.Alternative.Statements[0])
		testLiteralExpression(t, alternative.Expression, tt.alternative)
	})
}

func TestFunctionLiteralParsing(t *testing.T) {
	type funcParam struct {
		name         string
		defaultValue interface{}
	}
	tests := []struct {
		desc          string
		input         string
		name          string
		parameters    []funcParam
		bodyStatement string
	}{
		{
			"with parens",
			`def foo(x, y)
			  x + y
          end`,
			"foo",
			[]funcParam{
				{name: "x", defaultValue: nil},
				{name: "y", defaultValue: nil},
			},
			"x + y",
		},
		{
			"without parens",
			`def bar x, y
          x + y
          end`,
			"bar",
			[]funcParam{
				{name: "x", defaultValue: nil},
				{name: "y", defaultValue: nil},
			},
			"x + y",
		},
		{
			"without arguments",
			`def qux
          x + y
          end`,
			"qux",
			[]funcParam{},
			"x + y",
		},
		{
			"expression separator semicolon no arguments",
			"def qux; x + y; end",
			"qux",
			[]funcParam{},
			"x + y",
		},
		{
			"expression separator semicolon two arguments",
			"def foo x, y; x + y; end",
			"foo",
			[]funcParam{
				{name: "x", defaultValue: nil},
				{name: "y", defaultValue: nil},
			},
			"x + y",
		},
		{
			"expression separator semicolon with parens and two arguments",
			"def foo(x, y); x + y; end",
			"foo",
			[]funcParam{
				{name: "x", defaultValue: nil},
				{name: "y", defaultValue: nil},
			},
			"x + y",
		},
		{
			"upcase function name",
			`def Qux
          x + y
          end
          `,
			"Qux",
			[]funcParam{},
			"x + y",
		},
		{
			"two arguments with defaults without parens",
			`def foo x = 2, y = 3
          x + y
          end
          `,
			"foo",
			[]funcParam{
				{name: "x", defaultValue: 2},
				{name: "y", defaultValue: 3},
			},
			"x + y",
		},
		{
			"operator as function name",
			`def <=>
          x + y
          end
          `,
			"<=>",
			[]funcParam{},
			"x + y",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			program, err := parseSource(tt.input)
			checkParserErrors(t, err)
			assert.Equal(t, len(program.Statements), 1)

			stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
			assert.That(t, ok, "program.Statements[0] is not ast.ExpressionStatement. got=%T", program.Statements[0])

			function, ok := stmt.Expression.(*ast.FunctionLiteral)
			assert.That(t, ok, "stmt.Expression is not ast.FunctionLiteral. got=%T", stmt.Expression)
			assert.Equal(t, function.Name, tt.name)
			assert.Equal(t, len(function.Parameters), len(tt.parameters))

			for i, param := range function.Parameters {
				assert.Equal(t, param.Name, tt.parameters[i].name)
				testLiteralExpression(t, param.Default, tt.parameters[i].defaultValue)
			}
			assert.Equal(t, len(function.Body.Statements), 1)

			bodyStmt, ok := function.Body.Statements[0].(*ast.ExpressionStatement)
			assert.That(t, ok, "function body stmt is not ast.ExpressionStatement. got=%T", function.Body.Statements[0])
			assert.Equal(t, bodyStmt.Code(), tt.bodyStatement)
		})
	}
}

func TestBlockExpressionParsing(t *testing.T) {
	tests := []struct {
		input         string
		parameters    []string
		bodyStatement string
	}{
		{
			`method { |x, y|
			  x + y
			  }`,
			[]string{"x", "y"},
			"x + y",
		},
		{
			`method {
          x + y
          }`,
			[]string{},
			"x + y",
		},
		{
			"method { x + y; }",
			[]string{},
			"x + y",
		},
		{
			"method { |x, y|; x + y; }",
			[]string{"x", "y"},
			"x + y",
		},
		{
			"method { |x, y|; x + y; }",
			[]string{"x", "y"},
			"x + y",
		},
		{
			"method { |x, y|; x.add y }",
			[]string{"x", "y"},
			"x.add(y)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			program, err := parseSource(tt.input)
			checkParserErrors(t, err)
			assert.Equal(t, len(program.Statements), 1)

			stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
			assert.That(t, ok, "program.Statements[0] is not ast.ExpressionStatement. got=%T", program.Statements[0])

			call, ok := stmt.Expression.(*ast.ContextCallExpression)
			assert.That(t, ok, "stmt.Expression is not *ast.ContextCallExpression. got=%T", stmt.Expression)

			block := assert.Type[*ast.FunctionLiteral](t, call.Block, "*ast.ContextCallExpression.Block is not *ast.FunctionLiteral. got=%T", call.Block)
			assert.NotEqual(t, block, nil)
			assert.Equal(t, len(block.Parameters), len(tt.parameters))

			for i, param := range block.Parameters {
				assert.Equal(t, param.Name, tt.parameters[i])
			}
			assert.Equal(t, len(block.Body.Statements), 1)

			bodyStmt, ok := block.Body.Statements[0].(*ast.ExpressionStatement)
			assert.That(t, ok, "block body stmt is not ast.ExpressionStatement. got=%T", block.Body.Statements[0])
			assert.Equal(t, bodyStmt.Code(), tt.bodyStatement)
		})
	}
}

func TestFunctionParameterParsing(t *testing.T) {
	type funcParam struct {
		name         string
		defaultValue interface{}
	}
	tests := []struct {
		desc           string
		input          string
		expectedParams []funcParam
	}{
		{
			desc:           "no params with parens",
			input:          "def fn(); end",
			expectedParams: []funcParam{},
		},
		{
			desc:           "one param with parens",
			input:          "def fn(x); end",
			expectedParams: []funcParam{{name: "x"}},
		},
		{
			desc:           "multiple params with parens",
			input:          "def fn(x, y, z); end",
			expectedParams: []funcParam{{name: "x"}, {name: "y"}, {name: "z"}},
		},
		{
			desc:           "multiple params first two defaults with parens",
			input:          "def fn(x = 3, y = 18, z); end",
			expectedParams: []funcParam{{name: "x", defaultValue: 3}, {name: "y", defaultValue: 18}, {name: "z"}},
		},
		{
			desc:           "multiple params middle default with parens",
			input:          "def fn(x, y = 18, z); end",
			expectedParams: []funcParam{{name: "x"}, {name: "y", defaultValue: 18}, {name: "z"}},
		},
		{
			desc:           "multiple params last default with parens",
			input:          "def fn(x, y, z = 1); end",
			expectedParams: []funcParam{{name: "x"}, {name: "y"}, {name: "z", defaultValue: 1}},
		},
		{
			desc:           "multiple params last array splat with parens",
			input:          "def fn(x, y, *z); end",
			expectedParams: []funcParam{{name: "x"}, {name: "y"}, {name: "z"}},
		},
		{
			desc:           "one param array splat with parens",
			input:          "def fn(*x); end",
			expectedParams: []funcParam{{name: "x"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			program, err := parseSource(tt.input)
			checkParserErrors(t, err)

			stmt := program.Statements[0].(*ast.ExpressionStatement)
			function := stmt.Expression.(*ast.FunctionLiteral)

			assert.Equal(t, len(function.Parameters), len(tt.expectedParams))

			for i, ident := range tt.expectedParams {
				assert.Equal(t, function.Parameters[i].Name, ident.name)
				testLiteralExpression(t, function.Parameters[i].Default, ident.defaultValue)
			}
		})
	}
}

func TestBlockParameterParsing(t *testing.T) {
	type funcParam struct {
		name         string
		defaultValue interface{}
	}
	tests := []struct {
		desc           string
		input          string
		expectedParams []funcParam
	}{
		{
			desc:           "empty brace block",
			input:          "method {}",
			expectedParams: []funcParam{},
		},
		{
			desc:           "empty brace block params",
			input:          "method { || }",
			expectedParams: []funcParam{},
		},
		{
			desc:           "one brace block param",
			input:          "method { |x| }",
			expectedParams: []funcParam{{name: "x"}},
		},
		{
			desc:           "multiple brace block params",
			input:          "method { |x, y, z| }",
			expectedParams: []funcParam{{name: "x"}, {name: "y"}, {name: "z"}},
		},
		{
			desc:           "multiple brace block params with defaults",
			input:          "method { |x = 3, y = 2, z| }",
			expectedParams: []funcParam{{name: "x", defaultValue: 3}, {name: "y", defaultValue: 2}, {name: "z"}},
		},
		{
			desc:           "multiple brace block params with middle default",
			input:          "method { |x, y = 2, z| }",
			expectedParams: []funcParam{{name: "x"}, {name: "y", defaultValue: 2}, {name: "z"}},
		},
		{
			desc:           "multiple brace block params last defaults",
			input:          "method { |x, y, z = 2| }",
			expectedParams: []funcParam{{name: "x"}, {name: "y"}, {name: "z", defaultValue: 2}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			program, err := parseSource(tt.input)
			checkParserErrors(t, err)

			stmt := program.Statements[0].(*ast.ExpressionStatement)
			call, ok := stmt.Expression.(*ast.ContextCallExpression)
			assert.That(t, ok, "stmt.Expression is not *ast.ContextCallExpression. got=%T", stmt.Expression)

			block := assert.Type[*ast.FunctionLiteral](t, call.Block, "*ast.ContextCallExpression.Block is not *ast.FunctionLiteral. got=%T", call.Block)
			assert.NotEqual(t, block, nil)
			assert.Equal(t, len(block.Parameters), len(tt.expectedParams))

			for i, ident := range tt.expectedParams {
				assert.Equal(t, block.Parameters[i].Name, ident.name)
				testLiteralExpression(t, block.Parameters[i].Default, ident.defaultValue)
			}
		})
	}
}

func TestCallExpressionParsing(t *testing.T) {
	testCases := []struct {
		desc        string
		input       string
		context     string
		funcName    string
		arguments   []interface{}
		hasBlock    bool
		blockParams []string
	}{
		{
			desc:     "with parens",
			input:    "add(1, 2 * 3, 4 + 5);",
			funcName: "add",
			arguments: []interface{}{
				1, testInfix{2, infix.ASTERISK, 3}, testInfix{4, infix.PLUS, 5},
			},
		},
		{
			desc:     "without parens",
			input:    "add 1, 2 * 3, 4 + 5;",
			funcName: "add",
			arguments: []interface{}{
				1, testInfix{2, infix.ASTERISK, 3}, testInfix{4, infix.PLUS, 5},
			},
		},
		{
			desc:     "with parens and brace block",
			input:    "add(1, 2 * 3, 4 + 5) { |x| x };",
			funcName: "add",
			arguments: []interface{}{
				1, testInfix{2, infix.ASTERISK, 3}, testInfix{4, infix.PLUS, 5},
			},
			hasBlock:    true,
			blockParams: []string{"x"},
		},
		{
			desc:     "without parens with block",
			input:    "add 1, 2 * 3, 4 + 5 { |x| x };",
			funcName: "add",
			arguments: []interface{}{
				1, testInfix{2, infix.ASTERISK, 3}, testInfix{4, infix.PLUS, 5},
			},
			hasBlock:    true,
			blockParams: []string{"x"},
		},
		{
			desc:        "without parens without args with block",
			input:       "add { |x| x };",
			funcName:    "add",
			hasBlock:    true,
			blockParams: []string{"x"},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.desc, func(t *testing.T) {
			expr, err := parseExpression(tt.input)
			checkParserErrors(t, err)

			call, ok := expr.(*ast.ContextCallExpression)
			assert.That(t, ok, "expression is not ast.ContextCallExpression. got=%T", expr)

			assert.Equal(t, call.Function, tt.funcName)

			if tt.context != "" {
				testIdentifier(t, call.Context, tt.context)
			}

			assert.Equal(t, len(call.Arguments), len(tt.arguments))

			for i, arg := range call.Arguments {
				testExpression(t, arg, tt.arguments[i])
			}

			if tt.hasBlock {
				block := assert.Type[*ast.FunctionLiteral](t, call.Block, "*ast.ContextCallExpression.Block is not *ast.FunctionLiteral. got=%T", call.Block)
				assert.NotEqual(t, block, nil)
				assert.Equal(t, len(block.Parameters), len(tt.blockParams))
				for i, param := range block.Parameters {
					assert.Equal(t, param.Code(), tt.blockParams[i])
				}
			}
		})
	}
	t.Run("with parens and do block", func(t *testing.T) {
	})
	t.Run("without parens with block", func(t *testing.T) {
	})
	t.Run("without parens", func(t *testing.T) {
	})
	t.Run("without parens without args with block", func(t *testing.T) {
	})
}

func TestCallExpressionWithoutParens(t *testing.T) {
	tests := []struct {
		input         string
		expectedIdent string
	}{
		{
			input:         "puts([1][0])",
			expectedIdent: "puts",
		},
		{
			input:         "puts [1][0]",
			expectedIdent: "puts",
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			program, err := parseSource(tt.input)
			checkParserErrors(t, err)

			stmt := program.Statements[0].(*ast.ExpressionStatement)
			exp, ok := stmt.Expression.(*ast.ContextCallExpression)
			assert.That(t, ok, "stmt.Expression is not ast.ContextCallExpression. got=%T", stmt.Expression)
			assert.NotEqual(t, stmt.Expression, nil)
			assert.Equal(t, stmt.Expression.Code(), "puts([1][0])")
			assert.Equal(t, exp.Function, tt.expectedIdent)
		})
	}
}
func TestCallExpressionParameterParsing(t *testing.T) {
	tests := []struct {
		input         string
		expectedIdent string
		expectedArgs  []string
	}{
		{
			input:         "add();",
			expectedIdent: "add",
			expectedArgs:  []string{},
		},
		{
			input:         "add(1);",
			expectedIdent: "add",
			expectedArgs:  []string{"1"},
		},
		{
			input:         "add(1, 2 * 3, 4 + 5);",
			expectedIdent: "add",
			expectedArgs:  []string{"1", "2 * 3", "4 + 5"},
		},
		{
			input:         "add 1;",
			expectedIdent: "add",
			expectedArgs:  []string{"1"},
		},
		{
			input:         `add "foo";`,
			expectedIdent: "add",
			expectedArgs:  []string{"\"foo\""},
		},
		{
			input:         `add :foo;`,
			expectedIdent: "add",
			expectedArgs:  []string{":foo"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			program, err := parseSource(tt.input)
			checkParserErrors(t, err)

			stmt := program.Statements[0].(*ast.ExpressionStatement)
			exp, ok := stmt.Expression.(*ast.ContextCallExpression)
			assert.That(t, ok, "stmt.Expression is not ast.ContextCallExpression. got=%T", stmt.Expression)

			assert.Equal(t, exp.Function, tt.expectedIdent)
			assert.Equal(t, len(exp.Arguments), len(tt.expectedArgs))

			for i, arg := range tt.expectedArgs {
				assert.Equal(t, exp.Arguments[i].Code(), arg)
			}
		})
	}
}

func TestContextCallExpression(t *testing.T) {
	t.Run("context call with multiple args with parens", func(t *testing.T) {
		input := "foo.add(1, 2 * 3, 4 + 5);"

		program, err := parseSource(input)
		checkParserErrors(t, err)
		assert.Equal(t, len(program.Statements), 1)

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		assert.That(t, ok, "stmt is not ast.ExpressionStatement. got=%T", program.Statements[0])

		exp, ok := stmt.Expression.(*ast.ContextCallExpression)
		assert.That(t, ok, "stmt.Expression is not ast.ContextCallExpression. got=%T", stmt.Expression)
		testIdentifier(t, exp.Context, "foo")
		assert.Equal(t, exp.Function, "add")
		assert.Equal(t, len(exp.Arguments), 3)
		testLiteralExpression(t, exp.Arguments[0], 1)
		testInfixExpression(t, exp.Arguments[1], 2, infix.ASTERISK, 3)
		testInfixExpression(t, exp.Arguments[2], 4, infix.PLUS, 5)
	})
	t.Run("context call with multiple args with parens and block", func(t *testing.T) {
		input := "foo.add(1, 2 * 3, 4 + 5) { |x|x.to_s };"

		program, err := parseSource(input)
		checkParserErrors(t, err)
		assert.Equal(t, len(program.Statements), 1)

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		assert.That(t, ok, "stmt is not ast.ExpressionStatement. got=%T", program.Statements[0])

		exp, ok := stmt.Expression.(*ast.ContextCallExpression)
		assert.That(t, ok, "stmt.Expression is not ast.ContextCallExpression. got=%T", stmt.Expression)
		testIdentifier(t, exp.Context, "foo")
		assert.Equal(t, exp.Function, "add")
		assert.Equal(t, len(exp.Arguments), 3)
		testLiteralExpression(t, exp.Arguments[0], 1)
		testInfixExpression(t, exp.Arguments[1], 2, infix.ASTERISK, 3)
		testInfixExpression(t, exp.Arguments[2], 4, infix.PLUS, 5)
		assert.NotEqual(t, exp.Block, nil)
	})
	t.Run("context call with multiple args no parens", func(t *testing.T) {
		input := "foo.add 1, 2 * 3, 4 + 5;"

		program, err := parseSource(input)
		checkParserErrors(t, err)
		assert.Equal(t, len(program.Statements), 1)

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		assert.That(t, ok, "stmt is not ast.ExpressionStatement. got=%T", program.Statements[0])

		exp, ok := stmt.Expression.(*ast.ContextCallExpression)
		assert.That(t, ok, "stmt.Expression is not ast.ContextCallExpression. got=%T", stmt.Expression)
		testIdentifier(t, exp.Context, "foo")
		assert.Equal(t, exp.Function, "add")
		assert.Equal(t, len(exp.Arguments), 3)
		testLiteralExpression(t, exp.Arguments[0], 1)
		testInfixExpression(t, exp.Arguments[1], 2, infix.ASTERISK, 3)
		testInfixExpression(t, exp.Arguments[2], 4, infix.PLUS, 5)
	})
	t.Run("context call with multiple args no parens with block", func(t *testing.T) {
		input := "foo.add 1, 2 * 3, 4 + 5 { |x| x };"

		program, err := parseSource(input)
		checkParserErrors(t, err)
		assert.Equal(t, len(program.Statements), 1)

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		assert.That(t, ok, "stmt is not ast.ExpressionStatement. got=%T", program.Statements[0])

		exp, ok := stmt.Expression.(*ast.ContextCallExpression)
		assert.That(t, ok, "stmt.Expression is not ast.ContextCallExpression. got=%T", stmt.Expression)
		testIdentifier(t, exp.Context, "foo")
		assert.Equal(t, exp.Function, "add")
		assert.Equal(t, len(exp.Arguments), 3)
		testLiteralExpression(t, exp.Arguments[0], 1)
		testInfixExpression(t, exp.Arguments[1], 2, infix.ASTERISK, 3)
		testInfixExpression(t, exp.Arguments[2], 4, infix.PLUS, 5)
		assert.NotEqual(t, exp.Block, nil)
	})
	t.Run("context call with no args", func(t *testing.T) {
		input := "foo.add;"

		program, err := parseSource(input)
		checkParserErrors(t, err)
		assert.Equal(t, len(program.Statements), 1)

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		assert.That(t, ok, "stmt is not ast.ExpressionStatement. got=%T", program.Statements[0])

		exp, ok := stmt.Expression.(*ast.ContextCallExpression)
		assert.That(t, ok, "stmt.Expression is not ast.ContextCallExpression. got=%T", stmt.Expression)
		testIdentifier(t, exp.Context, "foo")
		assert.Equal(t, exp.Function, "add")
		assert.Equal(t, len(exp.Arguments), 0)
	})
	t.Run("context call on nonident with no dot", func(t *testing.T) {
		input := "1 add;"

		program, err := parseSource(input)
		checkParserErrors(t, err)
		assert.Equal(t, len(program.Statements), 1)

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		assert.That(t, ok, "stmt is not ast.ExpressionStatement. got=%T", program.Statements[0])

		exp, ok := stmt.Expression.(*ast.ContextCallExpression)
		assert.That(t, ok, "stmt.Expression is not ast.ContextCallExpression. got=%T", stmt.Expression)
		testIntegerLiteral(t, exp.Context, 1)
		assert.Equal(t, exp.Function, "add")
		assert.Equal(t, len(exp.Arguments), 0)
	})
	t.Run("context call on nonident with dot", func(t *testing.T) {
		input := "1.add"

		program, err := parseSource(input)
		checkParserErrors(t, err)
		assert.Equal(t, len(program.Statements), 1)

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		assert.That(t, ok, "stmt is not ast.ExpressionStatement. got=%T", program.Statements[0])

		exp, ok := stmt.Expression.(*ast.ContextCallExpression)
		assert.That(t, ok, "stmt.Expression is not ast.ContextCallExpression. got=%T", stmt.Expression)
		testIntegerLiteral(t, exp.Context, 1)
		assert.Equal(t, exp.Function, "add")
		assert.Equal(t, len(exp.Arguments), 0)
	})
	t.Run("context call on nonident with no dot multiargs", func(t *testing.T) {
		input := "1 add 1"

		program, err := parseSource(input)
		checkParserErrors(t, err)
		assert.Equal(t, len(program.Statements), 1)

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		assert.That(t, ok, "stmt is not ast.ExpressionStatement. got=%T", program.Statements[0])

		exp, ok := stmt.Expression.(*ast.ContextCallExpression)
		assert.That(t, ok, "stmt.Expression is not ast.ContextCallExpression. got=%T", stmt.Expression)
		testIntegerLiteral(t, exp.Context, 1)
		assert.Equal(t, exp.Function, "add")
		assert.Equal(t, len(exp.Arguments), 1)
		testIntegerLiteral(t, exp.Arguments[0], 1)
	})
	t.Run("context call on ident with no dot", func(t *testing.T) {
		input := "foo add;"

		program, err := parseSource(input)
		checkParserErrors(t, err)
		assert.Equal(t, len(program.Statements), 1)

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		assert.That(t, ok, "stmt is not ast.ExpressionStatement. got=%T", program.Statements[0])

		exp, ok := stmt.Expression.(*ast.ContextCallExpression)
		assert.That(t, ok, "stmt.Expression is not ast.ContextCallExpression. got=%T", stmt.Expression)
		assert.Equal(t, exp.Function, "foo")
		assert.Equal(t, len(exp.Arguments), 1)
		testIdentifier(t, exp.Arguments[0], "add")
	})
	t.Run("context call on const with no dot", func(t *testing.T) {
		input := "Integer add;"

		program, err := parseSource(input)
		checkParserErrors(t, err)
		assert.Equal(t, len(program.Statements), 1)

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		assert.That(t, ok, "stmt is not ast.ExpressionStatement. got=%T", program.Statements[0])

		exp, ok := stmt.Expression.(*ast.ContextCallExpression)
		assert.That(t, ok, "stmt.Expression is not ast.ContextCallExpression. got=%T", stmt.Expression)
		assert.Equal(t, exp.Function, "Integer")
		assert.Equal(t, len(exp.Arguments), 1)
		testIdentifier(t, exp.Arguments[0], "add")
	})
	t.Run("context call on ident with no dot Const as arg", func(t *testing.T) {
		input := "add Integer;"

		program, err := parseSource(input)
		checkParserErrors(t, err)
		assert.Equal(t, len(program.Statements), 1)

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		assert.That(t, ok, "stmt is not ast.ExpressionStatement. got=%T", program.Statements[0])

		exp, ok := stmt.Expression.(*ast.ContextCallExpression)
		assert.That(t, ok, "stmt.Expression is not ast.ContextCallExpression. got=%T", stmt.Expression)

		assert.Equal(t, exp.Function, "add")
		assert.Equal(t, len(exp.Arguments), 1)

		testIdentifier(t, exp.Arguments[0], "Integer")
	})
	t.Run("chained context call with dot without parens", func(t *testing.T) {
		input := "foo.add.bar;"

		program, err := parseSource(input)
		checkParserErrors(t, err)
		assert.Equal(t, len(program.Statements), 1)

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		assert.That(t, ok, "stmt is not ast.ExpressionStatement. got=%T", program.Statements[0])

		exp, ok := stmt.Expression.(*ast.ContextCallExpression)
		assert.That(t, ok, "stmt.Expression is not ast.ContextCallExpression. got=%T", stmt.Expression)

		context, ok := exp.Context.(*ast.ContextCallExpression)
		assert.That(t, ok, "expr.Context is not ast.ContextCallExpression. got=%T", exp.Context)

		testIdentifier(t, context.Context, "foo")
		assert.Equal(t, context.Function, "add")
		assert.Equal(t, len(context.Arguments), 0)

		assert.Equal(t, exp.Function, "bar")
		assert.Equal(t, len(exp.Arguments), 0)
	})
	t.Run("chained context call with dot without parens", func(t *testing.T) {
		input := "1.add.bar;"

		program, err := parseSource(input)
		checkParserErrors(t, err)
		assert.Equal(t, len(program.Statements), 1)

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		assert.That(t, ok, "stmt is not ast.ExpressionStatement. got=%T", program.Statements[0])

		exp, ok := stmt.Expression.(*ast.ContextCallExpression)
		assert.That(t, ok, "stmt.Expression is not ast.ContextCallExpression. got=%T", stmt.Expression)

		context, ok := exp.Context.(*ast.ContextCallExpression)
		assert.That(t, ok, "expr.Context is not ast.ContextCallExpression. got=%T", exp.Context)

		testIntegerLiteral(t, context.Context, 1)
		assert.Equal(t, context.Function, "add")
		assert.Equal(t, len(context.Arguments), 0)
		assert.Equal(t, exp.Function, "bar")
		assert.Equal(t, len(exp.Arguments), 0)
	})
	t.Run("chained context call with dot with parens", func(t *testing.T) {
		input := "foo.add().bar();"

		program, err := parseSource(input)
		checkParserErrors(t, err)
		assert.Equal(t, len(program.Statements), 1)

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		assert.That(t, ok, "stmt is not ast.ExpressionStatement. got=%T", program.Statements[0])

		exp, ok := stmt.Expression.(*ast.ContextCallExpression)
		assert.That(t, ok, "stmt.Expression is not ast.ContextCallExpression. got=%T", stmt.Expression)

		context, ok := exp.Context.(*ast.ContextCallExpression)
		assert.That(t, ok, "expr.Context is not ast.ContextCallExpression. got=%T", exp.Context)
		assert.Equal(t, context.Function, "add")
		assert.Equal(t, len(context.Arguments), 0)
		assert.Equal(t, exp.Function, "bar")
		assert.Equal(t, len(exp.Arguments), 0)
	})
	t.Run("allow operators as method name", func(t *testing.T) {
		input := "foo.<=>;"

		program, err := parseSource(input)
		checkParserErrors(t, err)
		assert.Equal(t, len(program.Statements), 1)

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		assert.That(t, ok, "stmt is not ast.ExpressionStatement. got=%T", program.Statements[0])

		expr, ok := stmt.Expression.(*ast.ContextCallExpression)
		assert.That(t, ok, "stmt.Expression is not ast.ContextCallExpression. got=%T", stmt.Expression)
		testIdentifier(t, expr.Context, "foo")
		assert.Equal(t, expr.Function, "<=>")
	})
}

func TestStringLiteralExpression(t *testing.T) {
	input := `"hello world";`

	program, err := parseSource(input)
	checkParserErrors(t, err)

	stmt := program.Statements[0].(*ast.ExpressionStatement)
	literal, ok := stmt.Expression.(*ast.StringLiteral)
	assert.That(t, ok, "stmt.Expression is not ast.StringLiteral. got=%T", stmt.Expression)
	assert.Equal(t, literal.Value, "hello world")
}

func TestSymbolExpression(t *testing.T) {
	tests := []struct {
		input string
		value string
	}{
		{
			`:symbol;`,
			"symbol",
		},
		{
			`:UNDEF`,
			"UNDEF",
		},
	}

	for _, tt := range tests {
		program, err := parseSource(tt.input)
		checkParserErrors(t, err)

		stmt := program.Statements[0].(*ast.ExpressionStatement)
		literal, ok := stmt.Expression.(*ast.SymbolLiteral)
		assert.That(t, ok, "stmt.Expression is not ast.SymbolLiteral. got=%T", stmt.Expression)
		assert.Equal(t, literal.Value, tt.value)
	}
}

func TestParsingArrayLiterals(t *testing.T) {
	input := "[1, 2 * 2, 3 + 3, {'foo'=>2}]"
	program, err := parseSource(input)
	checkParserErrors(t, err)
	assert.Equal(t, len(program.Statements), 1)

	stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
	assert.That(t, ok, "stmt not ast.ExpressionStatement. got=%T", program.Statements[0])

	array, ok := stmt.Expression.(*ast.ArrayLiteral)
	assert.That(t, ok, "stmt.Expression is not ast.ArrayLiteral. got=%T", stmt.Expression)
	assert.Equal(t, len(array.Elements), 4)
	testIntegerLiteral(t, array.Elements[0], 1)
	testInfixExpression(t, array.Elements[1], 2, infix.ASTERISK, 2)
	testInfixExpression(t, array.Elements[2], 3, infix.PLUS, 3)
	testHashLiteral(t, array.Elements[3], map[string]string{"\"foo\"": "2"})
}

func TestParsingIndexExpressions(t *testing.T) {
	t.Run("one arg as index", func(t *testing.T) {
		input := "myArray[1 + 1]"
		program, err := parseSource(input)
		checkParserErrors(t, err)

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		assert.That(t, ok, "stmt not ast.ExpressionStatement. got=%T", program.Statements[0])
		indexExp, ok := stmt.Expression.(*ast.IndexExpression)
		assert.That(t, ok, "stmt.Expression is not ast.IndexExpression. got=%T", stmt.Expression)
		testIdentifier(t, indexExp.Left, "myArray")
		testInfixExpression(t, indexExp.Index, 1, infix.PLUS, 1)
	})
	t.Run("two args as index", func(t *testing.T) {
		t.Run("integers", func(t *testing.T) {
			input := "myArray[1, 1]"
			program, err := parseSource(input)
			checkParserErrors(t, err)

			stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
			assert.That(t, ok, "stmt not ast.ExpressionStatement. got=%T", program.Statements[0])
			indexExp, ok := stmt.Expression.(*ast.IndexExpression)
			assert.That(t, ok, "stmt.Expression is not ast.IndexExpression. got=%T", stmt.Expression)

			testIdentifier(t, indexExp.Left, "myArray")

			index, ok := indexExp.Index.(ast.ExpressionList)
			assert.That(t, ok, "indexExp.Index not ast.ExpressionList. got=%T", indexExp.Index)

			assert.Equal(t, len(index), 2)

			i0, ok := index[0].(*ast.IntegerLiteral)
			assert.That(t, ok, "indexExp.Index[0] not ast.IntegerLiteral. got=%T", index[0])

			assert.Equal(t, i0.Value, 1)

			i1, ok := index[1].(*ast.IntegerLiteral)
			assert.That(t, ok, "indexExp.Index[1] not ast.IntegerLiteral. got=%T", index[1])
			assert.Equal(t, i1.Value, 1)
		})
		t.Run("method calls as index", func(t *testing.T) {
			input := "myArray[foo.bar, 1]"
			program, err := parseSource(input)
			checkParserErrors(t, err)

			stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
			assert.That(t, ok, "stmt not ast.ExpressionStatement. got=%T", program.Statements[0])

			indexExp, ok := stmt.Expression.(*ast.IndexExpression)
			assert.That(t, ok, "stmt.Expression is not ast.IndexExpression. got=%T", stmt.Expression)
			testIdentifier(t, indexExp.Left, "myArray")
			assert.Equal(t, indexExp.Index.Code(), "foo.bar, 1")
		})
		t.Run("method calls as length", func(t *testing.T) {
			input := "myArray[1, foo.bar]"
			program, err := parseSource(input)
			checkParserErrors(t, err)

			stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
			assert.That(t, ok, "stmt not ast.ExpressionStatement. got=%T", program.Statements[0])

			indexExp, ok := stmt.Expression.(*ast.IndexExpression)
			assert.That(t, ok, "stmt.Expression is not ast.IndexExpression. got=%T", stmt.Expression)
			testIdentifier(t, indexExp.Left, "myArray")
			assert.Equal(t, indexExp.Index.Code(), "1, foo.bar")
		})
	})
}

func TestParseHash(t *testing.T) {
	tests := []struct {
		input   string
		hashMap map[string]string
	}{
		{
			input:   `{"foo" => 42}`,
			hashMap: map[string]string{"\"foo\"": "42"},
		},
		{
			input:   `{"foo" => 42, "bar" => "baz"}`,
			hashMap: map[string]string{"\"foo\"": "42", "\"bar\"": "\"baz\""},
		},
		{
			input: `{
				"foo" => 42,
			}`,
			hashMap: map[string]string{"\"foo\"": "42"},
		},
		{
			input: `{
				# "foo" => 42,
			}`,
			hashMap: map[string]string{},
		},
		{
			input: `{
			# comment
				"foo" => 42,
			}`,
			hashMap: map[string]string{"\"foo\"": "42"},
		},
		{
			input: `{
			"foo" => 42,
				# comment
			}`,
			hashMap: map[string]string{"\"foo\"": "42"},
		},
		{
			input: `{ # comment
				"foo" => 42,
			}`,
			hashMap: map[string]string{"\"foo\"": "42"},
		},
		{
			input: `{
				"foo" => 42,
				"bar" => "baz"
			}`,
			hashMap: map[string]string{"\"foo\"": "42", "\"bar\"": "\"baz\""},
		},
	}

	for _, tt := range tests {
		program, err := parseSource(tt.input)
		// program, err := parseSource(tt.input, p.Trace)
		checkParserErrors(t, err)

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		assert.That(t, ok, "stmt is not ast.ExpressionStatement. got=%T", program.Statements[0])

		testHashLiteral(t, stmt.Expression, tt.hashMap)
	}
}

func TestRangeLiteral(t *testing.T) {
	tests := []struct {
		input     string
		ranges    [2]int
		inclusive bool
	}{
		{
			input:     `1..10`,
			ranges:    [2]int{1, 10},
			inclusive: true,
		},
		{
			input:     `1...10`,
			ranges:    [2]int{1, 10},
			inclusive: false,
		},
	}

	for _, tt := range tests {
		// program, err := parseSource(tt.input, p.Trace)
		program, err := parseSource(tt.input)
		checkParserErrors(t, err)

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		assert.That(t, ok, "stmt is not ast.ExpressionStatement. got=%T", stmt)
		testRangeLiteral(t, stmt.Expression, tt.ranges[0], tt.ranges[1], tt.inclusive)
	}
}

func TestRegexLiteral(t *testing.T) {
	tests := []struct {
		input     string
		expected  string
		modifiers string
	}{
		{
			input:    "/foo/",
			expected: "foo",
		},
	}

	for _, tt := range tests {
		program, err := parseSource(tt.input)
		checkParserErrors(t, err)

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		assert.That(t, ok, "stmt is not ast.ExpressionStatement. got=%T", stmt)

		regexLit, ok := stmt.Expression.(*ast.StringLiteral)
		assert.That(t, ok, "stmt.Expression is not ast.RegexLiteral. got=%T", stmt.Expression)
		assert.Equal(t, regexLit.Value, tt.expected)

	}
}

func TestArraySplat(t *testing.T) {
	type Expected struct {
		name     string
		is_splat bool
	}
	tests := []struct {
		input    string
		length   int
		expected []Expected
	}{
		{
			input:  "[*foo]",
			length: 1,
			expected: []Expected{
				{
					name:     "foo",
					is_splat: true,
				},
			},
		},
		{
			input:  "[*foo, bar]",
			length: 2,
			expected: []Expected{
				{
					name:     "foo",
					is_splat: true,
				},
				{
					name:     "bar",
					is_splat: false,
				},
			},
		},
		{
			input:  "[foo, *bar]",
			length: 2,
			expected: []Expected{
				{
					name:     "foo",
					is_splat: false,
				},
				{
					name:     "bar",
					is_splat: true,
				},
			},
		},
	}

	for _, tt := range tests {
		program, err := parseSource(tt.input)
		checkParserErrors(t, err)

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		assert.That(t, ok, "stmt is not ast.ExpressionStatement. got=%T", program.Statements[0])

		arrayLit, ok := stmt.Expression.(*ast.ArrayLiteral)
		assert.That(t, ok, "stmt.Expression is not ast.ArrayLiteral. got=%T", stmt.Expression)
		assert.Equal(t, len(arrayLit.Elements), tt.length)

		for i, expected := range tt.expected {
			element := arrayLit.Elements[i]
			if expected.is_splat {
				testSplat(t, element, &ast.Identifier{Value: expected.name})
			} else {
				testIdentifier(t, element, expected.name)
			}

		}
	}
}

func TestParsePyraRb(t *testing.T) {
	// t.Skip("Not implemented yet")
	filename := "../pyra.rb"
	file, err := os.ReadFile(filename)
	if err != nil {
		t.Skip("Skipping test, file not found:", filename)
	}

	program, err := parseSource(string(file))
	checkParserErrors(t, err)

	fmt.Printf("Parsed %d statements\n", len(program.Statements))

}

//===========================================================
//
//  ##   ##  #####  ##      #####   #####  #####     ####
//  ##   ##  ##     ##      ##  ##  ##     ##  ##   ##
//  #######  #####  ##      #####   #####  #####     ###
//  ##   ##  ##     ##      ##      ##     ##  ##      ##
//  ##   ##  #####  ######  ##      #####  ##   ##  ####
//
//===========================================================

func testRangeLiteral(
	t *testing.T,
	exp ast.Expression,
	start, end int,
	inclusive bool,
) {
	t.Helper()
	rangeLit, ok := exp.(*ast.RangeLiteral)
	assert.That(t, ok, "exp not *ast.RangeLiteral. got=%T", exp)
	testIntegerLiteral(t, rangeLit.Left, int64(start))
	testIntegerLiteral(t, rangeLit.Right, int64(end))
	assert.Equal(t, rangeLit.Inclusive, inclusive)
}

func testExpression(t *testing.T, exp ast.Expression, expected interface{}) {
	t.Helper()
	if inf, ok := expected.(testInfix); ok {
		testInfixExpression(t, exp, inf.left, inf.operator, inf.right)
	} else {
		testLiteralExpression(t, exp, expected)
	}
}

type testInfix struct {
	left     interface{}
	operator infix.Infix
	right    interface{}
}

func testInfixExpression(
	t *testing.T,
	exp ast.Expression,
	left interface{},
	operator infix.Infix,
	right interface{},
) {
	t.Helper()
	opExp, ok := exp.(*ast.InfixExpression)
	assert.That(t, ok, "exp is not ast.OperatorExpression. got=%T", exp)
	testLiteralExpression(t, opExp.Left, left)
	assert.Equal(t, opExp.Operator, operator)
	testLiteralExpression(t, opExp.Right, right)
}

func testLiteralExpression(
	t *testing.T,
	exp ast.Expression,
	expected interface{},
) {
	t.Helper()
	switch v := expected.(type) {
	case int:
		testIntegerLiteral(t, exp, int64(v))
	case int64:
		testIntegerLiteral(t, exp, v)
	case string:
		if strings.HasPrefix(v, ":") {
			testSymbol(t, exp, strings.TrimPrefix(v, ":"))
		} else {
			testIdentifier(t, exp, v)
		}
	case bool:
		testBooleanLiteral(t, exp, v)
	case map[string]string:
		testHashLiteral(t, exp, v)
	case []string:
		testArrayLiteral(t, exp, v)
	case nil:
		//
	default:
		t.Errorf("type of expression not handled. got=%T", exp)
	}
}

func testIntegerLiteral(t *testing.T, il ast.Expression, value int64) {
	t.Helper()
	if prefix, ok := il.(*ast.PrefixExpression); ok {
		_, ok := prefix.Right.(*ast.IntegerLiteral)
		assert.That(t, ok, "prefix.Right not *ast.IntegerLiteral. got=%T", prefix.Right)
		assert.Equal(t, prefix.Operator, "+")
		prefixedInt := fmt.Sprintf("%s%s", prefix.Operator, prefix.Right.Code())
		i, err := strconv.ParseInt(prefixedInt, 10, 64)
		assert.NoError(t, err)
		il = &ast.IntegerLiteral{Value: i}
	}
	integ, ok := il.(*ast.IntegerLiteral)
	assert.That(t, ok, "expression not *ast.IntegerLiteral. got=%T", il)
	assert.Equal(t, integ.Value, value)
}

func testGlobal(t *testing.T, exp ast.Expression, value string) {
	t.Helper()
	global, ok := exp.(*ast.Identifier)
	assert.That(t, ok, "exp not *ast.Global. got=%T", exp)
	assert.Equal(t, global.Value, value)
	assert.That(t, global.IsGlobal())
}

func testSymbol(t *testing.T, exp ast.Expression, value string) {
	t.Helper()
	symbol, ok := exp.(*ast.SymbolLiteral)
	assert.That(t, ok, "exp not *ast.SymbolLiteral. got=%T", exp)
	assert.Equal(t, symbol.Value, value)
}

func testIdentifier(t *testing.T, exp ast.Expression, value string) {
	t.Helper()
	ident, ok := exp.(*ast.Identifier)
	assert.That(t, ok, "exp not *ast.Identifier. got=%T", exp)
	assert.Equal(t, ident.Value, value)
}

func testSplat(t *testing.T, exp ast.Expression, value ast.Expression) {
	t.Helper()
	splat, ok := exp.(*ast.Splat)
	assert.That(t, ok, "exp not *ast.Splat. got=%T", exp)
	testIdentifier(t, splat.Value, value.Code())
}

func testBooleanLiteral(t *testing.T, exp ast.Expression, value bool) {
	t.Helper()
	bo, ok := exp.(*ast.SymbolLiteral)
	assert.That(t, ok, "exp not *ast.SymbolLiteral. got=%T", exp)

	var expected string
	if value {
		expected = "true"
	} else {
		expected = "false"
	}
	assert.Equal(t, bo.Value, expected)
}

func testArrayLiteral(t *testing.T, expr ast.Expression, value []string) {
	t.Helper()
	array, ok := expr.(*ast.ArrayLiteral)
	assert.That(t, ok, "expr not *ast.ArrayLiteral. got=%T", expr)

	assert.Equal(t, len(array.Elements), len(value))
	arr := make([]string, len(array.Elements))
	for i, v := range array.Elements {
		arr[i] = v.Code()
	}
	assert.EqualCmp(t, arr, value, compare.Arrays)
}

func testHashLiteral(t *testing.T, expr ast.Expression, value map[string]string) {
	t.Helper()
	hash := assert.Type[*ast.HashLiteral](t, expr, "expr not *ast.HashLiteral. got=%T", expr)
	hashMap := make(map[string]string)
	for k, v := range hash.Map {
		hashMap[k.Code()] = v.Code()
	}

	// if !reflect.DeepEqual(hashMap, value) {
	// 	t.Logf("Expected hash to equal\n%q\n\tgot\n%q\n", value, hashMap)
	// 	return false
	// }
	assert.EqualCmp(t, hashMap, value, compare.Maps)
}

func parseSource(src string) (*ast.Program, *p.Errors) {
	prog, err := p.ParseFile(gotoken.NewFileSet(), "", src)
	var parserErrors *p.Errors
	if err != nil {
		parserErrors = err.(*p.Errors)
	}
	return prog, parserErrors
}

func parseExpression(src string) (ast.Expression, *p.Errors) {
	expr, err := p.ParseExprFrom(gotoken.NewFileSet(), "", src)
	var parserErrors *p.Errors
	if err != nil {
		parserErrors = err.(*p.Errors)
	}
	return expr, parserErrors
}

func checkParserErrors(t *testing.T, err error, withStack ...bool) {
	t.Helper()
	if err == nil {
		return
	}
	printStack := false
	if len(withStack) != 0 {
		printStack = withStack[0]
	}
	// TODO: this breaks with nil-pointer dereference
	// parserErrors := assert.Type[*p.Errors](t, err, "Unexpected error type: %T:%v\n", err, err)
	parserErrors, ok := err.(*p.Errors)
	if parserErrors == nil {
		return
	}
	assert.That(t, ok, "Unexpected parser error: %T:%v\n", err, err)

	type stackTracer interface {
		StackTrace() errors.StackTrace
	}

	t.Errorf("parser has %d errors", len(parserErrors.Errors))
	for _, e := range parserErrors.Errors {
		t.Errorf("%v", e)
		if stackErr, ok := e.(stackTracer); ok && printStack {
			st := stackErr.StackTrace()
			fmt.Printf("Error stack:%+v\n", st[0:2]) // top two frames
		}

	}
	t.FailNow()
}
