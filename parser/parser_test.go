package parser

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/lczyk/goruby/ast"
	"github.com/lczyk/goruby/token"
	"github.com/pkg/errors"
)

var parseMode Mode = ParseComments

func TestMain(m *testing.M) {
	mode := flag.String("parser.mode", "ParseComments", "parser.mode=ParseComments")
	flag.Parse()
	var ok bool
	parseMode, ok = parseModes[*mode]
	if !ok {
		fmt.Printf("Unknown parse mode %s\n", *mode)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func TestBlockCapture(t *testing.T) {
	tests := []struct {
		desc   string
		input  string
		result ast.Expression
		err    error
	}{
		{
			desc:  "block capture in func params as only argument",
			input: "def foo &block; end",
			result: &ast.FunctionLiteral{
				Name: &ast.Identifier{Value: "foo"},
				CapturedBlock: &ast.BlockCapture{
					Name: &ast.Identifier{Value: "block"},
				},
			},
		},
		{
			desc:  "block capture in func params as last arguments",
			input: "def foo x, y, &block; end",
			result: &ast.FunctionLiteral{
				Name: &ast.Identifier{Value: "foo"},
				Parameters: []*ast.FunctionParameter{
					{Name: &ast.Identifier{Value: "x"}},
					{Name: &ast.Identifier{Value: "y"}},
				},
				CapturedBlock: &ast.BlockCapture{
					Name: &ast.Identifier{Value: "block"},
				},
			},
		},
		{
			desc:   "block capture in func params not last arguments",
			input:  "def foo x, &block, y; end",
			result: nil,
			err: &unexpectedTokenError{
				expectedTokens: []token.Type{token.NEWLINE, token.SEMICOLON},
				actualToken:    token.COMMA,
			},
		},
		{
			desc:   "block capture in func params on integer",
			input:  "def foo &2; end",
			result: nil,
			err: &unexpectedTokenError{
				expectedTokens: []token.Type{token.IDENT, token.NIL},
				actualToken:    token.INT,
				actualLiteral:  "2",
			},
		},
		{
			desc: "block capture only statement in func body",
			input: `
			def foo
				&block
			end`,
			result: &ast.FunctionLiteral{
				Name:       &ast.Identifier{Value: "foo"},
				Parameters: []*ast.FunctionParameter{},
				Body: &ast.BlockStatement{
					Statements: []ast.Statement{
						&ast.ExpressionStatement{
							Expression: &ast.BlockCapture{
								Name: &ast.Identifier{Value: "block"},
							},
						},
					},
				},
			},
		},
		{
			desc: "block capture as arg on call in func body",
			input: `
			def foo
				each &block
			end`,
			result: &ast.FunctionLiteral{
				Name:       &ast.Identifier{Value: "foo"},
				Parameters: []*ast.FunctionParameter{},
				Body: &ast.BlockStatement{
					Statements: []ast.Statement{
						&ast.ExpressionStatement{
							Expression: &ast.ContextCallExpression{
								Function: &ast.Identifier{Value: "each"},
								Arguments: []ast.Expression{
									&ast.BlockCapture{
										Name: &ast.Identifier{Value: "block"},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			expr, err := parseExpression(tt.input)
			compareFirstParserError(t, tt.err, err)

			if !ast.Equal(expr, tt.result) {
				t.Logf("Expected AST node to equal %v, got %v", tt.result, expr)
				t.Fail()
			}
		})
	}
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
			name:      "instance varibale",
			input:     `@x = 3`,
			leftType:  reflect.TypeOf(&ast.InstanceVariable{}),
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

			if len(program.Statements) != 1 {
				t.Fatalf(
					"program.Statements does not contain 1 statements. got=%d",
					len(program.Statements),
				)
			}
			stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
			if !ok {
				t.Fatalf(
					"program.Statements[0] is not ast.ExpressionStatement. got=%T",
					program.Statements[0],
				)
			}

			assign, ok := stmt.Expression.(*ast.Assignment)
			if !ok {
				t.Fatalf(
					"stmt.Expression is not *ast.Assignment. got=%T",
					stmt.Expression,
				)
			}

			{
				actual := reflect.TypeOf(assign.Left)
				if tt.leftType != actual {
					t.Fatalf(
						"assign.Left is not %v. got=%v",
						tt.leftType,
						actual,
					)
				}
			}

			{
				actual := reflect.TypeOf(assign.Right)
				if tt.rightType != actual {
					t.Fatalf(
						"assign.Right is not %v. got=%v",
						tt.rightType,
						actual,
					)
				}
			}
		})
	}
}

func TestAssignmentOperator(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		leftType      reflect.Type
		rightOperator string
	}{
		{
			name:          "-=",
			input:         `x -= 3`,
			leftType:      reflect.TypeOf(&ast.Identifier{}),
			rightOperator: "-",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			program, err := parseSource(tt.input)
			checkParserErrors(t, err)

			if len(program.Statements) != 1 {
				t.Logf(
					"program.Statements does not contain 1 statements. got=%d",
					len(program.Statements),
				)
				t.Logf(
					"program.Statements: %v",
					program.Statements,
				)
				t.FailNow()
			}
			stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
			if !ok {
				t.Fatalf(
					"program.Statements[0] is not ast.ExpressionStatement. got=%T",
					program.Statements[0],
				)
			}

			assign, ok := stmt.Expression.(*ast.Assignment)
			if !ok {
				t.Fatalf(
					"stmt.Expression is not *ast.Assignment. got=%T",
					stmt.Expression,
				)
			}

			{
				actual := reflect.TypeOf(assign.Left)
				if tt.leftType != actual {
					t.Fatalf(
						"assign.Left is not %v. got=%v",
						tt.leftType,
						actual,
					)
				}
			}

			{
				infix, ok := assign.Right.(*ast.InfixExpression)
				if !ok {
					t.Logf("Expected right assign type to be %T, got %T", infix, assign.Right)
					t.FailNow()
				}

				if infix.Operator != tt.rightOperator {
					t.Logf(
						"Expected right assign infix operator to be %q, got %q",
						tt.rightOperator,
						infix.Operator,
					)
					t.Fail()
				}
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
			{"x = 5_0;", "x", "5_0"},
			{"y = true;", "y", "true"},
			{"foobar = y;", "foobar", "y"},
			{"foobar = (12 + 2 * bar) - x;", "foobar", "(12 + 2 * bar) - x"},
		}

		for _, tt := range tests {
			program, err := parseSource(tt.input)
			checkParserErrors(t, err)

			if len(program.Statements) != 1 {
				t.Fatalf(
					"program.Statements does not contain 1 statements. got=%d",
					len(program.Statements),
				)
			}
			stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
			if !ok {
				t.Fatalf(
					"program.Statements[0] is not ast.ExpressionStatement. got=%T",
					program.Statements[0],
				)
			}

			variable, ok := stmt.Expression.(*ast.Assignment)
			if !ok {
				t.Fatalf(
					"stmt.Expression is not *ast.VariableAssignment. got=%T",
					stmt.Expression,
				)
			}

			if !testIdentifier(t, variable.Left, tt.expectedIdentifier) {
				return
			}

			val := variable.Right.String()

			if val != tt.expectedValue {
				t.Logf(
					"Expected variable value to equal %s, got %s\n",
					tt.expectedValue,
					val,
				)
				t.Fail()
			}
		}
	})
	t.Run("const assignment within function", func(t *testing.T) {
		// Ruby allows constant assignment inside methods (runtime warning, not parse error).
		tests := []struct {
			desc  string
			input string
		}{
			{
				desc: "single const assign",
				input: `
				def foo
					Ten = 10
				end`,
			},
			{
				desc: "const assign as multiassign",
				input: `
				def foo
					x, Ten = 10, 20
				end`,
			},
		}

		for _, tt := range tests {
			t.Run(tt.desc, func(t *testing.T) {
				_, errs := parseExpression(tt.input)
				if errs != nil {
					t.Logf("Expected no error, got %v", errs)
					t.Fail()
				}
			})
		}
	})
}

func TestWhileExpression(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name: "with explicit do",
			input: `
			while x < y do
				x += x
			end`,
		},
		{
			name: "without explicit do",
			input: `
			while x < y
				x += x
			end`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			program, err := parseSource(tt.input)
			checkParserErrors(t, err)

			if len(program.Statements) != 1 {
				t.Logf(
					"program.Body does not contain %d statements. got=%d\n",
					1,
					len(program.Statements),
				)
				t.Logf("%s\n", program.Statements)
				t.FailNow()
			}

			stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
			if !ok {
				t.Fatalf(
					"program.Statements[0] is not ast.ExpressionStatement. got=%T",
					program.Statements[0],
				)
			}

			exp, ok := stmt.Expression.(*ast.LoopExpression)
			if !ok {
				t.Fatalf(
					"stmt.Expression is not %T. got=%T",
					exp,
					stmt.Expression,
				)
			}
		})
	}
}

func TestGlobalAssignment(t *testing.T) {
	input := "$foo = 3"

	program, err := parseSource(input)
	checkParserErrors(t, err)

	if len(program.Statements) != 1 {
		t.Fatalf(
			"program.Statements does not contain 1 statements. got=%d",
			len(program.Statements),
		)
	}
	stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
	if !ok {
		t.Fatalf(
			"program.Statements[0] is not ast.ExpressionStatement. got=%T",
			program.Statements[0],
		)
	}

	variable, ok := stmt.Expression.(*ast.Assignment)
	if !ok {
		t.Fatalf(
			"stmt.Expression is not %T. got=%T",
			variable,
			stmt.Expression,
		)
	}

	expectedGlobal := "$foo"

	if !testGlobal(t, variable.Left, expectedGlobal) {
		return
	}

	val := variable.Right.String()

	expectedValue := "3"

	if val != expectedValue {
		t.Logf(
			"Expected variable value to equal %s, got %s\n",
			expectedValue,
			val,
		)
		t.Fail()
	}
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
			input:     "x[0], @y, $z, A = 3, 4, 5, 6;",
			variables: []string{"x[0]", "@y", "$z", "A"},
			values:    []string{"3", "4", "5", "6"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			expr, err := parseExpression(tt.input)
			checkParserErrors(t, err)

			assign, ok := expr.(*ast.Assignment)
			if !ok {
				t.Logf("Expected expression to be %T, got %T\n", assign, expr)
				t.FailNow()
			}

			left, ok := assign.Left.(ast.ExpressionList)
			if !ok {
				t.Logf("Expected left to be %T, got %T\n", left, assign.Left)
				t.FailNow()
			}

			actualVars := make([]string, len(left))
			for i, v := range left {
				actualVars[i] = v.String()
			}

			if !reflect.DeepEqual(tt.variables, actualVars) {
				t.Logf("Expected variable identifiers to equal %s, got %s\n", tt.variables, actualVars)
				t.Fail()
			}

			if !reflect.DeepEqual(strings.Join(tt.values, ", "), assign.Right.String()) {
				t.Logf("Expected variable values to equal %s, got %s\n", tt.values, assign.Right.String())
				t.Fail()
			}
		})
	}
}

func TestInstanceVariable(t *testing.T) {
	input := "@foo"

	program, err := parseSource(input)
	checkParserErrors(t, err)

	if len(program.Statements) != 1 {
		t.Fatalf(
			"program.Statements does not contain 1 statements. got=%d",
			len(program.Statements),
		)
	}

	stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
	if !ok {
		t.Fatalf(
			"program.Statements[0] is not ast.ExpressionStatement. got=%T",
			program.Statements[0],
		)
	}
	instVar, ok := stmt.Expression.(*ast.InstanceVariable)
	if !ok {
		t.Fatalf("Expression not %T. got=%T", instVar, stmt.Expression)
	}

	testLiteralExpression(t, instVar.Name, "foo")
}

func TestExceptionHandling(t *testing.T) {
	type rescue struct {
		classes   []string
		exception string
		body      string
	}
	tests := []struct {
		input   string
		body    string
		rescues []rescue
	}{
		{
			input: `
begin
end
`,
			body: "",
		},
		{
			input: `
begin
	2
end
`,
			body: "2",
		},
		{
			input: `
begin
	2
rescue
	3
end
`,
			body:    "2",
			rescues: []rescue{{body: "3"}},
		},
		{
			input: `
begin
	2
rescue Error
	3
end
`,
			body:    "2",
			rescues: []rescue{{classes: []string{"Error"}, body: "3"}},
		},
		{
			input: `
begin
	2
rescue Error, StandardError
	3
end
`,
			body:    "2",
			rescues: []rescue{{classes: []string{"Error", "StandardError"}, body: "3"}},
		},
		{
			input: `
begin
	2
rescue => e
	3
end
`,
			body:    "2",
			rescues: []rescue{{body: "3", exception: "e"}},
		},
		{
			input: `
begin
	2
rescue Error => e
	3
end
`,
			body:    "2",
			rescues: []rescue{{classes: []string{"Error"}, body: "3", exception: "e"}},
		},
	}

	for _, tt := range tests {
		program, err := parseSource(tt.input)
		checkParserErrors(t, err)

		if len(program.Statements) != 1 {
			t.Fatalf(
				"program.Statements does not contain 1 statements. got=%d",
				len(program.Statements),
			)
		}

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		if !ok {
			t.Fatalf(
				"program.Statements[0] is not ast.ExpressionStatement. got=%T",
				program.Statements[0],
			)
		}
		begin, ok := stmt.Expression.(*ast.ExceptionHandlingBlock)
		if !ok {
			t.Fatalf("Expression not %T. got=%T", begin, stmt.Expression)
		}

		body := begin.TryBody.String()
		if body != tt.body {
			t.Logf("Expected TryBody to equal\n%s\n\tgot\n%s\n", tt.body, body)
			t.Fail()
		}

		if len(begin.Rescues) != len(tt.rescues) {
			t.Logf("Expected %d rescue blocks, got %d\n", len(tt.rescues), len(begin.Rescues))
			t.Fail()
		}

		var rescues []rescue
		for _, r := range begin.Rescues {
			re := rescue{body: r.Body.String()}
			for _, ec := range r.ExceptionClasses {
				re.classes = append(re.classes, ec.String())
			}
			if r.Exception != nil {
				re.exception = r.Exception.String()
			}
			rescues = append(rescues, re)
		}

		if !reflect.DeepEqual(tt.rescues, rescues) {
			t.Logf("Expected rescues to equal\n%s\n\tgot\n%s\n", tt.rescues, rescues)
			t.Fail()
		}
	}
}

func TestReturnStatements(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectedValue interface{}
		expectError   error
	}{
		{name: "single int", input: "return 5;", expectedValue: 5},
		{name: "single bool", input: "return true;", expectedValue: true},
		{name: "single ident", input: "return foobar;", expectedValue: "foobar"},
		{name: "multi value", input: "return 3, 5, 8;", expectedValue: []string{"3", "5", "8"}},
		{name: "bare return", input: "return\n", expectedValue: nil},
		{name: "return with whitespace only", input: "return;", expectedValue: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			program, err := parseSource(tt.input)
			if tt.expectError != nil {
				compareFirstParserError(t, tt.expectError, err)
				return
			}
			checkParserErrors(t, err)

			if len(program.Statements) != 1 {
				t.Fatalf("expected 1 statement, got %d", len(program.Statements))
			}
			stmt := program.Statements[0]
			returnStmt, ok := stmt.(*ast.ReturnStatement)
			if !ok {
				t.Fatalf("stmt not *ast.ReturnStatement. got=%T", stmt)
			}
			if returnStmt.TokenLiteral() != "return" {
				t.Fatalf("returnStmt.TokenLiteral not 'return', got %q", returnStmt.TokenLiteral())
			}
			if tt.expectedValue == nil {
				if returnStmt.ReturnValue != nil {
					t.Errorf("expected nil ReturnValue, got %v", returnStmt.ReturnValue)
				}
			} else {
				if !testLiteralExpression(t, returnStmt.ReturnValue, tt.expectedValue) {
					t.Fail()
				}
			}
		})
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
				commentValue: " a comment",
			},
			{
				input:        "# a comment",
				commentValue: " a comment",
			},
			{
				input:        "# a comment;",
				commentValue: " a comment;",
			},
		}

		for _, tt := range tests {
			program, err := parseSource(tt.input)
			checkParserErrors(t, err)

			if len(program.Statements) != 1 {
				t.Fatalf(
					"program has not enough statements. got=%d",
					len(program.Statements),
				)
			}

			comment, ok := program.Statements[0].(*ast.Comment)
			if !ok {
				t.Logf("Expected program.Statements[0] to be %T, got %T\n", comment, program.Statements[0])
				t.FailNow()
			}

			if comment.Value != tt.commentValue {
				t.Logf("Expected comment value to equal %q, got %q\n", tt.commentValue, comment.Value)
				t.Fail()
			}
		}
	})
	t.Run("inline comment", func(t *testing.T) {
		tests := []struct {
			input        string
			commentValue string
		}{
			{
				input:        "foo # a comment\n",
				commentValue: " a comment",
			},
			{
				input:        "foo # a comment",
				commentValue: " a comment",
			},
			{
				input:        "foo # a comment;",
				commentValue: " a comment;",
			},
		}

		for _, tt := range tests {
			program, err := parseSource(tt.input)
			checkParserErrors(t, err)

			if len(program.Statements) != 2 {
				t.Fatalf(
					"program has not enough statements. got=%d",
					len(program.Statements),
				)
			}

			comment, ok := program.Statements[1].(*ast.Comment)
			if !ok {
				t.Logf("Expected program.Statements[1] to be %T, got %T\n", comment, program.Statements[1])
				t.FailNow()
			}

			if comment.Value != tt.commentValue {
				t.Logf("Expected comment value to equal %q, got %q\n", tt.commentValue, comment.Value)
				t.Fail()
			}
		}
	})
}

