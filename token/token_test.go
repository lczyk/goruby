package token

import "testing"

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
		if got := tt.typ.String(); got != tt.want {
			t.Errorf("%s.String() = %q, want %q", tt.want, got, tt.want)
		}
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
		if got := LookupIdent(tt.ident); got != tt.want {
			t.Errorf("LookupIdent(%q) = %s, want %s", tt.ident, got, tt.want)
		}
	}
}

func TestNewToken(t *testing.T) {
	tok := NewToken(IDENT, "foo", 42)
	if tok.Type != IDENT {
		t.Errorf("Type = %s, want IDENT", tok.Type)
	}
	if tok.Literal != "foo" {
		t.Errorf("Literal = %q, want foo", tok.Literal)
	}
	if tok.Pos != 42 {
		t.Errorf("Pos = %d, want 42", tok.Pos)
	}
}

func TestTokenIsLiteral(t *testing.T) {
	tok := Token{Type: INT}
	if !tok.IsLiteral() {
		t.Error("INT should be a literal")
	}
	tok2 := Token{Type: ASSIGN}
	if tok2.IsLiteral() {
		t.Error("ASSIGN should not be a literal")
	}
}

func TestTokenIsOperator(t *testing.T) {
	tok := Token{Type: PLUS}
	if !tok.IsOperator() {
		t.Error("PLUS should be an operator")
	}
	tok2 := Token{Type: IDENT}
	if tok2.IsOperator() {
		t.Error("IDENT should not be an operator")
	}
}

func TestTokenIsAssignOperator(t *testing.T) {
	tok := Token{Type: ADDASSIGN}
	if !tok.IsAssignOperator() {
		t.Error("ADDASSIGN should be an assign operator")
	}
	tok2 := Token{Type: PLUS}
	if tok2.IsAssignOperator() {
		t.Error("PLUS should not be an assign operator")
	}
}

func TestTokenIsKeyword(t *testing.T) {
	tok := Token{Type: IF}
	if !tok.IsKeyword() {
		t.Error("IF should be a keyword")
	}
	tok2 := Token{Type: IDENT}
	if tok2.IsKeyword() {
		t.Error("IDENT should not be a keyword")
	}
}

func TestTypeIsLiteral(t *testing.T) {
	if !INT.IsLiteral() {
		t.Error("INT should be literal")
	}
	if ASSIGN.IsLiteral() {
		t.Error("ASSIGN should not be literal")
	}
}

func TestTypeIsOperator(t *testing.T) {
	if !PLUS.IsOperator() {
		t.Error("PLUS should be operator")
	}
	if IDENT.IsOperator() {
		t.Error("IDENT should not be operator")
	}
}

func TestTypeIsAssignOperator(t *testing.T) {
	if !ADDASSIGN.IsAssignOperator() {
		t.Error("ADDASSIGN should be assign operator")
	}
	if PLUS.IsAssignOperator() {
		t.Error("PLUS should not be assign operator")
	}
}

func TestTypeIsKeyword(t *testing.T) {
	if !IF.IsKeyword() {
		t.Error("IF should be keyword")
	}
	if IDENT.IsKeyword() {
		t.Error("IDENT should not be keyword")
	}
}
