package ast

import (
	"testing"

	"github.com/lczyk/goruby/token"
)

func Test_Equal(t *testing.T) {
	intLiteralPointer := &IntegerLiteral{Value: 89}

	testCases := []struct {
		desc  string
		x, y  Node
		equal bool
	}{
		{
			"pointer equal",
			intLiteralPointer,
			intLiteralPointer,
			true,
		},
		{
			"literal value equal",
			&IntegerLiteral{Value: 17},
			&IntegerLiteral{Value: 17},
			true,
		},
		{
			"literal value with same tokens equal",
			&IntegerLiteral{Value: 17, Token: token.NewToken(token.INT, "17", 32)},
			&IntegerLiteral{Value: 17, Token: token.NewToken(token.INT, "17", 32)},
			true,
		},
		{
			"literal value with different tokens equal",
			&IntegerLiteral{Value: 17, Token: token.NewToken(token.INT, "17", 32)},
			&IntegerLiteral{Value: 17, Token: token.NewToken(token.INT, "17", 68)},
			true,
		},
		{
			"complex ast value equal",
			&InfixExpression{
				Left:     &IntegerLiteral{Value: 17},
				Operator: "+",
				Right:    &IntegerLiteral{Value: 42},
			},
			&InfixExpression{
				Left:     &IntegerLiteral{Value: 17},
				Operator: "+",
				Right:    &IntegerLiteral{Value: 42},
			},
			true,
		},
		{
			"complex ast value not equal",
			&InfixExpression{
				Left:     &IntegerLiteral{Value: 17},
				Operator: "+",
				Right:    &IntegerLiteral{Value: 42},
			},
			&InfixExpression{
				Left:     &IntegerLiteral{Value: 22},
				Operator: "-",
				Right:    &IntegerLiteral{Value: 54},
			},
			false,
		},
		{
			"complex ast value with different Tokens equal",
			&InfixExpression{
				Token:    token.NewToken(token.PLUS, "+", 77),
				Left:     &IntegerLiteral{Value: 17},
				Operator: "+",
				Right:    &IntegerLiteral{Value: 42},
			},
			&InfixExpression{
				Token:    token.NewToken(token.PLUS, "+", 999),
				Left:     &IntegerLiteral{Value: 17},
				Operator: "+",
				Right:    &IntegerLiteral{Value: 42},
			},
			true,
		},
		{
			"different ast nodes with same stringification do not equal",
			&ExpressionStatement{
				Expression: &InfixExpression{
					Token:    token.NewToken(token.PLUS, "+", 77),
					Left:     &IntegerLiteral{Value: 17},
					Operator: "+",
					Right:    &IntegerLiteral{Value: 42},
				},
			},
			&InfixExpression{
				Token:    token.NewToken(token.PLUS, "+", 999),
				Left:     &IntegerLiteral{Value: 17},
				Operator: "+",
				Right:    &IntegerLiteral{Value: 42},
			},
			false,
		},
		{
			"same ast nodes with same stringification but different content do not equal",
			&BlockStatement{
				Statements: []Statement{
					&ExpressionStatement{
						Expression: &InfixExpression{
							Token:    token.NewToken(token.PLUS, "+", 77),
							Left:     &IntegerLiteral{Value: 17},
							Operator: "+",
							Right:    &IntegerLiteral{Value: 42},
						},
					},
					&ExpressionStatement{
						Expression: &IntegerLiteral{Value: 3},
					},
					&ExpressionStatement{
						Expression: &IntegerLiteral{Value: 2},
					},
				},
			},
			&BlockStatement{
				Statements: []Statement{
					&ExpressionStatement{
						Expression: &InfixExpression{
							Token:    token.NewToken(token.PLUS, "+", 77),
							Left:     &IntegerLiteral{Value: 17},
							Operator: "+",
							Right:    &IntegerLiteral{Value: 42},
						},
					},
					&ExpressionStatement{
						Expression: &IntegerLiteral{Value: 2},
					},
				},
			},
			false,
		},
	}

	for _, tt := range testCases {
		tt := tt
		t.Run(tt.desc, func(t *testing.T) {
			t.Parallel()
			ok := Equal(tt.x, tt.y)

			if ok != tt.equal {
				t.Logf("Expected Equal to return %t, got %t", tt.equal, ok)
				t.Fail()
			}
		})
	}
}

func Test_compare(t *testing.T) {
	// compare with 3+ nodes hits the next != nil branch
	a := &Identifier{Value: "x"}
	b := &Identifier{Value: "x"}
	c := &Identifier{Value: "x"}
	if !compare(a, b, c) {
		t.Errorf("expected compare to return true for three equal identifiers")
	}
	d := &Identifier{Value: "y"}
	if compare(a, b, d) {
		t.Errorf("expected compare to return false for mismatched third arg")
	}
	// len(nodes) < 2 returns false
	if compare() {
		t.Errorf("expected compare() to return false")
	}
	if compare(a) {
		t.Errorf("expected compare with 1 node to return false")
	}
	// type mismatch between pair
	il := &IntegerLiteral{Value: 1}
	if compare(a, il) {
		t.Errorf("expected compare to return false for type mismatch")
	}
}