func TestIdentifierExpression(t *testing.T) {
	t.Run("local variable", func(t *testing.T) {
		input := "foobar;"

		program, err := parseSource(input)
		checkParserErrors(t, err)

		if len(program.Statements) != 1 {
			t.Fatalf(
				"program has not enough statements. got=%d",
				len(program.Statements),
			)
		}
		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		if !ok {
			t.Fatalf(
				"program.Statements[0] is not ast.ExpressionStatement. got=%T",
				program.Statements[0],
			)
		}

		ident, ok := stmt.Expression.(*ast.Identifier)
		if !ok {
			t.Fatalf("expression not *ast.Identifier. got=%T", stmt.Expression)
		}
		if ident.Value != "foobar" {
			t.Errorf("ident.Value not %s. got=%s", "foobar", ident.Value)
		}
		if ident.TokenLiteral() != "foobar" {
			t.Errorf(
				"ident.TokenLiteral not %s. got=%s", "foobar",
				ident.TokenLiteral(),
			)
		}
	})
	t.Run("constant", func(t *testing.T) {
		input := "Foobar;"

		program, err := parseSource(input)
		checkParserErrors(t, err)

		if len(program.Statements) != 1 {
			t.Fatalf(
				"program has not enough statements. got=%d",
				len(program.Statements),
			)
		}
		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		if !ok {
			t.Fatalf(
				"program.Statements[0] is not ast.ExpressionStatement. got=%T",
				program.Statements[0],
			)
		}

		ident, ok := stmt.Expression.(*ast.Identifier)
		if !ok {
			t.Fatalf("expression not *ast.Identifier. got=%T", stmt.Expression)
		}
		if ident.Value != "Foobar" {
			t.Errorf("ident.Value not %s. got=%s", "Foobar", ident.Value)
		}
		if ident.TokenLiteral() != "Foobar" {
			t.Errorf(
				"ident.TokenLiteral not %s. got=%s", "Foobar",
				ident.TokenLiteral(),
			)
		}
	})
}

func TestGlobalExpression(t *testing.T) {
	input := "$foobar;"

	program, err := parseSource(input)
	checkParserErrors(t, err)

	if len(program.Statements) != 1 {
		t.Fatalf(
			"program has not enough statements. got=%d",
			len(program.Statements),
		)
	}
	stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
	if !ok {
		t.Fatalf(
			"program.Statements[0] is not ast.ExpressionStatement. got=%T",
			program.Statements[0],
		)
	}

	global, ok := stmt.Expression.(*ast.Global)
	if !ok {
		t.Fatalf("expression not *ast.Global. got=%T", stmt.Expression)
	}
	if global.Value != "$foobar" {
		t.Errorf("ident.Value not %s. got=%s", "$foobar", global.Value)
	}
	if global.TokenLiteral() != "$foobar" {
		t.Errorf(
			"global.TokenLiteral not %s. got=%s", "$foobar",
			global.TokenLiteral(),
		)
	}
}

func TestScopedIdentifierExpression(t *testing.T) {
	input := "A::B"

	program, err := parseSource(input)
	checkParserErrors(t, err)

	if len(program.Statements) != 1 {
		t.Fatalf(
			"program has not enough statements. got=%d",
			len(program.Statements),
		)
	}
	stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
	if !ok {
		t.Fatalf(
			"program.Statements[0] is not ast.ExpressionStatement. got=%T",
			program.Statements[0],
		)
	}

	_, ok = stmt.Expression.(*ast.ScopedIdentifier)
	if !ok {
		t.Logf("Expected expression to be *ast.ScopedIdentifier, got %T", stmt.Expression)
		t.Fail()
	}
}

func TestSelfExpression(t *testing.T) {
	input := "self;"

	program, err := parseSource(input)
	checkParserErrors(t, err)

	if len(program.Statements) != 1 {
		t.Fatalf(
			"program has not enough statements. got=%d",
			len(program.Statements),
		)
	}
	stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
	if !ok {
		t.Fatalf(
			"program.Statements[0] is not ast.ExpressionStatement. got=%T",
			program.Statements[0],
		)
	}

	_, ok = stmt.Expression.(*ast.Self)
	if !ok {
		t.Fatalf("expression not *ast.Self. got=%T", stmt.Expression)
	}
}

func TestKeyword__FILE__(t *testing.T) {
	t.Run("keyword found", func(t *testing.T) {
		input := "__FILE__;"

		program, err := ParseFile("a_filename.rb", input, 0)
		checkParserErrors(t, err)

		if len(program.Statements) != 1 {
			t.Fatalf(
				"program has not enough statements. got=%d",
				len(program.Statements),
			)
		}
		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		if !ok {
			t.Fatalf(
				"program.Statements[0] is not ast.ExpressionStatement. got=%T",
				program.Statements[0],
			)
		}

		file, ok := stmt.Expression.(*ast.Keyword__FILE__)
		if !ok {
			t.Fatalf("expression not *ast.Keyword__FILE__. got=%T", stmt.Expression)
		}

		expected := "a_filename.rb"

		if expected != file.Filename {
			t.Logf("Expected filename to equal %q, got %q\n", expected, file.Filename)
			t.Fail()
		}
	})
	t.Run("assignment to keyword", func(t *testing.T) {
		input := "__FILE__ = 42;"

		_, err := parseSource(input)

		expected := "1:10: syntax error: Can't assign to __FILE__"

		parserErrors := err.errors
		if len(parserErrors) != 1 {
			t.Logf("Expected one error, got %d\n", len(parserErrors))
			t.Logf("Errors: %v\n", err)
			t.FailNow()
		}

		if expected != parserErrors[0].Error() {
			t.Logf("Expected error to equal\n%q\n\tgot\n%q\n", expected, parserErrors[0].Error())
			t.Fail()
		}

	})
}

func TestYieldExpression(t *testing.T) {
	tests := []struct {
		input         string
		expectedIdent string
		expectedArgs  []string
	}{
		{
			input:        "yield;",
			expectedArgs: []string{},
		},
		{
			input:        "yield 1, 2 + 3;",
			expectedArgs: []string{"1", "2 + 3"},
		},
		{
			input:        "yield(1, 2 + 3);",
			expectedArgs: []string{"1", "2 + 3"},
		},
	}

	for _, tt := range tests {
		program, err := parseSource(tt.input)
		checkParserErrors(t, err)

		if len(program.Statements) != 1 {
			t.Fatalf(
				"program has not enough statements. got=%d",
				len(program.Statements),
			)
		}
		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		if !ok {
			t.Fatalf(
				"program.Statements[0] is not ast.ExpressionStatement. got=%T",
				program.Statements[0],
			)
		}

		yield, ok := stmt.Expression.(*ast.YieldExpression)
		if !ok {
			t.Fatalf("expression not *ast.YieldExpression. got=%T", stmt.Expression)
		}

		if len(yield.Arguments) != len(tt.expectedArgs) {
			t.Logf("Expected %d arguments, got %d", len(tt.expectedArgs), len(yield.Arguments))
			t.Fail()
		}

		actualArgs := make([]string, len(yield.Arguments))
		for i, arg := range yield.Arguments {
			actualArgs[i] = arg.String()
		}

		if !reflect.DeepEqual(tt.expectedArgs, actualArgs) {
			t.Logf("Expected arguments to equal\n%v\n\tgot\n%v\n", tt.expectedArgs, actualArgs)
			t.Fail()
		}
	}
}

func TestIntegerLiteralExpression(t *testing.T) {
	tests := []struct {
		input       string
		expectValue int64
		expectBig   bool
	}{
		{input: "5", expectValue: 5},
		{input: "0x1A", expectValue: 26},
		{input: "0XFF", expectValue: 255},
		{input: "0b101", expectValue: 5},
		{input: "0B111", expectValue: 7},
		{input: "0o77", expectValue: 63},
		{input: "0O77", expectValue: 63},
		{input: "0d99", expectValue: 99},
		{input: "0D99", expectValue: 99},
		{input: "09", expectValue: 9},
		{input: "1_000", expectValue: 1000},
		{input: "42r", expectValue: 42},
		{input: "42i", expectValue: 42},
		{input: "0xFFFFFFFFFFFFFFFF", expectBig: true},
		{input: "9223372036854775808", expectBig: true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			program, err := parseSource(tt.input)
			checkParserErrors(t, err)

			if len(program.Statements) != 1 {
				t.Fatalf("expected 1 statement, got %d", len(program.Statements))
			}
			stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
			if !ok {
				t.Fatalf("expected *ast.ExpressionStatement, got %T", program.Statements[0])
			}
			literal, ok := stmt.Expression.(*ast.IntegerLiteral)
			if !ok {
				t.Fatalf("expected *ast.IntegerLiteral, got %T", stmt.Expression)
			}
			if tt.expectBig {
				if literal.BigInt == nil {
					t.Errorf("expected BigInt to be set")
				}
			} else {
				if literal.Value != tt.expectValue {
					t.Errorf("expected Value=%d, got %d", tt.expectValue, literal.Value)
				}
			}
		})
	}
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

		if len(program.Statements) != 1 {
			t.Fatalf(
				"program.Statements does not contain %d statements. got=%d\n",
				1,
				len(program.Statements),
			)
		}

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		if !ok {
			t.Fatalf(
				"program.Statements[0] is not ast.ExpressionStatement. got=%T",
				program.Statements[0],
			)
		}

		exp, ok := stmt.Expression.(*ast.PrefixExpression)
		if !ok {
			t.Fatalf("stmt is not ast.PrefixExpression. got=%T", stmt.Expression)
		}
		if exp.Operator != tt.operator {
			t.Fatalf(
				"exp.Operator is not '%s'. got=%s",
				tt.operator, exp.Operator,
			)
		}
		if !testLiteralExpression(t, exp.Right, tt.value) {
			return
		}
	}
}

func TestParsingInfixExpressions(t *testing.T) {
	t.Run("literal expressions", func(t *testing.T) {
		infixTests := []struct {
			input      string
			leftValue  interface{}
			operator   string
			rightValue interface{}
		}{
			{"5 + 5;", 5, "+", 5},
			{"5 - 5;", 5, "-", 5},
			{"5 * 5;", 5, "*", 5},
			{"5 / 5;", 5, "/", 5},
			{"5 % 5;", 5, "%", 5},
			{"5 > 5;", 5, ">", 5},
			{"5 < 5;", 5, "<", 5},
			{"5 >= 5;", 5, ">=", 5},
			{"5 <= 5;", 5, "<=", 5},
			{"5 == 5;", 5, "==", 5},
			{"5 != 5;", 5, "!=", 5},
			{"5 <=> 5;", 5, "<=>", 5},
			{"foobar + barfoo;", "foobar", "+", "barfoo"},
			{"foobar - barfoo;", "foobar", "-", "barfoo"},
			{"foobar * barfoo;", "foobar", "*", "barfoo"},
			{"foobar / barfoo;", "foobar", "/", "barfoo"},
			{"foobar > barfoo;", "foobar", ">", "barfoo"},
			{"foobar < barfoo;", "foobar", "<", "barfoo"},
			{"foobar == barfoo;", "foobar", "==", "barfoo"},
			{"foobar <=> barfoo;", "foobar", "<=>", "barfoo"},
			{"foobar != barfoo;", "foobar", "!=", "barfoo"},
			{"true == true", true, "==", true},
			{"true != false", true, "!=", false},
			{"false == false", false, "==", false},
			{"false || false", false, "||", false},
			{"false && false", false, "&&", false},
		}

		for _, tt := range infixTests {
			program, err := parseSource(tt.input)
			checkParserErrors(t, err)

			if len(program.Statements) != 1 {
				t.Fatalf(
					"program.Statements does not contain %d statements. got=%d\n",
					1,
					len(program.Statements),
				)
			}

			stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
			if !ok {
				t.Fatalf(
					"program.Statements[0] is not ast.ExpressionStatement. got=%T",
					program.Statements[0],
				)
			}

			if !testInfixExpression(t, stmt.Expression, tt.leftValue,
				tt.operator, tt.rightValue) {
				return
			}
		}
	})
	t.Run("symbols expressions", func(t *testing.T) {
		input := ":bar <=> 13"

		program, err := parseSource(input)
		checkParserErrors(t, err)

		if len(program.Statements) != 1 {
			t.Fatalf(
				"program.Statements does not contain %d statements. got=%d\n",
				1,
				len(program.Statements),
			)
		}

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		if !ok {
			t.Fatalf(
				"program.Statements[0] is not ast.ExpressionStatement. got=%T",
				program.Statements[0],
			)
		}

		infix, ok := stmt.Expression.(*ast.InfixExpression)
		if !ok {
			t.Fatalf(
				"stmt.Expression is not %T. got=%T",
				infix,
				stmt.Expression,
			)
		}
	})
	t.Run("call expression no args", func(t *testing.T) {
		input := "foo.bar <=> 13"

		program, err := parseSource(input)
		checkParserErrors(t, err)

		if len(program.Statements) != 1 {
			t.Fatalf(
				"program.Statements does not contain %d statements. got=%d\n",
				1,
				len(program.Statements),
			)
		}

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		if !ok {
			t.Fatalf(
				"program.Statements[0] is not ast.ExpressionStatement. got=%T",
				program.Statements[0],
			)
		}

		infix, ok := stmt.Expression.(*ast.InfixExpression)
		if !ok {
			t.Fatalf(
				"stmt.Expression is not %T. got=%T",
				infix,
				stmt.Expression,
			)
		}
	})
	t.Run("call expression with one arg", func(t *testing.T) {
		input := "foo.bar 3 <=> 13"

		program, err := parseSource(input)
		checkParserErrors(t, err)

		if len(program.Statements) != 1 {
			t.Fatalf(
				"program.Statements does not contain %d statements. got=%d\n",
				1,
				len(program.Statements),
			)
		}

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		if !ok {
			t.Fatalf(
				"program.Statements[0] is not ast.ExpressionStatement. got=%T",
				program.Statements[0],
			)
		}

		cce, ok := stmt.Expression.(*ast.ContextCallExpression)
		if !ok {
			t.Fatalf(
				"stmt.Expression is not %T. got=%T",
				cce,
				stmt.Expression,
			)
		}
	})
	t.Run("call expression with two args", func(t *testing.T) {
		input := "foo.bar 3, 5 <=> 13"

		program, err := parseSource(input)
		checkParserErrors(t, err)

		if len(program.Statements) != 1 {
			t.Fatalf(
				"program.Statements does not contain %d statements. got=%d\n",
				1,
				len(program.Statements),
			)
		}

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		if !ok {
			t.Fatalf(
				"program.Statements[0] is not ast.ExpressionStatement. got=%T",
				program.Statements[0],
			)
		}

		cce, ok := stmt.Expression.(*ast.ContextCallExpression)
		if !ok {
			t.Fatalf(
				"stmt.Expression is not %T. got=%T",
				cce,
				stmt.Expression,
			)
		}
	})
	t.Run("complex infix with call expression with just a block", func(t *testing.T) {
		input := "1 + 21 * 8 - 3 <=> foo { |x| x }"

		expr, err := parseExpression(input)
		checkParserErrors(t, err)

		infix, ok := expr.(*ast.InfixExpression)
		if !ok {
			t.Fatalf(
				"stmt.Expression is not %T. got=%T",
				infix,
				expr,
			)
		}
	})
	t.Run("easy infix with call expression with just a block", func(t *testing.T) {
		input := "1 <=> foo { |x| x }"

		expr, err := parseExpression(input)
		checkParserErrors(t, err)

		infix, ok := expr.(*ast.InfixExpression)
		if !ok {
			t.Fatalf(
				"stmt.Expression is not %T. got=%T",
				infix,
				expr,
			)
		}
	})
}

