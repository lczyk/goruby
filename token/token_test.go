package token

import (
	"testing"

	"github.com/lczyk/assert"
)

func TestTypeString(t *testing.T) {
	tests := []struct {
		typ  Type
		want string
	}{
		{ILLEGAL, "ILLEGAL"},
		{EOF, "EOF"},
		{IDENT, "IDENT"},
		{CONST, "CONST"},
		{ASSIGN, "="},
		{PLUS, "+"},
		{NEWLINE, "NEWLINE"},
		{IF, "if"},
		{END, "end"},
		{Type(999), "token(999)"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.typ.String(), tt.want)
	}
}

func TestLookupIdent(t *testing.T) {
	tests := []struct {
		ident string
		want  Type
	}{
		{"if", IF},
		{"unless", UNLESS},
		{"end", END},
		{"def", DEF},
		{"class", CLASS},
		{"Foo", CONST},
		{"Bar", CONST},
		{"foo", IDENT},
		{"_foo", IDENT},
		{"", IDENT},
	}
	for _, tt := range tests {
		assert.Equal(t, LookupIdent(tt.ident), tt.want)
	}
}

func TestNewToken(t *testing.T) {
	tok := NewToken(IDENT, "foo", 42)
	assert.Equal(t, tok.Type, IDENT)
	assert.Equal(t, tok.Pos, 42)
	assert.Equal(t, int(tok.End), 3) // End derived from len("foo")
}

func TestTokenIsLiteral(t *testing.T) {
	assert.That(t, Token{Type: INT}.IsLiteral(), "INT should be a literal")
	assert.That(t, !Token{Type: ASSIGN}.IsLiteral(), "ASSIGN should not be a literal")
}

func TestTokenIsOperator(t *testing.T) {
	assert.That(t, Token{Type: PLUS}.IsOperator(), "PLUS should be an operator")
	assert.That(t, !Token{Type: IDENT}.IsOperator(), "IDENT should not be an operator")
}

func TestTokenIsAssignOperator(t *testing.T) {
	assert.That(t, Token{Type: ADDASSIGN}.IsAssignOperator(), "ADDASSIGN should be an assign operator")
	assert.That(t, !Token{Type: PLUS}.IsAssignOperator(), "PLUS should not be an assign operator")
}

func TestTokenIsKeyword(t *testing.T) {
	assert.That(t, Token{Type: IF}.IsKeyword(), "IF should be a keyword")
	assert.That(t, !Token{Type: IDENT}.IsKeyword(), "IDENT should not be a keyword")
}

func TestTypeIsLiteral(t *testing.T) {
	assert.That(t, INT.IsLiteral(), "INT should be literal")
	assert.That(t, !ASSIGN.IsLiteral(), "ASSIGN should not be literal")
}

func TestTypeIsOperator(t *testing.T) {
	assert.That(t, PLUS.IsOperator(), "PLUS should be operator")
	assert.That(t, !IDENT.IsOperator(), "IDENT should not be operator")
}

func TestTypeIsAssignOperator(t *testing.T) {
	assert.That(t, ADDASSIGN.IsAssignOperator(), "ADDASSIGN should be assign operator")
	assert.That(t, !PLUS.IsAssignOperator(), "PLUS should not be assign operator")
}

func TestTypeIsKeyword(t *testing.T) {
	assert.That(t, IF.IsKeyword(), "IF should be keyword")
	assert.That(t, !IDENT.IsKeyword(), "IDENT should not be keyword")
}
