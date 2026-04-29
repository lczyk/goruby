package token

import (
	"testing"

	"github.com/lczyk/assert"
)

func filterEmpty(arr []string) []string {
	var filtered []string
	for _, repr := range arr {
		if repr != "" {
			filtered = append(filtered, repr)
		}
	}
	return filtered
}
func findMissingSkipEmpty(present []string, arr []string) []string {
	present_map := make(map[string]bool)
	for _, repr := range present {
		present_map[repr] = true
	}

	// Find missing strings
	var missing []string
	for _, repr := range arr {
		if _, ok := present_map[repr]; !ok {
			missing = append(missing, repr)
		}
	}

	return missing
}

func TestTypeSting(t *testing.T) {
	tests := []struct {
		tk   Type
		str  string
		repr string
	}{
		{tk: ILLEGAL, str: "ILLEGAL", repr: "ILLEGAL"},
		{tk: EOF, str: "EOF", repr: "EOF"},
		{tk: IDENT, str: "IDENT", repr: "IDENT"},
		{tk: INT, str: "INT", repr: "INT"},
		{tk: FLOAT, str: "FLOAT", repr: "FLOAT"},
		{tk: STRING, str: "STRING", repr: "STRING"},
		//
		{tk: ASSIGN, str: "ASSIGN", repr: "="},
		{tk: ADDASSIGN, str: "ADDASSIGN", repr: "+="},
		{tk: SUBASSIGN, str: "SUBASSIGN", repr: "-="},
		{tk: MULASSIGN, str: "MULASSIGN", repr: "*="},
		{tk: DIVASSIGN, str: "DIVASSIGN", repr: "/="},
		{tk: MODASSIGN, str: "MODASSIGN", repr: "%="},
		//
		{tk: PLUS, str: "PLUS", repr: "+"},
		{tk: MINUS, str: "MINUS", repr: "-"},
		{tk: BANG, str: "BANG", repr: "!"},
		{tk: ASTERISK, str: "ASTERISK", repr: "*"},
		{tk: SLASH, str: "SLASH", repr: "/"},
		{tk: MODULO, str: "MODULO", repr: "%"},
		{tk: AND, str: "AND", repr: "&"},
		{tk: LOGICALAND, str: "LOGICALAND", repr: "&&"},
		{tk: PIPE, str: "PIPE", repr: "|"},
		{tk: LOGICALOR, str: "LOGICALOR", repr: "||"},
		//
		{tk: LT, str: "LT", repr: "<"},
		{tk: LTE, str: "LTE", repr: "<="},
		{tk: GT, str: "GT", repr: ">"},
		{tk: GTE, str: "GTE", repr: ">="},
		{tk: EQ, str: "EQ", repr: "=="},
		{tk: NOTEQ, str: "NOTEQ", repr: "!="},
		{tk: SPACESHIP, str: "SPACESHIP", repr: "<=>"},
		{tk: LSHIFT, str: "LSHIFT", repr: "<<"},
		//
		{tk: HASHROCKET, str: "HASHROCKET", repr: "=>"},
		{tk: LAMBDAROCKET, str: "LAMBDAROCKET", repr: "->"},
		//
		{tk: NEWLINE, str: "NEWLINE", repr: "\\n"},
		{tk: COMMA, str: "COMMA", repr: ","},
		{tk: SEMICOLON, str: "SEMICOLON", repr: ";"},
		{tk: COMMENT, str: "COMMENT", repr: "# ..."},
		//
		{tk: DOT, str: "DOT", repr: "."},
		{tk: DDOT, str: "DDOT", repr: ".."},
		{tk: DDDOT, str: "DDDOT", repr: "..."},
		{tk: COLON, str: "COLON", repr: ":"},
		{tk: LPAREN, str: "LPAREN", repr: "("},
		{tk: RPAREN, str: "RPAREN", repr: ")"},
		{tk: LBRACE, str: "LBRACE", repr: "{"},
		{tk: RBRACE, str: "RBRACE", repr: "}"},
		{tk: LBRACKET, str: "LBRACKET", repr: "["},
		{tk: SLBRACKET, str: "SLBRACKET", repr: "_["},
		{tk: RBRACKET, str: "RBRACKET", repr: "]"},
		//
		{tk: QMARK, str: "QMARK", repr: "?"},
		{tk: SYMBOL, str: "SYMBOL", repr: ":"},
		{tk: POW, str: "POW", repr: "**"},
		//
		{tk: DEF, str: "DEF", repr: "def"},
		{tk: END, str: "END", repr: "end"},
		{tk: IF, str: "IF", repr: "if"},
		{tk: THEN, str: "THEN", repr: "then"},
		{tk: ELSE, str: "ELSE", repr: "else"},
		{tk: ELSIF, str: "ELSIF", repr: "elsif"},
		{tk: UNLESS, str: "UNLESS", repr: "unless"},
		{tk: TRUE, str: "TRUE", repr: "true"},
		{tk: FALSE, str: "FALSE", repr: "false"},
		{tk: RETURN, str: "RETURN", repr: "return"},
		{tk: NIL, str: "NIL", repr: "nil"},
		// {tk: WHILE, str: "WHILE", repr: "while"},
		{tk: LOOP, str: "LOOP", repr: "loop"},
		{tk: BREAK, str: "BREAK", repr: "break"},
	}

	seen := make(map[Type]bool)

	for _, test := range tests {
		assert.That(t, !seen[test.tk], "Duplicate token %s in tests!", test.tk)
		seen[test.tk] = true

		assert.Equal(t, test.tk.String(), test.str)
		assert.Equal(t, ToType(test.str), test.tk)
		assert.NotEqual(t, test.repr, "")
		assert.Equal(t, type_reprs[test.tk], test.repr)
	}

	test_strings := make([]string, 0)
	test_reprs := make([]string, 0)
	for _, test := range tests {
		test_strings = append(test_strings, test.str)
		test_reprs = append(test_reprs, test.repr)
	}

	missing := findMissingSkipEmpty(test_reprs, filterEmpty(type_reprs[:]))
	assert.Equal(t, len(missing), 0)

	missing = findMissingSkipEmpty(test_strings, filterEmpty(type_strings[:]))
	assert.Equal(t, len(missing), 0)

}

func TestIllegalTypeIsZero(t *testing.T) {
	assert.Equal(t, ILLEGAL, 0)
}

func TestTypeCount(t *testing.T) {

	n_type_reprs := len(filterEmpty(type_reprs[:]))
	n_type_strings := len(filterEmpty(type_strings[:]))
	assert.Equal(t, type_count, n_type_reprs)
	assert.Equal(t, type_count, n_type_strings)
}