func TestOperatorPrecedenceParsing(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			"-a * b",
			"-a * b",
		},
		{
			"!-a",
			"!-a",
		},
		{
			"a + b + c",
			"a + b + c",
		},
		{
			"a + b - c",
			"a + b - c",
		},
		{
			"a * b * c",
			"a * b * c",
		},
		{
			"a * b / c",
			"a * b / c",
		},
		{
			"a + b / c",
			"a + b / c",
		},
		{
			"a + b * c + d / e - f",
			"a + b * c + d / e - f",
		},
		{
			"3 + 4; -5 * 5",
			"3 + 4\n-5 * 5",
		},
		{
			"5 > 4 == 3 < 4",
			"5 > 4 == 3 < 4",
		},
		{
			"5 < 4 != 3 > 4",
			"5 < 4 != 3 > 4",
		},
		{
			"3 + 4 * 5 == 3 * 1 + 4 * 5",
			"3 + 4 * 5 == 3 * 1 + 4 * 5",
		},
		{
			"3 + 4 * 5 == 3 * 1 + 4 * 5",
			"3 + 4 * 5 == 3 * 1 + 4 * 5",
		},
		{
			"true | true",
			"true | true",
		},
		{
			"true & true",
			"true & true",
		},
		{
			"3 > 5 == false",
			"3 > 5 == false",
		},
		{
			"3 < 5 == true",
			"3 < 5 == true",
		},
		{
			"1 + (2 + 3) + 4",
			"1 + (2 + 3) + 4",
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
			"(5 + 5) * 2 * (5 + 5)",
		},
		{
			"-(5 + 5)",
			"-(5 + 5)",
		},
		{
			"!(true == true)",
			"!(true == true)",
		},
		{
			"a + add(b * c) + d",
			"a + add(b * c) + d",
		},
		{
			"add(a, b, 1, 2 * 3, 4 + 5, add(6, 7 * 8))",
			"add(a, b, 1, 2 * 3, 4 + 5, add(6, 7 * 8))",
		},
		{
			"add(a + b + c * d / f + g)",
			"add(a + b + c * d / f + g)",
		},
		{
			"add(a + b + c * d / f + g)",
			"add(a + b + c * d / f + g)",
		},
		{
			"x = 12 * 3;",
			"x = 12 * 3",
		},
		{
			"x = 3 + 4 * 3;",
			"x = 3 + 4 * 3",
		},
		{
			"x = add(4) * 3;",
			"x = add(4) * 3",
		},
		{
			"add(x = add(4) * 3);",
			"add(x = add(4) * 3)",
		},
		{
			"a = b = 0;",
			"a = b = 0",
		},
		{
			"a * [1, 2, 3, 4][b * c] * d",
			"a * [1, 2, 3, 4][b * c] * d",
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

			actual := program.String()
			if actual != tt.expected {
				t.Errorf("expected=%q, got=%q", tt.expected, actual)
			}
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
		{
			"method do; x; end",
			nil,
			"x",
		},
		{
			`
			method do
				x
			end`,
			nil,
			"x",
		},
		{
			"method do |x| x; end",
			[]*ast.Identifier{{Value: "x"}},
			"x",
		},
		{
			`method do |x|
				x
			end`,
			[]*ast.Identifier{{Value: "x"}},
			"x",
		},
	}

	for _, tt := range tests {
		program, err := parseSource(tt.input)
		checkParserErrors(t, err)

		if len(program.Statements) != 1 {
			t.Fatalf(
				"program has not enough statements. got=%d",
				len(program.Statements),
			)
		}

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		if !ok {
			t.Fatalf(
				"program.Statements[0] is not ast.ExpressionStatement. got=%T",
				program.Statements[0],
			)
		}

		call, ok := stmt.Expression.(*ast.ContextCallExpression)
		if !ok {
			t.Fatalf("exp not *ast.ContextCallExpression. got=%T", stmt.Expression)
		}

		block := call.Block
		if block == nil {
			t.Logf("Expected block not to be nil")
			t.FailNow()
		}

		if len(block.Parameters) != len(tt.expectedArguments) {
			t.Logf("Expected %d parameters, got %d", len(tt.expectedArguments), len(block.Parameters))
			t.Fail()
		}

		for i, arg := range block.Parameters {
			expected := tt.expectedArguments[i]
			expectedArg := expected.String()
			actualArg := arg.String()

			if expectedArg != actualArg {
				t.Logf(
					"Expected block argument %d to equal\n%s\n\tgot\n%s\n",
					i,
					expectedArg,
					actualArg,
				)
				t.Fail()
			}
		}

		body := block.Body.String()
		expectedBody := tt.expectedBody
		if expectedBody != body {
			t.Logf("Expected body to equal\n%s\n\tgot\n%s\n", expectedBody, body)
			t.Fail()
		}
	}
}

func TestBooleanExpression(t *testing.T) {
	tests := []struct {
		input           string
		expectedBoolean bool
	}{
		{"true;", true},
		{"false;", false},
	}

	for _, tt := range tests {
		program, err := parseSource(tt.input)
		checkParserErrors(t, err)

		if len(program.Statements) != 1 {
			t.Fatalf(
				"program has not enough statements. got=%d",
				len(program.Statements),
			)
		}

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		if !ok {
			t.Fatalf(
				"program.Statements[0] is not ast.ExpressionStatement. got=%T",
				program.Statements[0],
			)
		}

		boolean, ok := stmt.Expression.(*ast.Boolean)
		if !ok {
			t.Fatalf("exp not *ast.Boolean. got=%T", stmt.Expression)
		}
		if boolean.Value != tt.expectedBoolean {
			t.Errorf(
				"boolean.Value not %t. got=%t",
				tt.expectedBoolean,
				boolean.Value)
		}
	}
}

func TestNilExpression(t *testing.T) {
	input := "nil;"

	program, err := parseSource(input)
	checkParserErrors(t, err)

	if len(program.Statements) != 1 {
		t.Fatalf(
			"program has not enough statements. got=%d",
			len(program.Statements),
		)
	}

	stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
	if !ok {
		t.Fatalf(
			"program.Statements[0] is not ast.ExpressionStatement. got=%T",
			program.Statements[0],
		)
	}

	if _, ok := stmt.Expression.(*ast.Nil); !ok {
		t.Fatalf("exp not *ast.Nil. got=%T", stmt.Expression)
	}
}

