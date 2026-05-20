package ast

import (
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/goruby/token"
)

func TestString(t *testing.T) {
	program := &Program{
		Statements: []Statement{
			&ExpressionStatement{
				Expression: &Assignment{
					Left: &Identifier{
						Token: token.Token{Type: token.IDENT},
						Value: "myVar",
					},
					Right: &Identifier{
						Token: token.Token{Type: token.IDENT},
						Value: "anotherVar",
					},
				},
			},
		},
	}
	assert.Equal(t, program.String(), "myVar = anotherVar")
}