func TestConditionalExpression(t *testing.T) {
	t.Run("with operator expression", func(t *testing.T) {
		tests := []struct {
			input                         string
			expectedConditionLeft         string
			expectedConditionOperator     string
			expectedConditionRight        string
			expectedConsequenceExpression string
		}{
			{`if x < y
			x
			end`, "x", "<", "y", "x"},
			{`if x < y then
			x
			end`, "x", "<", "y", "x"},
			{`if x < y; x
			end`, "x", "<", "y", "x"},
			{`if x < y
			if x == 3
			y
			end
			x
			end`, "x", "<", "y", "if x == 3\ny\nendx"},
			{`if x < y
			x = Object x
			end`, "x", "<", "y", "x = Object(x)"},
			{"x 3 if x < y", "x", "<", "y", "x(3)"},
			{"x.add 3 if x < y", "x", "<", "y", "x.add(3)"},
			{"yield 3 if x < y", "x", "<", "y", "yield(3)"},
			{"yield self if x < y", "x", "<", "y", "yield(self)"},
			{`unless x < y
			x
			end`, "x", "<", "y", "x"},
			{`unless x < y then
			x
			end`, "x", "<", "y", "x"},
			{`unless x < y; x
			end`, "x", "<", "y", "x"},
			{`unless x < y
			if x == 3
			y
			end
			x
			end`, "x", "<", "y", "if x == 3\ny\nendx"},
			{`unless x < y
			x = Object x
			end`, "x", "<", "y", "x = Object(x)"},
			{"x = 3 if x < y", "x", "<", "y", "x = 3"},
			{"@x = 3 if x < y", "x", "<", "y", "@x = 3"},
			{"x = 3 unless x < y", "x", "<", "y", "x = 3"},
			{"@x = 3 unless x < y", "x", "<", "y", "@x = 3"},
			{"x 3 unless x < y", "x", "<", "y", "x(3)"},
			{"x.add 3 unless x < y", "x", "<", "y", "x.add(3)"},
			{"yield 3 unless x < y", "x", "<", "y", "yield(3)"},
			{"yield self unless x < y", "x", "<", "y", "yield(self)"},
		}

		for _, tt := range tests {
			t.Run("expression "+tt.input, func(t *testing.T) {
				program, err := parseSource(tt.input)
				checkParserErrors(t, err)

				if len(program.Statements) != 1 {
					t.Fatalf(
						"program.Body does not contain %d statements. got=%d\n",
						1,
						len(program.Statements),
					)
				}

				stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
				if !ok {
					t.Fatalf(
						"program.Statements[0] is not ast.ExpressionStatement. got=%T",
						program.Statements[0],
					)
				}

				exp, ok := stmt.Expression.(*ast.ConditionalExpression)
				if !ok {
					t.Fatalf(
						"stmt.Expression is not %T. got=%T",
						exp,
						stmt.Expression,
					)
				}

				if !testInfixExpression(
					t,
					exp.Condition,
					tt.expectedConditionLeft,
					tt.expectedConditionOperator,
					tt.expectedConditionRight,
				) {
					return
				}

				consequenceBody := ""
				for _, stmt := range exp.Consequence.Statements {
					consequence, ok := stmt.(*ast.ExpressionStatement)
					if !ok {
						t.Fatalf(
							"Statements[0] is not ast.ExpressionStatement. got=%T",
							exp.Consequence.Statements[0],
						)
					}

					consequenceBody += consequence.Expression.String()
				}

				if consequenceBody != tt.expectedConsequenceExpression {
					t.Logf(
						"Expected consequence to equal %q, got %q\n",
						tt.expectedConsequenceExpression,
						consequenceBody,
					)
					t.Fail()
				}

				if exp.Alternative != nil {
					t.Errorf("exp.Alternative.Statements was not nil. got=%+v", exp.Alternative)
				}
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
			end`, "x", "exist?", "y", "x"},
			{`unless x.exist? :y
			x = Object x
			end`, "x", "exist?", "y", "x = Object x"},
			{`unless x.exist? :y
			x
			end`, "x", "exist?", "y", "x"},
			{`unless x.exist? :y
			x = Object x
			end`, "x", "exist?", "y", "x = Object x"},
		}

		for _, tt := range tests {
			program, err := parseSource(tt.input)
			checkParserErrors(t, err)

			if len(program.Statements) != 1 {
				t.Fatalf(
					"program.Body does not contain %d statements. got=%d\n",
					1,
					len(program.Statements),
				)
			}

			stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
			if !ok {
				t.Fatalf(
					"program.Statements[0] is not ast.ExpressionStatement. got=%T",
					program.Statements[0],
				)
			}

			exp, ok := stmt.Expression.(*ast.ConditionalExpression)
			if !ok {
				t.Fatalf(
					"stmt.Expression is not %T. got=%T",
					exp,
					stmt.Expression,
				)
			}

			call, ok := exp.Condition.(*ast.ContextCallExpression)
			if !ok {
				t.Fatalf(
					"exp.Condition is not %T. got=%T",
					call,
					exp.Condition,
				)
			}

			if call.Function.String() != tt.condMethod {
				t.Logf(
					"Expected condition call method to equal %q, got %q\n",
					tt.condMethod,
					call.Function.String(),
				)
			}

			args := []string{}
			for _, a := range call.Arguments {
				args = append(args, a.String())
			}
			if strings.Join(args, " ") != tt.condArg {
				t.Logf(
					"Expected condition call args to equal %q, got %q\n",
					tt.condArg,
					strings.Join(args, " "),
				)
			}

			if call.Context.String() != tt.condContext {
				t.Logf(
					"Expected condition call context to equal %q, got %q\n",
					tt.condContext,
					call.Context.String(),
				)
			}

			consequenceBody := ""
			for _, stmt := range exp.Consequence.Statements {
				consequence, ok := stmt.(*ast.ExpressionStatement)
				if !ok {
					t.Fatalf(
						"Statements[0] is not ast.ExpressionStatement. got=%T",
						exp.Consequence.Statements[0],
					)
				}

				consequenceBody += consequence.Expression.String()
			}

			if consequenceBody != tt.consequence {
				t.Logf(
					"Expected consequence to equal %q, got %q\n",
					tt.consequence,
					consequenceBody,
				)
			}

			if exp.Alternative != nil {
				t.Errorf("exp.Alternative.Statements was not nil. got=%+v", exp.Alternative)
			}
		}
	})
}

func TestConditionalExpressionWithAlternative(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		condition   [3]string
		consequence string
		alternative string
	}{
		{
			"regular if else",
			`
			if x < y
				x
			else
				y
			end`,
			[3]string{"x", "<", "y"},
			"x",
			"y",
		},
		{
			"tenary if",
			"x < y ? x : y;",
			[3]string{"x", "<", "y"},
			"x",
			"y",
		},
		{
			"tenary if with symbol as consequence",
			"x < y ? :x : y;",
			[3]string{"x", "<", "y"},
			":x",
			"y",
		},
		{
			"tenary if with symbol as alternative",
			"x < y ? x : :y;",
			[3]string{"x", "<", "y"},
			"x",
			":y",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			program, err := parseSource(tt.input)
			checkParserErrors(t, err)

			if len(program.Statements) != 1 {
				t.Fatalf("program.Body does not contain %d statements. got=%d\n",
					1, len(program.Statements))
			}

			stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
			if !ok {
				t.Fatalf("program.Statements[0] is not ast.ExpressionStatement. got=%T",
					program.Statements[0])
			}

			exp, ok := stmt.Expression.(*ast.ConditionalExpression)
			if !ok {
				t.Fatalf("stmt.Expression is not ast.IfExpression. got=%T", stmt.Expression)
			}

			if !testInfixExpression(t, exp.Condition, tt.condition[0], tt.condition[1], tt.condition[2]) {
				return
			}

			if len(exp.Consequence.Statements) != 1 {
				t.Errorf("consequence is not 1 statements. got=%d\n",
					len(exp.Consequence.Statements))
			}

			consequence, ok := exp.Consequence.Statements[0].(*ast.ExpressionStatement)
			if !ok {
				t.Fatalf("Statements[0] is not ast.ExpressionStatement. got=%T",
					exp.Consequence.Statements[0])
			}

			if !testLiteralExpression(t, consequence.Expression, tt.consequence) {
				return
			}

			if len(exp.Alternative.Statements) != 1 {
				t.Errorf("exp.Alternative.Statements does not contain 1 statements. got=%d\n",
					len(exp.Alternative.Statements))
			}

			alternative, ok := exp.Alternative.Statements[0].(*ast.ExpressionStatement)
			if !ok {
				t.Fatalf("Statements[0] is not ast.ExpressionStatement. got=%T",
					exp.Alternative.Statements[0])
			}

			if !testLiteralExpression(t, alternative.Expression, tt.alternative) {
				return
			}
		})
	}
	t.Run("tenary if with call as consequence", func(t *testing.T) {
		tt := struct {
			input       string
			condition   [3]string
			consequence string
			alternative string
		}{
			"x < y ? x.foo : y;",
			[3]string{"x", "<", "y"},
			"x.foo",
			"y",
		}
		program, err := parseSource(tt.input)
		checkParserErrors(t, err)

		if len(program.Statements) != 1 {
			t.Fatalf("program.Body does not contain %d statements. got=%d\n",
				1, len(program.Statements))
		}

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		if !ok {
			t.Fatalf("program.Statements[0] is not ast.ExpressionStatement. got=%T",
				program.Statements[0])
		}

		exp, ok := stmt.Expression.(*ast.ConditionalExpression)
		if !ok {
			t.Fatalf("stmt.Expression is not ast.IfExpression. got=%T", stmt.Expression)
		}

		if !testInfixExpression(t, exp.Condition, tt.condition[0], tt.condition[1], tt.condition[2]) {
			return
		}

		if len(exp.Consequence.Statements) != 1 {
			t.Errorf("consequence is not 1 statements. got=%d\n",
				len(exp.Consequence.Statements))
		}

		consequence, ok := exp.Consequence.Statements[0].(*ast.ExpressionStatement)
		if !ok {
			t.Fatalf("Statements[0] is not ast.ExpressionStatement. got=%T",
				exp.Consequence.Statements[0])
		}

		if consequence.String() != tt.consequence {
			t.Logf("Expected consequence to equal %s, got %s", tt.consequence, consequence.String())
			t.Fail()
		}

		if len(exp.Alternative.Statements) != 1 {
			t.Errorf("exp.Alternative.Statements does not contain 1 statements. got=%d\n",
				len(exp.Alternative.Statements))
		}

		alternative, ok := exp.Alternative.Statements[0].(*ast.ExpressionStatement)
		if !ok {
			t.Fatalf("Statements[0] is not ast.ExpressionStatement. got=%T",
				exp.Alternative.Statements[0])
		}

		if !testLiteralExpression(t, alternative.Expression, tt.alternative) {
			return
		}
	})
}

func TestCaseExpression(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		expectError  error
		hasCondition bool
		whenCount    int
		hasElse      bool
	}{
		{
			name:         "case with condition, single when",
			input:        "case x\nwhen 1\n  y\nend",
			hasCondition: true,
			whenCount:    1,
		},
		{
			name:      "case without condition",
			input:     "case\nwhen 1\n  y\nend",
			whenCount: 1,
		},
		{
			name:         "case with else",
			input:        "case x\nwhen 1\n  y\nelse\n  z\nend",
			hasCondition: true,
			whenCount:    1,
			hasElse:      true,
		},
		{
			name:         "case with multiple when clauses",
			input:        "case x\nwhen 1\n  a\nwhen 2\n  b\nend",
			hasCondition: true,
			whenCount:    2,
		},
		{
			name:         "case with comma-separated when conditions",
			input:        "case x\nwhen 1, 2, 3\n  y\nend",
			hasCondition: true,
			whenCount:    1,
		},
		{
			name:         "case with then keyword",
			input:        "case x\nwhen 1 then\ny\nend",
			hasCondition: true,
			whenCount:    1,
		},
		{
			name:         "case missing end",
			input:        "case x\nwhen 1\n  y",
			hasCondition: true,
			expectError:  &unexpectedTokenError{expectedTokens: []token.Type{token.EOF}, actualToken: token.EOF},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			program, err := parseSource(tt.input)
			if tt.expectError != nil {
				compareFirstParserError(t, tt.expectError, err)
				return
			}
			checkParserErrors(t, err)

			if len(program.Statements) != 1 {
				t.Fatalf("expected 1 statement, got %d", len(program.Statements))
			}
			stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
			if !ok {
				t.Fatalf("expected *ast.ExpressionStatement, got %T", program.Statements[0])
			}

			caseExpr, ok := stmt.Expression.(*ast.CaseExpression)
			if !ok {
				t.Fatalf("expected *ast.CaseExpression, got %T", stmt.Expression)
			}

			if tt.hasCondition {
				if caseExpr.Condition == nil {
					t.Errorf("expected condition to be present")
				}
			} else {
				if caseExpr.Condition != nil {
					t.Errorf("expected no condition, got %s", caseExpr.Condition.String())
				}
			}

			if len(caseExpr.WhenClauses) != tt.whenCount {
				t.Errorf("expected %d when clauses, got %d", tt.whenCount, len(caseExpr.WhenClauses))
			}

			for _, wc := range caseExpr.WhenClauses {
				if len(wc.Conditions) == 0 {
					t.Errorf("when clause has no conditions")
				}
				if wc.Body == nil {
					t.Errorf("when clause has no body")
				}
			}

			if tt.hasElse {
				if caseExpr.ElseBody == nil {
					t.Errorf("expected else body")
				}
			} else {
				if caseExpr.ElseBody != nil {
					t.Errorf("expected no else body")
				}
			}

			if caseExpr.TokenLiteral() != "case" {
				t.Errorf("expected TokenLiteral to be 'case', got %q", caseExpr.TokenLiteral())
			}
		})
	}
}

func TestFunctionLiteralParsing(t *testing.T) {
	type funcParam struct {
		name         string
		defaultValue interface{}
	}
	tests := []struct {
		desc          string
		input         string
		receiver      string
		name          string
		parameters    []funcParam
		bodyStatement string
	}{
		{
			"with parens",
			`def foo(x, y)
			  x + y
          end`,
			"",
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
			"",
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
			"",
			"qux",
			[]funcParam{},
			"x + y",
		},
		{
			"expression separator semicolon no arguments",
			"def qux; x + y; end",
			"",
			"qux",
			[]funcParam{},
			"x + y",
		},
		{
			"expression separator semicolon two arguments",
			"def foo x, y; x + y; end",
			"",
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
			"",
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
			"",
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
			"",
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
			"",
			"<=>",
			[]funcParam{},
			"x + y",
		},
		{
			"function on local variable context",
			`def a.qux
          x + y
          end`,
			"a",
			"qux",
			[]funcParam{},
			"x + y",
		},
		{
			"function on const context",
			`def A.qux
          x + y
          end`,
			"A",
			"qux",
			[]funcParam{},
			"x + y",
		},
		{
			"upcase function on const context",
			`def A.Qux
          x + y
          end`,
			"A",
			"Qux",
			[]funcParam{},
			"x + y",
		},
		{
			"function on self context",
			`def self.qux
          x + y
          end`,
			"self",
			"qux",
			[]funcParam{},
			"x + y",
		},
		{
			"single-line body after parens",
			"def right() x + y end",
			"",
			"right",
			[]funcParam{},
			"x + y",
		},
		{
			"single-line body after parens with args",
			"def add(x, y) x + y end",
			"",
			"add",
			[]funcParam{
				{name: "x", defaultValue: nil},
				{name: "y", defaultValue: nil},
			},
			"x + y",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			program, err := parseSource(tt.input)
			checkParserErrors(t, err)

			if len(program.Statements) != 1 {
				t.Fatalf(
					"program.Body does not contain %d statements. got=%d\n",
					1,
					len(program.Statements),
				)
			}

			stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
			if !ok {
				t.Fatalf(
					"program.Statements[0] is not ast.ExpressionStatement. got=%T",
					program.Statements[0],
				)
			}

			function, ok := stmt.Expression.(*ast.FunctionLiteral)
			if !ok {
				t.Fatalf(
					"stmt.Expression is not ast.FunctionLiteral. got=%T",
					stmt.Expression,
				)
			}

			receiver := ""
			if function.Receiver != nil {
				receiver = function.Receiver.Value
			}
			if receiver != tt.receiver {
				t.Logf("function receiver wrong, want %q, got %q", tt.receiver, receiver)
				t.Fail()
			}

			functionName := function.Name.Value
			if functionName != tt.name {
				t.Logf("function name wrong, want %q, got %q", tt.name, functionName)
				t.Fail()
			}

			if len(function.Parameters) != len(tt.parameters) {
				t.Fatalf(
					"function literal parameters wrong. want %d, got=%d\n",
					len(tt.parameters),
					len(function.Parameters),
				)
			}

			for i, param := range function.Parameters {
				testLiteralExpression(t, param.Name, tt.parameters[i].name)
				testLiteralExpression(t, param.Default, tt.parameters[i].defaultValue)
			}

			if len(function.Body.Statements) != 1 {
				t.Fatalf(
					"function.Body.Statements has not 1 statements. got=%d\n",
					len(function.Body.Statements),
				)
			}

			bodyStmt, ok := function.Body.Statements[0].(*ast.ExpressionStatement)
			if !ok {
				t.Fatalf(
					"function body stmt is not ast.ExpressionStatement. got=%T",
					function.Body.Statements[0],
				)
			}

			statement := bodyStmt.String()
			if statement != tt.bodyStatement {
				t.Logf(
					"Expected body statement to equal\n%q\n\tgot\n%q\n",
					tt.bodyStatement,
					statement,
				)
				t.Fail()
			}
		})
	}
	t.Run("test function rescue block", func(t *testing.T) {
		input := `
			def foo
				3
			rescue Exception => e
				puts e
			end
		`

		program, err := parseSource(input)
		checkParserErrors(t, err)

		if len(program.Statements) != 1 {
			t.Fatalf(
				"program.Body does not contain %d statements. got=%d\n",
				1,
				len(program.Statements),
			)
		}

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		if !ok {
			t.Fatalf(
				"program.Statements[0] is not ast.ExpressionStatement. got=%T",
				program.Statements[0],
			)
		}

		function, ok := stmt.Expression.(*ast.FunctionLiteral)
		if !ok {
			t.Fatalf(
				"stmt.Expression is not %T. got=%T",
				function,
				stmt.Expression,
			)
		}
	})
}

func TestBlockExpressionParsing(t *testing.T) {
	tests := []struct {
		input         string
		parameters    []string
		bodyStatement string
	}{
		{
			`method do |x, y|
          x + y
          end`,
			[]string{"x", "y"},
			"x + y",
		},
		{
			`method do
          x + y
          end`,
			[]string{},
			"x + y",
		},
		{
			"method do ; x + y; end",
			[]string{},
			"x + y",
		},
		{
			"method do |x, y|; x + y; end",
			[]string{"x", "y"},
			"x + y",
		},
		{
			"method do |x, y|; x + y; end",
			[]string{"x", "y"},
			"x + y",
		},
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

			if len(program.Statements) != 1 {
				t.Fatalf(
					"program.Body does not contain %d statements. got=%d\n",
					1,
					len(program.Statements),
				)
			}

			stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
			if !ok {
				t.Logf(
					"program.Statements[0] is not ast.ExpressionStatement. got=%T",
					program.Statements[0],
				)
				t.Log(program.Statements)
				t.FailNow()
			}

			call, ok := stmt.Expression.(*ast.ContextCallExpression)
			if !ok {
				t.Logf(
					"stmt.Expression is not *ast.ContextCallExpression. got=%T",
					stmt.Expression,
				)
				t.Fail()
			}

			block := call.Block

			if block == nil {
				t.Logf("Expected block not to be nil")
				t.FailNow()
			}

			if len(block.Parameters) != len(tt.parameters) {
				t.Fatalf(
					"block literal parameters wrong. want %d, got=%d\n",
					len(tt.parameters),
					len(block.Parameters),
				)
			}

			for i, param := range block.Parameters {
				testLiteralExpression(t, param.Name, tt.parameters[i])
			}

			if len(block.Body.Statements) != 1 {
				t.Fatalf(
					"block.Body.Statements has not 1 statements. got=%d\n",
					len(block.Body.Statements),
				)
			}

			bodyStmt, ok := block.Body.Statements[0].(*ast.ExpressionStatement)
			if !ok {
				t.Fatalf(
					"block body stmt is not ast.ExpressionStatement. got=%T",
					block.Body.Statements[0],
				)
			}

			statement := bodyStmt.String()
			if statement != tt.bodyStatement {
				t.Logf(
					"Expected body statement to equal\n%q\n\tgot\n%q\n",
					tt.bodyStatement,
					statement,
				)
				t.Fail()
			}
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
		{
			desc:           "multiple params last block capture with parens",
			input:          "def fn(x, y, &z); end",
			expectedParams: []funcParam{{name: "x"}, {name: "y"}},
		},
		{
			desc:           "one param block capture with parens",
			input:          "def fn(&x); end",
			expectedParams: []funcParam{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			program, err := parseSource(tt.input)
			checkParserErrors(t, err)

			stmt := program.Statements[0].(*ast.ExpressionStatement)
			function := stmt.Expression.(*ast.FunctionLiteral)

			if len(function.Parameters) != len(tt.expectedParams) {
				t.Errorf(
					"length parameters wrong. want %d, got=%d\n",
					len(tt.expectedParams),
					len(function.Parameters),
				)
			}

			for i, ident := range tt.expectedParams {
				testLiteralExpression(t, function.Parameters[i].Name, ident.name)
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
			desc:           "empty do block",
			input:          "method do; end",
			expectedParams: []funcParam{},
		},
		{
			desc:           "empty do block params",
			input:          "method do ||; end",
			expectedParams: []funcParam{},
		},
		{
			desc:           "one do block param",
			input:          "method do |x|; end",
			expectedParams: []funcParam{{name: "x"}},
		},
		{
			desc:           "multiple do block params",
			input:          "method do |x, y, z|; end",
			expectedParams: []funcParam{{name: "x"}, {name: "y"}, {name: "z"}},
		},
		{
			desc:           "multiple brace block params with defaults",
			input:          "method { |x = 3, y = 2, z| }",
			expectedParams: []funcParam{{name: "x", defaultValue: 3}, {name: "y", defaultValue: 2}, {name: "z"}},
		},
		{
			desc:           "multiple do block params starting defaults",
			input:          "method do |x = 1, y = 8, z|; end",
			expectedParams: []funcParam{{name: "x", defaultValue: 1}, {name: "y", defaultValue: 8}, {name: "z"}},
		},
		{
			desc:           "multiple brace block params with middle default",
			input:          "method { |x, y = 2, z| }",
			expectedParams: []funcParam{{name: "x"}, {name: "y", defaultValue: 2}, {name: "z"}},
		},
		{
			desc:           "multiple do block params with middle default",
			input:          "method do |x, y = 8, z|; end",
			expectedParams: []funcParam{{name: "x"}, {name: "y", defaultValue: 8}, {name: "z"}},
		},
		{
			desc:           "multiple brace block params last defaults",
			input:          "method { |x, y, z = 2| }",
			expectedParams: []funcParam{{name: "x"}, {name: "y"}, {name: "z", defaultValue: 2}},
		},
		{
			desc:           "multiple do block params last defaults",
			input:          "method do |x, y, z = 4|; end",
			expectedParams: []funcParam{{name: "x"}, {name: "y"}, {name: "z", defaultValue: 4}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			program, err := parseSource(tt.input)
			checkParserErrors(t, err)

			stmt := program.Statements[0].(*ast.ExpressionStatement)
			call, ok := stmt.Expression.(*ast.ContextCallExpression)
			if !ok {
				t.Logf(
					"stmt.Expression is not *ast.ContextCallExpression. got=%T",
					stmt.Expression,
				)
				t.Fail()
			}

			block := call.Block

			if block == nil {
				t.Logf("Expected block not to be nil")
				t.FailNow()
			}

			if len(block.Parameters) != len(tt.expectedParams) {
				t.Errorf(
					"length parameters wrong. want %d, got=%d\n",
					len(tt.expectedParams),
					len(block.Parameters),
				)
			}

			for i, ident := range tt.expectedParams {
				testLiteralExpression(t, block.Parameters[i].Name, ident.name)
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
				1, infix{2, "*", 3}, infix{4, "+", 5},
			},
		},
		{
			desc:     "without parens",
			input:    "add 1, 2 * 3, 4 + 5;",
			funcName: "add",
			arguments: []interface{}{
				1, infix{2, "*", 3}, infix{4, "+", 5},
			},
		},
		{
			desc:     "with parens and brace block",
			input:    "add(1, 2 * 3, 4 + 5) { |x| x };",
			funcName: "add",
			arguments: []interface{}{
				1, infix{2, "*", 3}, infix{4, "+", 5},
			},
			hasBlock:    true,
			blockParams: []string{"x"},
		},
		{
			desc:     "with parens and do block",
			input:    "add(1, 2 * 3, 4 + 5) do |x| x; end;",
			funcName: "add",
			arguments: []interface{}{
				1, infix{2, "*", 3}, infix{4, "+", 5},
			},
			hasBlock:    true,
			blockParams: []string{"x"},
		},
		{
			desc:     "without parens with block",
			input:    "add 1, 2 * 3, 4 + 5 { |x| x };",
			funcName: "add",
			arguments: []interface{}{
				1, infix{2, "*", 3}, infix{4, "+", 5},
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
			if !ok {
				t.Fatalf(
					"expression is not %T, got=%T",
					call,
					expr,
				)
			}

			if !testIdentifier(t, call.Function, tt.funcName) {
				return
			}

			if tt.context != "" && !testIdentifier(t, call.Context, tt.context) {
				return
			}

			if len(call.Arguments) != len(tt.arguments) {
				t.Logf(
					"wrong length of arguments. want %d, got=%d",
					len(tt.arguments),
					len(call.Arguments),
				)
				t.Fail()
				for len(call.Arguments) > len(tt.arguments) {
					tt.arguments = append(tt.arguments, "<unexpected>")
				}
			}

			for i, arg := range call.Arguments {
				t.Logf("argument %d", i+1)
				testExpression(t, arg, tt.arguments[i])
			}

			if tt.hasBlock {
				if call.Block == nil {
					t.Logf("Expected function block not to be nil")
					t.FailNow()
				}

				if len(call.Block.Parameters) != len(tt.blockParams) {
					t.Logf(
						"wrong length of block parameters. want %d, got=%d",
						len(tt.blockParams),
						len(call.Block.Parameters),
					)
					t.Fail()
					for len(call.Block.Parameters) > len(tt.blockParams) {
						tt.blockParams = append(tt.blockParams, "<unexpected>")
					}
				}

				for i, param := range call.Block.Parameters {
					expected := tt.blockParams[i]
					actual := param.String()

					if expected != actual {
						t.Logf(
							"Expected block param %d to equal\n%s\n\tgot\n%s\n",
							i,
							expected,
							actual,
						)
						t.Fail()
					}
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
			if !ok {
				t.Fatalf(
					"stmt.Expression is not ast.ContextCallExpression. got=%T",
					stmt.Expression,
				)
			}

			if !testIdentifier(t, exp.Function, tt.expectedIdent) {
				return
			}

			if len(exp.Arguments) != len(tt.expectedArgs) {
				t.Fatalf("wrong number of arguments. want=%d, got=%d",
					len(tt.expectedArgs), len(exp.Arguments))
			}

			for i, arg := range tt.expectedArgs {
				if exp.Arguments[i].String() != arg {
					t.Errorf("argument %d wrong. want=%q, got=%q", i,
						arg, exp.Arguments[i].String())
				}
			}
		})
	}
}

func TestContextCallExpression(t *testing.T) {
	t.Run("context call with multiple args with parens", func(t *testing.T) {
		input := "foo.add(1, 2 * 3, 4 + 5);"

		program, err := parseSource(input)
		checkParserErrors(t, err)

		if len(program.Statements) != 1 {
			t.Fatalf("program.Statements does not contain %d statements. got=%d\n",
				1, len(program.Statements))
		}

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		if !ok {
			t.Fatalf("stmt is not ast.ExpressionStatement. got=%T",
				program.Statements[0])
		}

		exp, ok := stmt.Expression.(*ast.ContextCallExpression)
		if !ok {
			t.Fatalf("stmt.Expression is not ast.ContextCallExpression. got=%T",
				stmt.Expression)
		}

		if !testIdentifier(t, exp.Context, "foo") {
			return
		}

		if !testIdentifier(t, exp.Function, "add") {
			return
		}

		if len(exp.Arguments) != 3 {
			t.Fatalf("wrong length of arguments. got=%d", len(exp.Arguments))
		}

		testLiteralExpression(t, exp.Arguments[0], 1)
		testInfixExpression(t, exp.Arguments[1], 2, "*", 3)
		testInfixExpression(t, exp.Arguments[2], 4, "+", 5)
	})
	t.Run("context call with multiple args with parens and block", func(t *testing.T) {
		input := "foo.add(1, 2 * 3, 4 + 5) { |x|x.to_s };"

		program, err := parseSource(input)
		checkParserErrors(t, err)

		if len(program.Statements) != 1 {
			t.Logf("Input: %s\n", input)
			t.Fatalf("program.Statements does not contain %d statements. got=%d\n",
				1, len(program.Statements))
		}

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		if !ok {
			t.Fatalf("stmt is not ast.ExpressionStatement. got=%T",
				program.Statements[0])
		}

		exp, ok := stmt.Expression.(*ast.ContextCallExpression)
		if !ok {
			t.Fatalf("stmt.Expression is not ast.ContextCallExpression. got=%T",
				stmt.Expression)
		}

		if !testIdentifier(t, exp.Context, "foo") {
			return
		}

		if !testIdentifier(t, exp.Function, "add") {
			return
		}

		if len(exp.Arguments) != 3 {
			t.Fatalf("wrong length of arguments. got=%d", len(exp.Arguments))
		}

		testLiteralExpression(t, exp.Arguments[0], 1)
		testInfixExpression(t, exp.Arguments[1], 2, "*", 3)
		testInfixExpression(t, exp.Arguments[2], 4, "+", 5)

		if exp.Block == nil {
			t.Logf("Expected block not to be nil")
			t.Fail()
		}
	})
	t.Run("context call with multiple args no parens", func(t *testing.T) {
		input := "foo.add 1, 2 * 3, 4 + 5;"

		program, err := parseSource(input)
		checkParserErrors(t, err)

		if len(program.Statements) != 1 {
			t.Fatalf("program.Statements does not contain %d statements. got=%d\n",
				1, len(program.Statements))
		}

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		if !ok {
			t.Fatalf("stmt is not ast.ExpressionStatement. got=%T",
				program.Statements[0])
		}

		exp, ok := stmt.Expression.(*ast.ContextCallExpression)
		if !ok {
			t.Fatalf("stmt.Expression is not ast.ContextCallExpression. got=%T",
				stmt.Expression)
		}

		if !testIdentifier(t, exp.Context, "foo") {
			return
		}

		if !testIdentifier(t, exp.Function, "add") {
			return
		}

		if len(exp.Arguments) != 3 {
			t.Fatalf("wrong length of arguments. got=%d", len(exp.Arguments))
		}

		testLiteralExpression(t, exp.Arguments[0], 1)
		testInfixExpression(t, exp.Arguments[1], 2, "*", 3)
		testInfixExpression(t, exp.Arguments[2], 4, "+", 5)
	})
	t.Run("context call with multiple args no parens with block", func(t *testing.T) {
		input := "foo.add 1, 2 * 3, 4 + 5 { |x| x };"

		program, err := parseSource(input)
		checkParserErrors(t, err)

		if len(program.Statements) != 1 {
			t.Fatalf("program.Statements does not contain %d statements. got=%d\n",
				1, len(program.Statements))
		}

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		if !ok {
			t.Fatalf("stmt is not ast.ExpressionStatement. got=%T",
				program.Statements[0])
		}

		exp, ok := stmt.Expression.(*ast.ContextCallExpression)
		if !ok {
			t.Fatalf("stmt.Expression is not ast.ContextCallExpression. got=%T",
				stmt.Expression)
		}

		if !testIdentifier(t, exp.Context, "foo") {
			return
		}

		if !testIdentifier(t, exp.Function, "add") {
			return
		}

		if len(exp.Arguments) != 3 {
			t.Fatalf("wrong length of arguments. got=%d", len(exp.Arguments))
		}

		testLiteralExpression(t, exp.Arguments[0], 1)
		testInfixExpression(t, exp.Arguments[1], 2, "*", 3)
		testInfixExpression(t, exp.Arguments[2], 4, "+", 5)

		if exp.Block == nil {
			t.Logf("Expected block not to be nil")
			t.Fail()
		}
	})
	t.Run("context call with no args", func(t *testing.T) {
		input := "foo.add;"

		program, err := parseSource(input)
		checkParserErrors(t, err)

		if len(program.Statements) != 1 {
			t.Fatalf("program.Statements does not contain %d statements. got=%d\n",
				1, len(program.Statements))
		}

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		if !ok {
			t.Fatalf("stmt is not ast.ExpressionStatement. got=%T",
				program.Statements[0])
		}

		exp, ok := stmt.Expression.(*ast.ContextCallExpression)
		if !ok {
			t.Fatalf("stmt.Expression is not ast.ContextCallExpression. got=%T",
				stmt.Expression)
		}

		if !testIdentifier(t, exp.Context, "foo") {
			return
		}

		if !testIdentifier(t, exp.Function, "add") {
			return
		}

		if len(exp.Arguments) != 0 {
			t.Fatalf("wrong length of arguments. got=%d", len(exp.Arguments))
		}
	})
	t.Run("context call on self with no args", func(t *testing.T) {
		input := "self.add;"

		program, err := parseSource(input)
		checkParserErrors(t, err)

		if len(program.Statements) != 1 {
			t.Fatalf("program.Statements does not contain %d statements. got=%d\n",
				1, len(program.Statements))
		}

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		if !ok {
			t.Fatalf("stmt is not ast.ExpressionStatement. got=%T",
				program.Statements[0])
		}

		exp, ok := stmt.Expression.(*ast.ContextCallExpression)
		if !ok {
			t.Fatalf("stmt.Expression is not ast.ContextCallExpression. got=%T",
				stmt.Expression)
		}

		if _, ok := exp.Context.(*ast.Self); !ok {
			t.Logf("exp.Context is not ast.Self, got=%T", exp.Context)
			t.Fail()
		}

		if !testIdentifier(t, exp.Function, "add") {
			return
		}

		if len(exp.Arguments) != 0 {
			t.Fatalf("wrong length of arguments. got=%d", len(exp.Arguments))
		}
	})
	t.Run("context call on self with no dot", func(t *testing.T) {
		input := "self add;"

		_, err := parseSource(input)

		if err == nil {
			t.Logf("Expected parser error, got nil")
			t.FailNow()
		}

		errs := err.errors
		cause := errors.Cause(errs[0])

		unexpectErr, ok := cause.(*unexpectedTokenError)
		if !ok {
			t.Logf("Expected err to be %T, got %T\n", unexpectErr, cause)
			t.FailNow()
		}

		{
			expected := []token.Type{token.DOT}
			actual := unexpectErr.expectedTokens
			if !reflect.DeepEqual(expected, actual) {
				t.Logf("Expected error to equal\n%+#v\n\tgot\n%+#v\n", expected, actual)
				t.Fail()
			}
		}

		{
			expected := token.IDENT
			actual := unexpectErr.actualToken
			if !reflect.DeepEqual(expected, actual) {
				t.Logf("Expected error to equal\n%+#v\n\tgot\n%+#v\n", expected, actual)
				t.Fail()
			}
		}
	})
	t.Run("context call on nonident with no dot", func(t *testing.T) {
		input := "1 add;"

		program, err := parseSource(input)
		checkParserErrors(t, err)

		if len(program.Statements) != 1 {
			t.Fatalf(
				"program.Statements does not contain %d statements. got=%d\n",
				1,
				len(program.Statements),
			)
		}

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		if !ok {
			t.Fatalf(
				"stmt is not ast.ExpressionStatement. got=%T",
				program.Statements[0],
			)
		}

		exp, ok := stmt.Expression.(*ast.ContextCallExpression)
		if !ok {
			t.Fatalf("stmt.Expression is not ast.ContextCallExpression. got=%T",
				stmt.Expression)
		}

		if !testIntegerLiteral(t, exp.Context, 1) {
			return
		}

		if !testIdentifier(t, exp.Function, "add") {
			return
		}

		if len(exp.Arguments) != 0 {
			t.Fatalf("wrong length of arguments. got=%d", len(exp.Arguments))
		}
	})
	t.Run("context call on nonident with dot", func(t *testing.T) {
		input := "1.add"

		program, err := parseSource(input)
		checkParserErrors(t, err)

		if len(program.Statements) != 1 {
			t.Fatalf(
				"program.Statements does not contain %d statements. got=%d\n",
				1,
				len(program.Statements),
			)
		}

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		if !ok {
			t.Fatalf(
				"stmt is not ast.ExpressionStatement. got=%T",
				program.Statements[0],
			)
		}

		exp, ok := stmt.Expression.(*ast.ContextCallExpression)
		if !ok {
			t.Fatalf("stmt.Expression is not ast.ContextCallExpression. got=%T",
				stmt.Expression)
		}

		if !testIntegerLiteral(t, exp.Context, 1) {
			return
		}

		if !testIdentifier(t, exp.Function, "add") {
			return
		}

		if len(exp.Arguments) != 0 {
			t.Fatalf("wrong length of arguments. got=%d", len(exp.Arguments))
		}
	})
	t.Run("context call on nonident with no dot multiargs", func(t *testing.T) {
		input := "1 add 1"

		program, err := parseSource(input)
		checkParserErrors(t, err)

		if len(program.Statements) != 1 {
			t.Fatalf(
				"program.Statements does not contain %d statements. got=%d\n",
				1,
				len(program.Statements),
			)
		}

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		if !ok {
			t.Fatalf(
				"stmt is not ast.ExpressionStatement. got=%T",
				program.Statements[0],
			)
		}

		exp, ok := stmt.Expression.(*ast.ContextCallExpression)
		if !ok {
			t.Fatalf("stmt.Expression is not ast.ContextCallExpression. got=%T",
				stmt.Expression)
		}

		if !testIntegerLiteral(t, exp.Context, 1) {
			return
		}

		if !testIdentifier(t, exp.Function, "add") {
			return
		}

		if len(exp.Arguments) != 1 {
			t.Fatalf(
				"wrong length of arguments. got=%d",
				len(exp.Arguments),
			)
		}

		if !testIntegerLiteral(t, exp.Arguments[0], 1) {
			return
		}
	})
	t.Run("context call on ident with no dot", func(t *testing.T) {
		input := "foo add;"

		program, err := parseSource(input)
		checkParserErrors(t, err)

		if len(program.Statements) != 1 {
			t.Fatalf("program.Statements does not contain %d statements. got=%d\n",
				1, len(program.Statements))
		}

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		if !ok {
			t.Fatalf("stmt is not ast.ExpressionStatement. got=%T",
				program.Statements[0])
		}

		exp, ok := stmt.Expression.(*ast.ContextCallExpression)
		if !ok {
			t.Fatalf("stmt.Expression is not ast.ContextCallExpression. got=%T",
				stmt.Expression)
		}

		if !testIdentifier(t, exp.Function, "foo") {
			return
		}

		if len(exp.Arguments) != 1 {
			t.Fatalf("wrong length of arguments. got=%d", len(exp.Arguments))
		}

		if !testIdentifier(t, exp.Arguments[0], "add") {
			return
		}
	})
	t.Run("context call on const with no dot", func(t *testing.T) {
		input := "Integer add;"

		program, err := parseSource(input)
		checkParserErrors(t, err)

		if len(program.Statements) != 1 {
			t.Fatalf("program.Statements does not contain %d statements. got=%d\n",
				1, len(program.Statements))
		}

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		if !ok {
			t.Fatalf("stmt is not ast.ExpressionStatement. got=%T",
				program.Statements[0])
		}

		exp, ok := stmt.Expression.(*ast.ContextCallExpression)
		if !ok {
			t.Fatalf("stmt.Expression is not ast.ContextCallExpression. got=%T",
				stmt.Expression)
		}

		if !testIdentifier(t, exp.Function, "Integer") {
			return
		}

		if len(exp.Arguments) != 1 {
			t.Fatalf("wrong length of arguments. got=%d", len(exp.Arguments))
		}

		if !testIdentifier(t, exp.Arguments[0], "add") {
			return
		}
	})
	t.Run("context call on ident with no dot Const as arg", func(t *testing.T) {
		input := "add Integer;"

		program, err := parseSource(input)
		checkParserErrors(t, err)

		if len(program.Statements) != 1 {
			t.Fatalf("program.Statements does not contain %d statements. got=%d\n",
				1, len(program.Statements))
		}

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		if !ok {
			t.Fatalf("stmt is not ast.ExpressionStatement. got=%T",
				program.Statements[0])
		}

		exp, ok := stmt.Expression.(*ast.ContextCallExpression)
		if !ok {
			t.Fatalf("stmt.Expression is not ast.ContextCallExpression. got=%T",
				stmt.Expression)
		}

		if !testIdentifier(t, exp.Function, "add") {
			return
		}

		if len(exp.Arguments) != 1 {
			t.Fatalf("wrong length of arguments. got=%d", len(exp.Arguments))
		}

		if !testIdentifier(t, exp.Arguments[0], "Integer") {
			return
		}
	})
	t.Run("chained context call with dot without parens", func(t *testing.T) {
		input := "foo.add.bar;"

		program, err := parseSource(input)
		checkParserErrors(t, err)

		if len(program.Statements) != 1 {
			t.Fatalf(
				"program.Statements does not contain %d statements. got=%d\n",
				1,
				len(program.Statements),
			)
		}

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		if !ok {
			t.Fatalf("stmt is not ast.ExpressionStatement. got=%T",
				program.Statements[0])
		}

		exp, ok := stmt.Expression.(*ast.ContextCallExpression)
		if !ok {
			t.Fatalf("stmt.Expression is not ast.ContextCallExpression. got=%T",
				stmt.Expression)
		}

		context, ok := exp.Context.(*ast.ContextCallExpression)
		if !ok {
			t.Fatalf(
				"expr.Context is not ast.ContextCallExpression. got=%T",
				exp.Context,
			)
		}

		if !testIdentifier(t, context.Context, "foo") {
			return
		}

		if !testIdentifier(t, context.Function, "add") {
			return
		}

		if len(context.Arguments) != 0 {
			t.Fatalf("wrong length of arguments. got=%d", len(context.Arguments))
		}

		if !testIdentifier(t, exp.Function, "bar") {
			return
		}

		if len(exp.Arguments) != 0 {
			t.Fatalf("wrong length of arguments. got=%d", len(exp.Arguments))
		}
	})
	t.Run("chained context call with dot without parens", func(t *testing.T) {
		input := "1.add.bar;"

		program, err := parseSource(input)
		checkParserErrors(t, err)

		if len(program.Statements) != 1 {
			t.Fatalf(
				"program.Statements does not contain %d statements. got=%d\n",
				1,
				len(program.Statements),
			)
		}

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		if !ok {
			t.Fatalf("stmt is not ast.ExpressionStatement. got=%T",
				program.Statements[0])
		}

		exp, ok := stmt.Expression.(*ast.ContextCallExpression)
		if !ok {
			t.Fatalf("stmt.Expression is not ast.ContextCallExpression. got=%T",
				stmt.Expression)
		}

		context, ok := exp.Context.(*ast.ContextCallExpression)
		if !ok {
			t.Fatalf(
				"expr.Context is not ast.ContextCallExpression. got=%T",
				exp.Context,
			)
		}

		if !testIntegerLiteral(t, context.Context, 1) {
			return
		}

		if !testIdentifier(t, context.Function, "add") {
			return
		}

		if len(context.Arguments) != 0 {
			t.Fatalf("wrong length of arguments. got=%d", len(context.Arguments))
		}

		if !testIdentifier(t, exp.Function, "bar") {
			return
		}

		if len(exp.Arguments) != 0 {
			t.Fatalf("wrong length of arguments. got=%d", len(exp.Arguments))
		}
	})
	t.Run("chained context call with dot with parens", func(t *testing.T) {
		input := "foo.add().bar();"

		program, err := parseSource(input)
		checkParserErrors(t, err)

		if len(program.Statements) != 1 {
			t.Fatalf(
				"program.Statements does not contain %d statements. got=%d\n",
				1,
				len(program.Statements),
			)
		}

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		if !ok {
			t.Fatalf("stmt is not ast.ExpressionStatement. got=%T",
				program.Statements[0])
		}

		exp, ok := stmt.Expression.(*ast.ContextCallExpression)
		if !ok {
			t.Fatalf("stmt.Expression is not ast.ContextCallExpression. got=%T",
				stmt.Expression)
		}

		context, ok := exp.Context.(*ast.ContextCallExpression)
		if !ok {
			t.Fatalf(
				"expr.Context is not ast.ContextCallExpression. got=%T",
				exp.Context,
			)
		}

		if !testIdentifier(t, context.Function, "add") {
			return
		}

		if len(context.Arguments) != 0 {
			t.Fatalf("wrong length of arguments. got=%d", len(context.Arguments))
		}

		if !testIdentifier(t, exp.Function, "bar") {
			return
		}

		if len(exp.Arguments) != 0 {
			t.Fatalf("wrong length of arguments. got=%d", len(exp.Arguments))
		}
	})
	t.Run("scope as context call", func(t *testing.T) {
		input := "foo.add::bar;"

		program, err := parseSource(input)
		checkParserErrors(t, err)

		if len(program.Statements) != 1 {
			t.Fatalf(
				"program.Statements does not contain %d statements. got=%d\n",
				1,
				len(program.Statements),
			)
		}

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		if !ok {
			t.Fatalf("stmt is not ast.ExpressionStatement. got=%T",
				program.Statements[0])
		}

		exp, ok := stmt.Expression.(*ast.ContextCallExpression)
		if !ok {
			t.Fatalf("stmt.Expression is not ast.ContextCallExpression. got=%T",
				stmt.Expression)
		}

		context, ok := exp.Context.(*ast.ContextCallExpression)
		if !ok {
			t.Fatalf(
				"expr.Context is not ast.ContextCallExpression. got=%T",
				exp.Context,
			)
		}

		if !testIdentifier(t, context.Function, "add") {
			return
		}

		if len(context.Arguments) != 0 {
			t.Fatalf("wrong length of arguments. got=%d", len(context.Arguments))
		}

		if !testIdentifier(t, exp.Function, "bar") {
			return
		}

		if len(exp.Arguments) != 0 {
			t.Fatalf("wrong length of arguments. got=%d", len(exp.Arguments))
		}
	})
	t.Run("allow `class` as method name", func(t *testing.T) {
		input := "foo.class;"

		program, err := parseSource(input)
		checkParserErrors(t, err)

		if len(program.Statements) != 1 {
			t.Fatalf(
				"program.Statements does not contain %d statements. got=%d\n",
				1,
				len(program.Statements),
			)
		}

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		if !ok {
			t.Fatalf("stmt is not ast.ExpressionStatement. got=%T",
				program.Statements[0])
		}

		expr, ok := stmt.Expression.(*ast.ContextCallExpression)
		if !ok {
			t.Fatalf("stmt.Expression is not ast.ContextCallExpression. got=%T",
				stmt.Expression)
		}

		if !testIdentifier(t, expr.Context, "foo") {
			return
		}

		if !testIdentifier(t, expr.Function, "class") {
			return
		}
	})
	t.Run("allow operators as method name", func(t *testing.T) {
		input := "foo.<=>;"

		program, err := parseSource(input)
		checkParserErrors(t, err)

		if len(program.Statements) != 1 {
			t.Fatalf(
				"program.Statements does not contain %d statements. got=%d\n",
				1,
				len(program.Statements),
			)
		}

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		if !ok {
			t.Fatalf("stmt is not ast.ExpressionStatement. got=%T",
				program.Statements[0])
		}

		expr, ok := stmt.Expression.(*ast.ContextCallExpression)
		if !ok {
			t.Fatalf("stmt.Expression is not ast.ContextCallExpression. got=%T",
				stmt.Expression)
		}

		if !testIdentifier(t, expr.Context, "foo") {
			return
		}

		if !testIdentifier(t, expr.Function, "<=>") {
			return
		}
	})
}

func TestStringLiteralExpression(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectVal   string
		expectParts bool
	}{
		{name: "simple", input: `"hello world"`, expectVal: "hello world"},
		{name: "empty", input: `""`, expectVal: ""},
		{name: "interpolated", input: `"hello #{name}"`, expectParts: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			program, err := parseSource(tt.input)
			checkParserErrors(t, err)

			stmt := program.Statements[0].(*ast.ExpressionStatement)
			literal, ok := stmt.Expression.(*ast.StringLiteral)
			if !ok {
				t.Fatalf("exp not *ast.StringLiteral. got=%T", stmt.Expression)
			}

			if tt.expectParts {
				if len(literal.Parts) == 0 {
					t.Errorf("expected Parts to be non-empty")
				}
			} else {
				if literal.Value != tt.expectVal {
					t.Errorf("literal.Value not %q. got=%q", tt.expectVal, literal.Value)
				}
			}
		})
	}
}

func TestInterpolatedRegex(t *testing.T) {
	tests := []string{
		"/simple/",
		"/foo bar/",
	}
	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			program, err := parseSource(input)
			checkParserErrors(t, err)
			if len(program.Statements) != 1 {
				t.Fatalf("expected 1 statement, got %d", len(program.Statements))
			}
			stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
			if !ok {
				t.Fatalf("expected *ast.ExpressionStatement, got %T", program.Statements[0])
			}
			_, ok = stmt.Expression.(*ast.RegexLiteral)
			if !ok {
				t.Fatalf("expected *ast.RegexLiteral, got %T", stmt.Expression)
			}
		})
	}
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
			`:"symbol";`,
			`"symbol"`,
		},
		{
			`:'symbol';`,
			`'symbol'`,
		},
		{`:+;`, "+"},
		{`:*;`, "*"},
		{`:-;`, "-"},
		{`:/;`, "/"},
		{`:%;`, "%"},
		{`:**;`, "**"},
		{`:<<;`, "<<"},
		{`:>>;`, ">>"},
		{`:<=>;`, "<=>"},
		{`:==;`, "=="},
		{`:!=;`, "!="},
		{`:=~;`, "=~"},
		{`:!~;`, "!~"},
		{`:===;`, "==="},
		{`:<;`, "<"},
		{`:>;`, ">"},
		{`:<=;`, "<="},
		{`:>=;`, ">="},
		{`:^;`, "^"},
		{`:|;`, "|"},
		{`:&;`, "&"},
		{`:~;`, "~"},
		{`:!;`, "!"},
		{`:[];`, "[]"},
		{`:[]=;`, "[]="},
		{`:end;`, "end"},
		{`:class;`, "class"},
		{`:def;`, "def"},
		{`:if;`, "if"},
		{`:do;`, "do"},
		{`:rescue;`, "rescue"},
	}

	for _, tt := range tests {
		program, err := parseSource(tt.input)
		checkParserErrors(t, err)

		stmt := program.Statements[0].(*ast.ExpressionStatement)
		literal, ok := stmt.Expression.(*ast.SymbolLiteral)
		if !ok {
			t.Fatalf("exp not *ast.SymbolLiteral. got=%T", stmt.Expression)
		}

		if literal.Value.String() != tt.value {
			t.Errorf("literal.Value not %q. got=%q", tt.value, literal.Value)
		}
	}
}

func TestParsingArrayLiterals(t *testing.T) {
	input := "[1, 2 * 2, 3 + 3, {'foo'=>2}]"
	program, err := parseSource(input)
	checkParserErrors(t, err)

	if len(program.Statements) != 1 {
		t.Logf("Expected only one statement, got %d\n", len(program.Statements))
		t.Fail()
	}

	stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
	array, ok := stmt.Expression.(*ast.ArrayLiteral)
	if !ok {
		t.Fatalf("exp not ast.ArrayLiteral. got=%T", stmt.Expression)
	}

	if len(array.Elements) != 4 {
		t.Fatalf("len(array.Elements) not 4. got=%d", len(array.Elements))
	}
	testIntegerLiteral(t, array.Elements[0], 1)
	testInfixExpression(t, array.Elements[1], 2, "*", 2)
	testInfixExpression(t, array.Elements[2], 3, "+", 3)
	testHashLiteral(t, array.Elements[3], map[string]string{"foo": "2"})
}

func TestParsingIndexExpressions(t *testing.T) {
	t.Run("one arg as index", func(t *testing.T) {
		input := "myArray[1 + 1]"
		program, err := parseSource(input)
		checkParserErrors(t, err)

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		indexExp, ok := stmt.Expression.(*ast.IndexExpression)
		if !ok {
			t.Fatalf("exp not *ast.IndexExpression. got=%T", stmt.Expression)
		}

		if !testIdentifier(t, indexExp.Left, "myArray") {
			return
		}

		if !testInfixExpression(t, indexExp.Arguments[0], 1, "+", 1) {
			return
		}
	})
	t.Run("two args as index", func(t *testing.T) {
		t.Run("integers", func(t *testing.T) {
			input := "myArray[1, 1]"
			program, err := parseSource(input)
			checkParserErrors(t, err)

			stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
			indexExp, ok := stmt.Expression.(*ast.IndexExpression)
			if !ok {
				t.Fatalf("exp not *ast.IndexExpression. got=%T", stmt.Expression)
			}

			if !testIdentifier(t, indexExp.Left, "myArray") {
				return
			}

			if !testIntegerLiteral(t, indexExp.Arguments[0], 1) {
				return
			}

			if !testIntegerLiteral(t, indexExp.Arguments[1], 1) {
				return
			}
		})
		t.Run("method calls as index", func(t *testing.T) {
			input := "myArray[foo.bar, 1]"
			program, err := parseSource(input)
			checkParserErrors(t, err)

			stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
			indexExp, ok := stmt.Expression.(*ast.IndexExpression)
			if !ok {
				t.Fatalf("exp not *ast.IndexExpression. got=%T", stmt.Expression)
			}

			if !testIdentifier(t, indexExp.Left, "myArray") {
				return
			}

			index := indexExp.Arguments[0].String()
			if index != "foo.bar" {
				t.Logf("Expected index arg to equal %s, got %s", "foo.bar", index)
				t.Fail()
			}

			if !testIntegerLiteral(t, indexExp.Arguments[1], 1) {
				return
			}
		})
		t.Run("method calls as length", func(t *testing.T) {
			input := "myArray[1, foo.bar]"
			program, err := parseSource(input)
			checkParserErrors(t, err)

			stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
			indexExp, ok := stmt.Expression.(*ast.IndexExpression)
			if !ok {
				t.Fatalf("exp not *ast.IndexExpression. got=%T", stmt.Expression)
			}

			if !testIdentifier(t, indexExp.Left, "myArray") {
				return
			}

			if !testIntegerLiteral(t, indexExp.Arguments[0], 1) {
				return
			}

			length := indexExp.Arguments[1].String()
			if length != "foo.bar" {
				t.Logf("Expected length arg to equal %s, got %s", "foo.bar", length)
				t.Fail()
			}
		})
	})
}

func TestParsingModuleExpressions(t *testing.T) {
	input := "module A\n3\nend\n"

	program, err := parseSource(input)
	checkParserErrors(t, err)

	stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
	_, ok = stmt.Expression.(*ast.ModuleExpression)
	if !ok {
		t.Fatalf("exp not *ast.ModuleExpression. got=%T", stmt.Expression)
	}
}

func TestParsingClassExpressions(t *testing.T) {
	t.Run("basic class", func(t *testing.T) {
		input := "class A\n3\nend\n"

		program, err := parseSource(input)
		checkParserErrors(t, err)

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		class, ok := stmt.Expression.(*ast.ClassExpression)
		if !ok {
			t.Fatalf("exp not *ast.ClassExpression. got=%T", stmt.Expression)
		}

		className := "A"
		if className != class.Name.String() {
			t.Logf("Expected class name to equal %q, got %q\n", className, class.Name.String())
			t.Fail()
		}
	})
	t.Run("class with superclass", func(t *testing.T) {
		input := "class A < B\n3\nend\n"

		program, err := parseSource(input)
		checkParserErrors(t, err)

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		class, ok := stmt.Expression.(*ast.ClassExpression)
		if !ok {
			t.Fatalf("exp not *ast.ClassExpression. got=%T", stmt.Expression)
		}

		className := "A"
		if className != class.Name.Value {
			t.Logf("Expected class name to equal %q, got %q\n", className, class.Name.Value)
			t.Fail()
		}

		superclassName := "B"
		superIdent, ok := class.SuperClass.(*ast.Identifier)
		if !ok || superclassName != superIdent.Value {
			t.Logf("Expected superclass name to equal %q, got %v\n", superclassName, class.SuperClass)
			t.Fail()
		}
	})
	t.Run("downcase class", func(t *testing.T) {
		input := "class a\n3\nend\n"
		_, err := parseSource(input)
		if err == nil {
			t.Errorf("expected error for lowercase class name")
		}
	})
}

func TestParsingSingletonClassExpressions(t *testing.T) {
	t.Run("class << self", func(t *testing.T) {
		input := "class << self\n3\nend\n"

		program, err := parseSource(input)
		checkParserErrors(t, err)

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		sc, ok := stmt.Expression.(*ast.SingletonClassExpression)
		if !ok {
			t.Fatalf("exp not *ast.SingletonClassExpression. got=%T", stmt.Expression)
		}

		selfExpr, ok := sc.Expr.(*ast.Self)
		if !ok {
			t.Fatalf("exp not *ast.Self. got=%T", sc.Expr)
		}
		_ = selfExpr
	})

	t.Run("class << other_expr", func(t *testing.T) {
		input := "class << some_var\n3\nend\n"

		program, err := parseSource(input)
		checkParserErrors(t, err)

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		sc, ok := stmt.Expression.(*ast.SingletonClassExpression)
		if !ok {
			t.Fatalf("exp not *ast.SingletonClassExpression. got=%T", stmt.Expression)
		}

		ident, ok := sc.Expr.(*ast.Identifier)
		if !ok {
			t.Fatalf("exp not *ast.Identifier. got=%T", sc.Expr)
		}
		if ident.Value != "some_var" {
			t.Logf("Expected expr 'some_var', got %q", ident.Value)
			t.Fail()
		}
	})
}

func TestReadSourceIORreader(t *testing.T) {
	// io.Reader path in readSource
	_, err := ParseFile("", strings.NewReader("1"), 0)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	// *bytes.Buffer path
	var buf bytes.Buffer
	buf.WriteString("1")
	_, err = ParseFile("", &buf, 0)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestContextCallScopeOperator(t *testing.T) {
	input := "Foo::bar"
	program, err := parseSource(input)
	checkParserErrors(t, err)
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
	stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
	if !ok {
		t.Fatalf("expected *ast.ExpressionStatement, got %T", program.Statements[0])
	}
	sid, ok := stmt.Expression.(*ast.ScopedIdentifier)
	if !ok {
		t.Fatalf("expected *ast.ScopedIdentifier, got %T", stmt.Expression)
	}
	if sid.Outer == nil || sid.Outer.Value != "Foo" {
		t.Errorf("expected Outer 'Foo', got %v", sid.Outer)
	}
}

func TestContextCallNoArgs(t *testing.T) {
	input := "foo.bar"
	program, err := parseSource(input)
	checkParserErrors(t, err)
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
	stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
	if !ok {
		t.Fatalf("expected *ast.ExpressionStatement, got %T", program.Statements[0])
	}
	cce, ok := stmt.Expression.(*ast.ContextCallExpression)
	if !ok {
		t.Fatalf("expected *ast.ContextCallExpression, got %T", stmt.Expression)
	}
	if len(cce.Arguments) != 0 {
		t.Errorf("expected 0 arguments, got %d", len(cce.Arguments))
	}
}

func TestCallExpressionWithParensNonIdent(t *testing.T) {
	input := "@ivar.call(1)"
	program, err := parseSource(input)
	checkParserErrors(t, err)
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
	stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
	if !ok {
		t.Fatalf("expected *ast.ExpressionStatement, got %T", program.Statements[0])
	}
	_, ok = stmt.Expression.(*ast.ContextCallExpression)
	if !ok {
		t.Fatalf("expected *ast.ContextCallExpression, got %T", stmt.Expression)
	}
}

func TestExceptionHandlingEnsure(t *testing.T) {
	input := "begin\n  1\nensure\n  2\nend\n"
	program, err := parseSource(input)
	checkParserErrors(t, err)
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
	stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
	if !ok {
		t.Fatalf("expected *ast.ExpressionStatement, got %T", program.Statements[0])
	}
	block, ok := stmt.Expression.(*ast.ExceptionHandlingBlock)
	if !ok {
		t.Fatalf("expected *ast.ExceptionHandlingBlock, got %T", stmt.Expression)
	}
	if block.EnsureBody == nil {
		t.Errorf("expected EnsureBody to be set")
	}
}

func TestParseHash(t *testing.T) {
	tests := []struct {
		input   string
		hashMap map[string]string
	}{
		{
			input:   `{"foo" => 42}`,
			hashMap: map[string]string{"foo": "42"},
		},
		{
			input:   `{"foo" => 42, "bar" => "baz"}`,
			hashMap: map[string]string{"foo": "42", "bar": "baz"},
		},
		{
			input:   `{foo: 42}`,
			hashMap: map[string]string{"foo": "42"},
		},
	}

	for _, tt := range tests {
		program, err := parseSource(tt.input)
		checkParserErrors(t, err)

		stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
		if !ok {
			t.Logf("Expected first statement to be *ast.ExpressionStatement, got %T\n", stmt)
			t.FailNow()
		}

		testHashLiteral(t, stmt.Expression, tt.hashMap)
	}
}

func TestFloatLiteralExpression(t *testing.T) {
	tests := []string{"1.5", "0.5", "1.5e10", "1.5E-10"}
	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			program, err := parseSource(input)
			checkParserErrors(t, err)
			if len(program.Statements) != 1 {
				t.Fatalf("expected 1 statement, got %d", len(program.Statements))
			}
			stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
			if !ok {
				t.Fatalf("expected *ast.ExpressionStatement, got %T", program.Statements[0])
			}
			_, ok = stmt.Expression.(*ast.FloatLiteral)
			if !ok {
				t.Fatalf("expected *ast.FloatLiteral, got %T", stmt.Expression)
			}
		})
	}
}

func TestJumpExpression(t *testing.T) {
	tests := []struct {
		input     string
		expectVal bool
	}{
		{input: "break", expectVal: false},
		{input: "next", expectVal: false},
		{input: "redo", expectVal: false},
		{input: "retry", expectVal: false},
		{input: "break 5", expectVal: true},
		{input: "next x", expectVal: true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			program, err := parseSource(tt.input)
			checkParserErrors(t, err)
			if len(program.Statements) != 1 {
				t.Fatalf("expected 1 statement, got %d", len(program.Statements))
			}
			stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
			if !ok {
				t.Fatalf("expected *ast.ExpressionStatement, got %T", program.Statements[0])
			}
			jmp, ok := stmt.Expression.(*ast.JumpExpression)
			if !ok {
				t.Fatalf("expected *ast.JumpExpression, got %T", stmt.Expression)
			}
			if tt.expectVal && jmp.Value == nil {
				t.Errorf("expected Value to be set")
			}
			if !tt.expectVal && jmp.Value != nil {
				t.Errorf("expected Value to be nil, got %v", jmp.Value)
			}
		})
	}
}

func TestDefinedExpression(t *testing.T) {
	tests := []string{"defined? x", "defined?(x)"}
	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			program, err := parseSource(input)
			checkParserErrors(t, err)
			if len(program.Statements) != 1 {
				t.Fatalf("expected 1 statement, got %d", len(program.Statements))
			}
			stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
			if !ok {
				t.Fatalf("expected *ast.ExpressionStatement, got %T", program.Statements[0])
			}
			expr, ok := stmt.Expression.(*ast.DefinedExpression)
			if !ok {
				t.Fatalf("expected *ast.DefinedExpression, got %T", stmt.Expression)
			}
			if expr.Expr == nil {
				t.Errorf("expected Expr to be set")
			}
		})
	}
}

func TestRescueModifier(t *testing.T) {
	input := "x rescue y"
	program, err := parseSource(input)
	checkParserErrors(t, err)
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
	stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
	if !ok {
		t.Fatalf("expected *ast.ExpressionStatement, got %T", program.Statements[0])
	}
	infix, ok := stmt.Expression.(*ast.InfixExpression)
	if !ok {
		t.Fatalf("expected *ast.InfixExpression, got %T", stmt.Expression)
	}
	if infix.Operator != "rescue" {
		t.Errorf("expected operator 'rescue', got %q", infix.Operator)
	}
}

func TestSplatExpression(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"*x", "*x"},
		{"*call(1, 2)", "*call(1, 2)"},
		// Splat absorbs the full expression up to the next comma / hashrocket.
		// Range (.., ...) is exempt from the wrap-when-infix rule so the
		// printer keeps MRI's bare *a..z form.
		{"*a..z", "*a .. z"},
		{"*a...z", "*a ... z"},
		{"*a + b", "*a + b"},
		{"*a * b", "*a * b"},
		{"*a == b", "*a == b"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			program, err := parseSource(tt.input)
			checkParserErrors(t, err)
			if len(program.Statements) != 1 {
				t.Fatalf("expected 1 statement, got %d", len(program.Statements))
			}
			stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
			if !ok {
				t.Fatalf("expected *ast.ExpressionStatement, got %T", program.Statements[0])
			}
			splat, ok := stmt.Expression.(*ast.SplatExpression)
			if !ok {
				t.Fatalf("expected *ast.SplatExpression, got %T", stmt.Expression)
			}
			got := splat.String()
			if got != tt.expected {
				t.Fatalf("wrong String() output: expected=%q, got=%q", tt.expected, got)
			}
		})
	}
}

func TestTopLevelScope(t *testing.T) {
	input := "::Foo"
	program, err := parseSource(input)
	checkParserErrors(t, err)
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
	stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
	if !ok {
		t.Fatalf("expected *ast.ExpressionStatement, got %T", program.Statements[0])
	}
	_, ok = stmt.Expression.(*ast.ScopedIdentifier)
	if !ok {
		t.Fatalf("expected *ast.ScopedIdentifier, got %T", stmt.Expression)
	}
}

func TestClassVariable(t *testing.T) {
	input := "@@foo"
	program, err := parseSource(input)
	checkParserErrors(t, err)
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
	stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
	if !ok {
		t.Fatalf("expected *ast.ExpressionStatement, got %T", program.Statements[0])
	}
	cv, ok := stmt.Expression.(*ast.ClassVariable)
	if !ok {
		t.Fatalf("expected *ast.ClassVariable, got %T", stmt.Expression)
	}
	if cv.Name == nil || cv.Name.Value != "foo" {
		t.Errorf("expected Name.Value 'foo', got %v", cv.Name)
	}
}

func TestKeyword__LINE__(t *testing.T) {
	input := "__LINE__"
	program, err := parseSource(input)
	checkParserErrors(t, err)
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
	stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
	if !ok {
		t.Fatalf("expected *ast.ExpressionStatement, got %T", program.Statements[0])
	}
	_, ok = stmt.Expression.(*ast.IntegerLiteral)
	if !ok {
		t.Fatalf("expected *ast.IntegerLiteral, got %T", stmt.Expression)
	}
}

func TestEncodingKeyword(t *testing.T) {
	input := "__ENCODING__"
	program, err := parseSource(input)
	checkParserErrors(t, err)
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
	stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
	if !ok {
		t.Fatalf("expected *ast.ExpressionStatement, got %T", program.Statements[0])
	}
	enc, ok := stmt.Expression.(*ast.Keyword__ENCODING__)
	if !ok {
		t.Fatalf("expected *ast.Keyword__ENCODING__, got %T", stmt.Expression)
	}
	if enc.Token.Literal != "__ENCODING__" {
		t.Errorf("expected Token.Literal '__ENCODING__', got %q", enc.Token.Literal)
	}
}

func TestErrorSkipPrefixes(t *testing.T) {
	// tokens that map to parseErrorSkip -- all should produce errors
	inputs := []string{"when", "else", ")", "=>"}
	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			_, err := parseSource(input)
			if err == nil {
				t.Errorf("expected error for %q", input)
			}
		})
	}
}

func TestLabelExpression(t *testing.T) {
	input := "foo: 42"
	program, err := parseSource(input)
	checkParserErrors(t, err)
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
	stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
	if !ok {
		t.Fatalf("expected *ast.ExpressionStatement, got %T", program.Statements[0])
	}
	infix, ok := stmt.Expression.(*ast.InfixExpression)
	if !ok {
		t.Fatalf("expected *ast.InfixExpression, got %T", stmt.Expression)
	}
	if infix.Operator != ":" {
		t.Errorf("expected operator ':', got %q", infix.Operator)
	}
}

func TestInstanceVariableNoIdent(t *testing.T) {
	input := "@"
	_, err := parseSource(input)
	if err == nil {
		t.Errorf("expected error for bare @")
	}
}

func TestModuleErrorPaths(t *testing.T) {
	tests := []string{
		"module A\n3",     // missing end
		"module A 3\nend", // missing newline after const
	}
	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			_, err := parseSource(input)
			if err == nil {
				t.Errorf("expected error for %q", input)
			}
		})
	}
}

func TestSetterAssignment(t *testing.T) {
	input := "obj.x = 5"
	program, err := parseSource(input)
	checkParserErrors(t, err)
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
	stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
	if !ok {
		t.Fatalf("expected *ast.ExpressionStatement, got %T", program.Statements[0])
	}
	cce, ok := stmt.Expression.(*ast.ContextCallExpression)
	if !ok {
		t.Fatalf("expected *ast.ContextCallExpression, got %T", stmt.Expression)
	}
	if cce.Function.Value != "x=" {
		t.Errorf("expected Function 'x=', got %q", cce.Function.Value)
	}
	if len(cce.Arguments) != 1 {
		t.Errorf("expected 1 argument, got %d", len(cce.Arguments))
	}
}

func TestContextCallNonIdent(t *testing.T) {
	tests := []string{"@x bar", "$x bar"}
	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			program, err := parseSource(input)
			checkParserErrors(t, err)
			if len(program.Statements) != 1 {
				t.Fatalf("expected 1 statement, got %d", len(program.Statements))
			}
			stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
			if !ok {
				t.Fatalf("expected *ast.ExpressionStatement, got %T", program.Statements[0])
			}
			_, ok = stmt.Expression.(*ast.ContextCallExpression)
			if !ok {
				t.Fatalf("expected *ast.ContextCallExpression, got %T", stmt.Expression)
			}
		})
	}
}

func TestCallBlockOnInfix(t *testing.T) {
	input := "x + y do\nend"
	program, err := parseSource(input)
	checkParserErrors(t, err)
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
	_, ok := program.Statements[0].(*ast.ExpressionStatement)
	if !ok {
		t.Fatalf("expected *ast.ExpressionStatement, got %T", program.Statements[0])
	}
}

func TestInterpolatedRegexEmbexpr(t *testing.T) {
	input := "/foo#{bar}baz/"
	program, err := parseSource(input)
	checkParserErrors(t, err)
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
	stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
	if !ok {
		t.Fatalf("expected *ast.ExpressionStatement, got %T", program.Statements[0])
	}
	rl, ok := stmt.Expression.(*ast.RegexLiteral)
	if !ok {
		t.Fatalf("expected *ast.RegexLiteral, got %T", stmt.Expression)
	}
	if len(rl.Parts) == 0 {
		t.Errorf("expected Parts to be non-empty")
	}
}

func TestParseErrorPaths(t *testing.T) {
	tests := []string{
		"::foo",   // top-level scope without CONST
		"@@",      // class var without IDENT
		"(1",      // grouped expr without closing paren
		"{1 => 2", // hash without closing brace
		"x[1",     // index without closing bracket
	}
	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			_, err := parseSource(input)
			if err == nil {
				t.Errorf("expected error for %q", input)
			}
		})
	}
}

func TestCallBlockErrorPath(t *testing.T) {
	// block on infix where right side is not an identifier -> error
	_, err := parseSource("x + 1 do\nend")
	if err == nil {
		t.Errorf("expected error for block on non-identifier")
	}
}

func TestSingletonClassErrorPath(t *testing.T) {
	_, err := parseSource("class <<\nend")
	if err == nil {
		t.Errorf("expected error for incomplete singleton class")
	}
}

func TestKeywordRestParameter(t *testing.T) {
	input := "def foo(**kwargs)\nend"
	program, err := parseSource(input)
	checkParserErrors(t, err)
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
	stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
	if !ok {
		t.Fatalf("expected *ast.ExpressionStatement, got %T", program.Statements[0])
	}
	fl, ok := stmt.Expression.(*ast.FunctionLiteral)
	if !ok {
		t.Fatalf("expected *ast.FunctionLiteral, got %T", stmt.Expression)
	}
	if len(fl.Parameters) != 1 {
		t.Fatalf("expected 1 param, got %d", len(fl.Parameters))
	}
	if !fl.Parameters[0].IsKeywordRest {
		t.Errorf("expected IsKeywordRest")
	}
	if fl.Parameters[0].Name.Value != "kwargs" {
		t.Errorf("expected name 'kwargs', got %q", fl.Parameters[0].Name.Value)
	}
}

func TestFirstParamKeyword(t *testing.T) {
	input := "def foo(a:)\nend"
	program, err := parseSource(input)
	checkParserErrors(t, err)
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
	stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
	if !ok {
		t.Fatalf("expected *ast.ExpressionStatement, got %T", program.Statements[0])
	}
	fl, ok := stmt.Expression.(*ast.FunctionLiteral)
	if !ok {
		t.Fatalf("expected *ast.FunctionLiteral, got %T", stmt.Expression)
	}
	if len(fl.Parameters) != 1 {
		t.Fatalf("expected 1 param, got %d", len(fl.Parameters))
	}
	if !fl.Parameters[0].IsKeyword {
		t.Errorf("expected IsKeyword")
	}
}

func TestArgumentForwarding(t *testing.T) {
	input := "def foo(...)\nend"
	program, err := parseSource(input)
	checkParserErrors(t, err)
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
	stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
	if !ok {
		t.Fatalf("expected *ast.ExpressionStatement, got %T", program.Statements[0])
	}
	fl, ok := stmt.Expression.(*ast.FunctionLiteral)
	if !ok {
		t.Fatalf("expected *ast.FunctionLiteral, got %T", stmt.Expression)
	}
	if len(fl.Parameters) != 1 {
		t.Fatalf("expected 1 param, got %d", len(fl.Parameters))
	}
	if !fl.Parameters[0].IsForwarding {
		t.Errorf("expected IsForwarding")
	}
}

func TestAnonymousBlockForwarding(t *testing.T) {
	input := "def foo(&)\nend"
	program, err := parseSource(input)
	checkParserErrors(t, err)
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
	stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
	if !ok {
		t.Fatalf("expected *ast.ExpressionStatement, got %T", program.Statements[0])
	}
	fl, ok := stmt.Expression.(*ast.FunctionLiteral)
	if !ok {
		t.Fatalf("expected *ast.FunctionLiteral, got %T", stmt.Expression)
	}
	if fl.CapturedBlock == nil {
		t.Fatalf("expected non-nil CapturedBlock for anonymous &")
	}
	if fl.CapturedBlock.Name != nil {
		t.Errorf("expected nil Name for anonymous &, got %s", fl.CapturedBlock.Name.Value)
	}
}

func TestParseStatementErrorPaths(t *testing.T) {
	// ILLEGAL token path
	_, err := parseSource("\\")
	if err == nil {
		t.Errorf("expected error for illegal character")
	}

	// Return statement error: incomplete expression after return
	_, err2 := parseSource("return x +")
	if err2 == nil {
		t.Errorf("expected error for incomplete return")
	}

	// if x y is a valid bare call: if x(y)
	_, err3 := parseSource("if x y\nend")
	if err3 != nil {
		t.Errorf("bare call in if condition should parse: %v", err3)
	}
}

func TestRightwardAssignment(t *testing.T) {
	tests := []string{
		"1 => x",
		"x => y",
		"foo(1, 2) => result",
	}
	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			program, err := parseSource(input)
			checkParserErrors(t, err)
			if len(program.Statements) != 1 {
				t.Fatalf("expected 1 statement, got %d", len(program.Statements))
			}
			stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
			if !ok {
				t.Fatalf("expected *ast.ExpressionStatement, got %T", program.Statements[0])
			}
			ra, ok := stmt.Expression.(*ast.RightwardAssignment)
			if !ok {
				t.Fatalf("expected *ast.RightwardAssignment, got %T", stmt.Expression)
			}
			if ra.Left == nil || ra.Right == nil {
				t.Errorf("expected Left and Right to be set")
			}
		})
	}
}

func TestCaseInExpression(t *testing.T) {
	input := "case x\nin 1\n  y\nend"
	program, err := parseSource(input)
	checkParserErrors(t, err)
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
	stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
	if !ok {
		t.Fatalf("expected *ast.ExpressionStatement, got %T", program.Statements[0])
	}
	caseExpr, ok := stmt.Expression.(*ast.CaseExpression)
	if !ok {
		t.Fatalf("expected *ast.CaseExpression, got %T", stmt.Expression)
	}
	if len(caseExpr.InClauses) != 1 {
		t.Errorf("expected 1 in clause, got %d", len(caseExpr.InClauses))
	}
	if caseExpr.InClauses[0].Body == nil {
		t.Errorf("expected in clause body")
	}
}

func TestBeginlessRange(t *testing.T) {
	tests := []string{"..5", "...5"}
	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			program, err := parseSource(input)
			checkParserErrors(t, err)
			if len(program.Statements) != 1 {
				t.Fatalf("expected 1 statement, got %d", len(program.Statements))
			}
			stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
			if !ok {
				t.Fatalf("expected *ast.ExpressionStatement, got %T", program.Statements[0])
			}
			infix, ok := stmt.Expression.(*ast.InfixExpression)
			if !ok {
				t.Fatalf("expected *ast.InfixExpression, got %T", stmt.Expression)
			}
			if infix.Left != nil {
				t.Errorf("expected nil Left for beginless range")
			}
		})
	}
}

func TestAliasExpression(t *testing.T) {
	input := "alias new old"
	program, err := parseSource(input)
	checkParserErrors(t, err)
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
	stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
	if !ok {
		t.Fatalf("expected *ast.ExpressionStatement, got %T", program.Statements[0])
	}
	a, ok := stmt.Expression.(*ast.AliasExpression)
	if !ok {
		t.Fatalf("expected *ast.AliasExpression, got %T", stmt.Expression)
	}
	if a.NewName.Value != "new" || a.OldName.Value != "old" {
		t.Errorf("expected new/old, got %q/%q", a.NewName.Value, a.OldName.Value)
	}
}

func TestUndefExpression(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantNames int
	}{
		{name: "single", input: "undef foo", wantNames: 1},
		{name: "multiple", input: "undef foo, bar, baz", wantNames: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			program, err := parseSource(tt.input)
			checkParserErrors(t, err)
			if len(program.Statements) != 1 {
				t.Fatalf("expected 1 statement, got %d", len(program.Statements))
			}
			stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
			if !ok {
				t.Fatalf("expected *ast.ExpressionStatement, got %T", program.Statements[0])
			}
			u, ok := stmt.Expression.(*ast.UndefExpression)
			if !ok {
				t.Fatalf("expected *ast.UndefExpression, got %T", stmt.Expression)
			}
			if len(u.Names) != tt.wantNames {
				t.Errorf("expected %d names, got %d", tt.wantNames, len(u.Names))
			}
		})
	}
}

func TestLambdaExpression(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantParams int
	}{
		{name: "no params", input: "-> { }", wantParams: 0},
		{name: "with params", input: "->(x, y) { }", wantParams: 2},
		{name: "do end body", input: "-> do\nend", wantParams: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			program, err := parseSource(tt.input)
			checkParserErrors(t, err)
			if len(program.Statements) != 1 {
				t.Fatalf("expected 1 statement, got %d", len(program.Statements))
			}
			stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
			if !ok {
				t.Fatalf("expected *ast.ExpressionStatement, got %T", program.Statements[0])
			}
			fl, ok := stmt.Expression.(*ast.FunctionLiteral)
			if !ok {
				t.Fatalf("expected *ast.FunctionLiteral, got %T", stmt.Expression)
			}
			if !fl.IsLambda {
				t.Errorf("expected IsLambda to be true")
			}
			if len(fl.Parameters) != tt.wantParams {
				t.Errorf("expected %d params, got %d", tt.wantParams, len(fl.Parameters))
			}
			if fl.Body == nil {
				t.Errorf("expected Body to be set")
			}
		})
	}
}

func TestSuperExpression(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantArgs int
	}{
		{name: "bare super", input: "super", wantArgs: 0},
		{name: "super with parens", input: "super(1, 2)", wantArgs: 2},
		{name: "super with args no parens", input: "super 1, 2", wantArgs: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			program, err := parseSource(tt.input)
			checkParserErrors(t, err)
			if len(program.Statements) != 1 {
				t.Fatalf("expected 1 statement, got %d", len(program.Statements))
			}
			stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
			if !ok {
				t.Fatalf("expected *ast.ExpressionStatement, got %T", program.Statements[0])
			}
			sup, ok := stmt.Expression.(*ast.SuperExpression)
			if !ok {
				t.Fatalf("expected *ast.SuperExpression, got %T", stmt.Expression)
			}
			if len(sup.Arguments) != tt.wantArgs {
				t.Errorf("expected %d args, got %d", tt.wantArgs, len(sup.Arguments))
			}
		})
	}
}

func TestAssignmentToFILE(t *testing.T) {
	input := "__FILE__ = 5"
	_, err := parseSource(input)
	if err == nil {
		t.Errorf("expected error for assignment to __FILE__")
	}
}

func testExpression(t *testing.T, exp ast.Expression, expected interface{}) bool {
	t.Helper()
	if inf, ok := expected.(infix); ok {
		return testInfixExpression(t, exp, inf.left, inf.operator, inf.right)
	}
	return testLiteralExpression(t, exp, expected)
}

type infix struct {
	left     interface{}
	operator string
	right    interface{}
}

func testInfixExpression(
	t *testing.T,
	exp ast.Expression,
	left interface{},
	operator string,
	right interface{},
) bool {
	t.Helper()
	opExp, ok := exp.(*ast.InfixExpression)
	if !ok {
		t.Errorf("exp is not ast.OperatorExpression. got=%T(%s)", exp, exp)
		return false
	}

	if !testLiteralExpression(t, opExp.Left, left) {
		return false
	}

	if opExp.Operator != operator {
		t.Errorf("exp.Operator is not '%s'. got=%q", operator, opExp.Operator)
		return false
	}

	if !testLiteralExpression(t, opExp.Right, right) {
		return false
	}

	return true
}

func testLiteralExpression(
	t *testing.T,
	exp ast.Expression,
	expected interface{},
) bool {
	t.Helper()
	switch v := expected.(type) {
	case int:
		return testIntegerLiteral(t, exp, int64(v))
	case int64:
		return testIntegerLiteral(t, exp, v)
	case string:
		if strings.HasPrefix(v, ":") {
			return testSymbol(t, exp, strings.TrimPrefix(v, ":"))
		}
		return testIdentifier(t, exp, v)
	case bool:
		return testBooleanLiteral(t, exp, v)
	case map[string]string:
		return testHashLiteral(t, exp, v)
	case []string:
		return testArrayLiteral(t, exp, v)
	case nil:
		return true
	}
	t.Errorf("type of expression not handled. got=%T", exp)
	return false
}

func testStringLiteral(t *testing.T, sl ast.Expression, value string) bool {
	t.Helper()
	str, ok := sl.(*ast.StringLiteral)
	if !ok {
		t.Errorf("expression not *ast.StringLiteral. got=%T", sl)
		return false
	}

	if str.Value != value {
		t.Errorf("str.Value not %s. got=%s", value, str.Value)
		return false
	}

	if str.TokenLiteral() != value {
		t.Errorf(
			"integer.TokenLiteral not %s. got=%s", value,
			str.TokenLiteral(),
		)
		return false
	}

	return true
}

func testIntegerLiteral(t *testing.T, il ast.Expression, value int64) bool {
	t.Helper()
	if prefix, ok := il.(*ast.PrefixExpression); ok {
		if _, ok := prefix.Right.(*ast.IntegerLiteral); !ok {
			t.Errorf("expression not *ast.IntegerLiteral. got=%T", il)
			return false
		}
		if !strings.ContainsAny(prefix.Operator, "+-") {
			t.Errorf("unsupported prefix: %q", prefix.Operator)
			return false
		}
		prefixedInt := fmt.Sprintf("%s%s", prefix.Operator, prefix.Right.String())
		i, err := strconv.ParseInt(prefixedInt, 10, 64)
		if err != nil {
			t.Errorf("could not parse prefix: %v", err)
			return false
		}
		il = &ast.IntegerLiteral{Value: i}
	}
	integ, ok := il.(*ast.IntegerLiteral)
	if !ok {
		t.Errorf("expression not *ast.IntegerLiteral. got=%T", il)
		return false
	}

	if integ.Value != value {
		t.Errorf("integer.Value not %d. got=%d", value, integ.Value)
		return false
	}

	if integ.TokenLiteral() != fmt.Sprintf("%d", value) {
		t.Errorf(
			"integer.TokenLiteral not %d. got=%s", value,
			integ.TokenLiteral(),
		)
		return false
	}

	return true
}

func testGlobal(t *testing.T, exp ast.Expression, value string) bool {
	t.Helper()
	global, ok := exp.(*ast.Global)
	if !ok {
		t.Errorf("exp not *ast.Identifier. got=%T", exp)
		return false
	}

	if global.Value != value {
		t.Errorf("global.Value not %s. got=%s", value, global.Value)
		return false
	}

	if global.TokenLiteral() != value {
		t.Errorf("global.TokenLiteral not %s. got=%s", value,
			global.TokenLiteral())
		return false
	}

	return true
}

func testSymbol(t *testing.T, exp ast.Expression, value string) bool {
	t.Helper()
	symbol, ok := exp.(*ast.SymbolLiteral)
	if !ok {
		t.Errorf("exp not %T. got=%T", symbol, exp)
		return false
	}

	if symbol.Value.String() != value {
		t.Errorf("symbol.Value not %s. got=%s", value, symbol.Value)
		return false
	}

	return true
}

func testIdentifier(t *testing.T, exp ast.Expression, value string) bool {
	t.Helper()
	ident, ok := exp.(*ast.Identifier)
	if !ok {
		t.Errorf("exp not *ast.Identifier. got=%T", exp)
		return false
	}

	if ident.Value != value {
		t.Errorf("ident.Value not %s. got=%s", value, ident.Value)
		return false
	}

	if ident.TokenLiteral() != value {
		t.Errorf("ident.TokenLiteral not %s. got=%s", value,
			ident.TokenLiteral())
		return false
	}

	return true
}

func testBooleanLiteral(t *testing.T, exp ast.Expression, value bool) bool {
	t.Helper()
	bo, ok := exp.(*ast.Boolean)
	if !ok {
		t.Errorf("exp not *ast.Boolean. got=%T", exp)
		return false
	}

	if bo.Value != value {
		t.Errorf("bo.Value not %t. got=%t", value, bo.Value)
		return false
	}

	if bo.TokenLiteral() != fmt.Sprintf("%t", value) {
		t.Errorf("bo.TokenLiteral not %t. got=%s",
			value, bo.TokenLiteral())
		return false
	}

	return true
}

func testArrayLiteral(t *testing.T, expr ast.Expression, value []string) bool {
	t.Helper()
	array, ok := expr.(*ast.ArrayLiteral)
	if !ok {
		t.Fatalf("expr not *ast.ArrayLiteral. got=%T", expr)
		return false
	}

	if len(array.Elements) != len(value) {
		t.Fatalf("len(array.Elements) not %d. got=%d", len(value), len(array.Elements))
	}

	arr := make([]string, len(array.Elements))
	for i, v := range array.Elements {
		arr[i] = v.String()
	}
	if !reflect.DeepEqual(arr, value) {
		t.Logf("Expected array to equal\n%q\n\tgot\n%q\n", value, array)
		return false
	}
	return true
}

func testHashLiteral(t *testing.T, expr ast.Expression, value map[string]string) bool {
	t.Helper()
	hash, ok := expr.(*ast.HashLiteral)
	if !ok {
		t.Errorf("expr not *ast.HashLiteral. got=%T", expr)
		return false
	}
	hashMap := make(map[string]string)
	if hash.Map != nil {
		for _, kv := range hash.Map.Entries() {
			hashMap[kv.Key.String()] = kv.Value.String()
		}
	}

	if !reflect.DeepEqual(hashMap, value) {
		t.Logf("Expected hash to equal\n%q\n\tgot\n%q\n", value, hashMap)
		return false
	}
	return true
}

func TestPercentLiterals(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// %q -- single-quoted string (non-interpolating)
		{"%q parens", "%q(hello)", `'hello'`},
		{"%q braces", "%q{hello world}", `'hello world'`},
		// %Q -- double-quoted string (interpolating)
		{"%Q simple", "%Q(hello)", `"hello"`},
		// bare % -- same as %Q
		{"bare % simple", "%(hello)", `"hello"`},
		// %w -- word array (non-interpolating). Preserved on roundtrip so
		// MRI's SymbolFlags / encoding-tag for ASCII-only literals match.
		{"%w words", "%w[a b c]", "%w[a b c]"},
		{"%w empty", "%w[]", `%w[]`},
		{"%w extra whitespace", "%w[  a  b  ]", "%w[a b]"},
		// %W -- word array (interpolating)
		{"%W words", "%W[a b c]", `%W[a b c]`},
		// %i -- symbol array (non-interpolating)
		{"%i symbols", "%i[foo bar]", "%i[foo bar]"},
		// %I -- symbol array (interpolating)
		{"%I symbols", "%I[foo bar]", "%I[foo bar]"},
		// %s -- symbol literal
		{"%s symbol", "%s(foo)", ":foo"},
		// %r -- regex
		{"%r regex", "%r{pattern}", `/pattern/`},
		// %x -- command
		{"%x command", "%x(ls -la)", "`ls -la`"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.expected == "TODO" {
				t.Skip("TODO")
			}
			program, err := parseSource(tt.input)
			checkParserErrors(t, err)
			if len(program.Statements) == 0 {
				t.Fatal("no statements")
			}
			stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
			if !ok {
				t.Fatalf("expected ExpressionStatement, got %T", program.Statements[0])
			}
			if stmt.Expression.String() != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, stmt.Expression.String())
			}
		})
	}
}

func TestHeredocParsing(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"unquoted heredoc", "<<EOS\nbody\nEOS\n"},
		{"single-quoted heredoc", "<<'EOS'\nbody\nEOS\n"},
		{"double-quoted heredoc", `<<"EOS"` + "\nbody\nEOS\n"},
		{"indented heredoc", "<<-EOS\n  body\n  EOS\n"},
		{"squiggy heredoc", "<<~EOS\n  body\n  EOS\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			program, err := parseSource(tt.input)
			checkParserErrors(t, err)
			if len(program.Statements) == 0 {
				t.Fatal("no statements")
			}
			stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
			if !ok {
				t.Fatalf("expected ExpressionStatement, got %T", program.Statements[0])
			}
			_, ok = stmt.Expression.(*ast.StringLiteral)
			if !ok {
				t.Errorf("expected StringLiteral, got %T", stmt.Expression)
			}
		})
	}
}

func TestSafeNavigation(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"safe nav bare method", "obj&.method"},
		{"safe nav with args", "obj&.method(1, 2)"},
		{"safe nav chained", "obj&.foo&.bar"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			program, err := parseSource(tt.input)
			checkParserErrors(t, err)
			if len(program.Statements) == 0 {
				t.Fatal("no statements")
			}
			stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
			if !ok {
				t.Fatalf("expected ExpressionStatement, got %T", program.Statements[0])
			}
			call, ok := stmt.Expression.(*ast.ContextCallExpression)
			if !ok {
				t.Errorf("expected ContextCallExpression, got %T", stmt.Expression)
			}
			if call != nil && call.Token.Type != token.LONELY {
				t.Errorf("expected LONELY token, got %s", call.Token.Type)
			}
		})
	}
}

func TestEndlessMethod(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"bare endless method", "def foo = 42"},
		{"endless method with params", "def add(x, y) = x + y"},
		{"endless method parens no params", "def foo() = nil"},
		{"endless method with string", `def greet(name) = "Hello, #{name}"`},
		{"endless method semicolon end", "def foo = 42; end"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			program, err := parseSource(tt.input)
			checkParserErrors(t, err)
			if len(program.Statements) == 0 {
				t.Fatal("no statements")
			}
			stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
			if !ok {
				t.Fatalf("expected ExpressionStatement, got %T", program.Statements[0])
			}
			fn, ok := stmt.Expression.(*ast.FunctionLiteral)
			if !ok {
				t.Fatalf("expected FunctionLiteral, got %T", stmt.Expression)
			}
			if fn.Body == nil || len(fn.Body.Statements) == 0 {
				t.Error("endless method body is empty")
			}
		})
	}
}

func TestBacktickXStr(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"backtick command", "`ls -la`"},
		{"backtick with interpolation", "`echo #{name}`"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			program, err := parseSource(tt.input)
			checkParserErrors(t, err)
			if len(program.Statements) == 0 {
				t.Fatal("no statements")
			}
			_, ok := program.Statements[0].(*ast.ExpressionStatement)
			if !ok {
				t.Fatalf("expected ExpressionStatement, got %T", program.Statements[0])
			}
		})
	}
}

func parseSource(src string, modes ...Mode) (*ast.Program, *Errors) {
	mode := parseMode
	for _, m := range modes {
		mode = mode | m
	}
	prog, err := ParseFile("", src, mode)
	var parserErrors *Errors
	if err != nil {
		parserErrors = err.(*Errors)
	}
	return prog, parserErrors
}

func parseExpression(src string, modes ...Mode) (ast.Expression, *Errors) {
	mode := parseMode
	for _, m := range modes {
		mode = mode | m
	}
	expr, err := ParseExprFrom("", src, mode)
	var parserErrors *Errors
	if err != nil {
		parserErrors = err.(*Errors)
	}
	return expr, parserErrors
}

func compareFirstParserError(t *testing.T, expected, actual error) {
	t.Helper()
	if expected == nil && actual == nil {
		return
	}
	parserErrors, ok := actual.(*Errors)
	if parserErrors == nil && expected == nil {
		return
	}
	if !ok {
		t.Logf("Unexpected parser error: %T:%v\n", actual, actual)
		t.FailNow()
	}
	if expected == nil && parserErrors != nil {
		t.Logf("Expected no error, got %T:%v", actual, actual)
		t.FailNow()
	}
	firstErr := parserErrors.errors[0]
	err := firstErr.Error()
	firstSpace := strings.Index(err, " ")
	err = err[firstSpace+1:]
	if err != expected.Error() {
		t.Logf("Expected first parser error to equal %v, got %v", expected, firstErr)
		t.FailNow()
	}
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
	parserErrors, ok := err.(*Errors)
	if parserErrors == nil {
		return
	}
	if !ok {
		t.Logf("Unexpected parser error: %T:%v\n", err, err)
		t.FailNow()
	}

	type stackTracer interface {
		StackTrace() errors.StackTrace
	}

	t.Errorf("parser has %d errors", len(parserErrors.errors))
	for _, e := range parserErrors.errors {
		t.Errorf("%v", e)
		if stackErr, ok := e.(stackTracer); ok && printStack {
			st := stackErr.StackTrace()
			fmt.Printf("Error stack:%+v\n", st[0:2]) // top two frames
		}

	}
	t.FailNow()
}

var rubyExtraExpectFail = map[string]string{}

func TestRubyExtraFixtures(t *testing.T) {
	extraDir := "../internal/integrationtest/testdata/ruby-extra/parser"
	entries, err := os.ReadDir(extraDir)
	if err != nil {
		t.Fatalf("cannot read ruby-extra dir: %v", err)
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".rb") {
			continue
		}
		t.Run(e.Name(), func(t *testing.T) {
			path := extraDir + "/" + e.Name()

			// per-file timeout to catch parser hangs (5s wall-clock)
			type result struct {
				prog *ast.Program
				err  error
			}
			done := make(chan result, 1)
			go func() {
				prog, err := ParseFile(path, nil, AllErrors|ParseComments)
				done <- result{prog, err}
			}()

			var prog *ast.Program
			var perr error
			select {
			case r := <-done:
				prog, perr = r.prog, r.err
			case <-time.After(5 * time.Second):
				t.Fatalf("TIMEOUT: parser hung on %s", e.Name())
			}

			reason, expectFail := rubyExtraExpectFail[e.Name()]

			if expectFail {
				if perr == nil && prog != nil {
					t.Errorf("STALE EXPECTED-FAIL: %s now parses successfully (reason: %s)",
						e.Name(), reason)
				}
				return
			}

			if perr != nil {
				t.Errorf("UNEXPECTED FAIL: %s should parse but got error:\n%v",
					e.Name(), perr)
				return
			}
			if prog == nil {
				t.Errorf("UNEXPECTED FAIL: %s returned nil program", e.Name())
				return
			}

			// String() must not panic
			_ = prog.String()

			// Walk must not panic
			ast.Walk(ast.VisitorFunc(func(n ast.Node) ast.Visitor {
				if n != nil {
					_ = n.String()
				}
				return nil
			}), prog)
		})
	}
}

func TestParserWithVersion(t *testing.T) {
	prog, err := ParseFile("", "x = 1", 0,
		WithVersion(token.MustParseVersion("3.0")))
	if err != nil {
		t.Fatalf("ParseFile with version: %v", err)
	}
	if prog == nil {
		t.Fatal("expected non-nil program")
	}
	if len(prog.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(prog.Statements))
	}
}

func TestParserRegressions(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{"multiline def params", "def foo(a,\n  b,\n  c)\n  1\nend"},
		{"multiline lambda params", "x = ->(a,\n  b) { a + b }"},
		{"multiline block params", "x.each {|a,\n  b| puts a }"},
		{"bare def params multiline", "def foo a,\n  b\n  1\nend"},
		{"backtick method def", "class Foo\n  def `(cmd)\n    cmd\n  end\nend"},
		{"backtick symbol", "x = :`"},
		{"def on nil", "def nil.foo; 1; end"},
		{"def on true", "def true.bar; 2; end"},
		{"setter method", "class Foo\n  def bar=(v)\n    @bar = v\n  end\nend"},
		{"setter method inline", "def n.count=(v); @count=v; end"},
		{"endless vs setter", "def foo = 42"},
		{"bang label key", "x = {save!: true}"},
		{"question label key", "x = {valid?: false}"},
		{"string label in hash", "x = {\"key\": 1}"},
		{"string label in call", "foo(\"key\": 1)"},
		{"string label after regular", "foo(a: 1, \"b\": 2)"},
		{"shovel assignment rhs", "ary << x = 1"},
		{"return in brace block", "loop{return}"},
		{"return in brace block value", "loop{return 42}"},
		{"block in expression list", "foo(bar(x) {1}, y)"},
		{"keyword method name after dot", "def FOO.class; end"},
		{"inline case when", "case x when Integer\n  true\nend"},
		{"inline case in", "case x\nin 1 then :a\nend"},
		{"rescue with int", "begin; raise; rescue 1; end"},
		{"rescue with splat", "begin; raise; rescue *arr; end"},
		{"alias in brace block", "x { alias foo bar }"},
		{"super() in brace block", "x { super() }"},
		{"super() with block", "super() do; end"},
		{"refine without block", "Module.new do\n  refine Foo\nend"},
		{"explicit .[]= with block", "x.[]=(nil, 1){}"},
		{"label after dot", "x.const_set:RUBY, val"},
		{"multi-digit global", "x = $11"},
		{"global symbol", "x = :$11"},
		{"char literal unicode", "x = ?\\u0041"},
		{"char literal octal", "x = ?\\000"},
		{"percent brace interp depth", "foo(%{{#{x} => y}}, z)"},
		{"percent newline not delim", "x = S(\"%%bar\") %\n[1]"},
		{"empty %s()", "x = %s()"},
		{"implicit array pattern", "case [1,2]\nin a, b\n  true\nend"},
		{"implicit array trailing comma", "case [0]\nin 0,;\n  true\nend"},
		{"pattern trailing comma array", "case [0]\nin [0,]\n  true\nend"},
		{"pattern hash trailing comma", "case {a: 0}\nin {a: 0,}\n  true\nend"},
		{"pattern label omission", "case {a: 0}\nin {a:,}\n  true\nend"},
		{"pattern string label", "case {\"a\" => 0}\nin \"a\": 0\n  true\nend"},
		{"pattern string label omission", "case {\"a\" => 0}\nin \"a\":;\n  true\nend"},
		{"pattern range", "case 5\nin 0..10\n  true\nend"},
		{"pattern hash multiline value", "case {a: 2}\nin {a:\n  2}\n  true\nend"},
		{"label omission in brackets", "x = Foo[a:]"},
		{"if bare call", "if oob? x\n  1\nend"},
		{"if not bare call", "if !oob? x\n  1\nend"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseSource(tt.src)
			if err != nil {
				t.Errorf("failed to parse %q: %v", tt.name, err)
			}
		})
	}
}
