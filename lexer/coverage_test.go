package lexer

import (
	"testing"

	"github.com/lczyk/goruby/token"
)

func TestLexerLiteralHeredocBody(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []struct {
			typ     token.Type
			literal string
		}
	}{
		{
			name:  "literal heredoc with content",
			input: "<<'EOS'\nhello\nworld\nEOS\n",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING, "hello\nworld\n"},
				{token.NEWLINE, "\n"},
				{token.EOF, ""},
			},
		},
		{
			name:  "squiggy literal heredoc with indent",
			input: "<<~'EOS'\n  hello\n  world\n  EOS\n",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING, "hello\nworld\n"},
				{token.NEWLINE, "\n"},
				{token.EOF, ""},
			},
		},
		{
			name:  "indented literal heredoc with tabs",
			input: "<<-'EOS'\n\tcontent\n\tEOS\n",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING, "\tcontent\n"},
				{token.NEWLINE, "\n"},
				{token.EOF, ""},
			},
		},
		{
			name:  "literal heredoc empty body",
			input: "<<'EOS'\nEOS\n",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING, ""},
				{token.NEWLINE, "\n"},
				{token.EOF, ""},
			},
		},
		{
			name:  "literal squiggy heredoc blank lines ignored",
			input: "<<~'EOS'\n\n  content\n  \n  EOS\n",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING, "\ncontent\n\n"},
				{token.NEWLINE, "\n"},
				{token.EOF, ""},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := New(tt.input)
			for i, exp := range tt.expected {
				if !l.HasNext() {
					t.Fatalf("pos %d: unexpected EOF (expected %s %q)", i, exp.typ, exp.literal)
				}
				tok := l.NextToken()
				if tok.Type != exp.typ {
					t.Errorf("pos %d: expected type %s, got %s (%q)", i, exp.typ, tok.Type, tok.Literal)
				}
				if tok.Literal != exp.literal {
					t.Errorf("pos %d: expected literal %q, got %q", i, exp.literal, tok.Literal)
				}
			}
		})
	}
}

func TestLexerBacktickInterpolation(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []struct {
			typ     token.Type
			literal string
		}
	}{
		{
			name:  "#@var interpolation in backtick",
			input: "`echo #@name`",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.XSTR_BEG, ""},
				{token.XSTR_CONTENT, "echo "},
				{token.AT, "@"},
				{token.IDENT, "name"},
				{token.XSTR_END, "`"},
			},
		},
		{
			name:  "#@@cvar interpolation in backtick",
			input: "`echo #@@var`",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.XSTR_BEG, ""},
				{token.XSTR_CONTENT, "echo "},
				{token.CLASS_VAR, "@@"},
				{token.IDENT, "var"},
				{token.XSTR_END, "`"},
			},
		},
		{
			name:  "escaped backslash in backtick",
			input: "`\\\\path`",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.XSTR_BEG, ""},
				{token.XSTR_CONTENT, "\\\\path"},
				{token.XSTR_END, "`"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := New(tt.input)
			for i, exp := range tt.expected {
				if !l.HasNext() {
					t.Fatalf("pos %d: unexpected EOF (expected %s %q)", i, exp.typ, exp.literal)
				}
				tok := l.NextToken()
				if tok.Type != exp.typ {
					t.Errorf("pos %d: expected type %s, got %s (%q)", i, exp.typ, tok.Type, tok.Literal)
				}
				if tok.Literal != exp.literal {
					t.Errorf("pos %d: expected literal %q, got %q", i, exp.literal, tok.Literal)
				}
			}
		})
	}
}

func TestLexerWithVersion(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		version token.RubyVersion
		expects []struct {
			typ     token.Type
			literal string
		}
	}{
		{
			name:    "lexer with explicit version",
			input:   "x = 1",
			version: token.MustParseVersion("3.0"),
			expects: []struct {
				typ     token.Type
				literal string
			}{
				{token.IDENT, "x"},
				{token.ASSIGN, "="},
				{token.INT, "1"},
			},
		},
		{
			name:    "lexer with 1.9 version",
			input:   "-> { 1 }",
			version: token.MustParseVersion("1.9"),
			expects: []struct {
				typ     token.Type
				literal string
			}{
				{token.LAMBDA, "->"},
				{token.LBRACE, "{"},
				{token.INT, "1"},
				{token.RBRACE, "}"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := New(tt.input, WithVersion(tt.version))
			for i, exp := range tt.expects {
				if !l.HasNext() {
					t.Fatalf("pos %d: unexpected EOF (expected %s %q)", i, exp.typ, exp.literal)
				}
				tok := l.NextToken()
				if tok.Type != exp.typ {
					t.Errorf("pos %d: expected type %s, got %s (%q)", i, exp.typ, tok.Type, tok.Literal)
				}
				if tok.Literal != exp.literal {
					t.Errorf("pos %d: expected literal %q, got %q", i, exp.literal, tok.Literal)
				}
			}
		})
	}
}

func TestLexerUncoveredOperators(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []struct {
			typ     token.Type
			literal string
		}
	}{
		{
			name:  "tilde operator",
			input: "~x",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.TILDE, "~"},
				{token.IDENT, "x"},
			},
		},
		{
			name:  "xor operator standalone",
			input: "a ^ b",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.IDENT, "a"},
				{token.XOR, "^"},
				{token.IDENT, "b"},
			},
		},
		{
			name:  "range operator",
			input: "1..10",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.INT, "1"},
				{token.RANGE, ".."},
				{token.INT, "10"},
			},
		},
		{
			name:  "range exclusive operator",
			input: "1...10",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.INT, "1"},
				{token.RANGEEX, "..."},
				{token.INT, "10"},
			},
		},
		{
			name:  "class << expr is lshift not heredoc",
			input: "class << self\nend",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.CLASS, "class"},
				{token.LSHIFT, "<<"},
				{token.SELF, "self"},
				{token.NEWLINE, "\n"},
				{token.END, "end"},
			},
		},
		{
			name:  "^= xor assign",
			input: "a ^= b",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.IDENT, "a"},
				{token.XORASSIGN, "^="},
				{token.IDENT, "b"},
			},
		},
		{
			name:  "match equals tilde",
			input: "x =~ /foo/",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.IDENT, "x"},
				{token.MATCH, "=~"},
				{token.REGEX_BEG, ""},
				{token.STRING_CONTENT, "foo"},
				{token.REGEX_END, ""},
			},
		},
		{
			name:  "negative match",
			input: "x !~ /foo/",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.IDENT, "x"},
				{token.NMATCH, "!~"},
				{token.REGEX_BEG, ""},
				{token.STRING_CONTENT, "foo"},
				{token.REGEX_END, ""},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := New(tt.input)
			for i, exp := range tt.expected {
				if !l.HasNext() {
					t.Fatalf("pos %d: unexpected EOF (expected %s %q)", i, exp.typ, exp.literal)
				}
				tok := l.NextToken()
				if tok.Type != exp.typ {
					t.Errorf("pos %d: expected type %s, got %s (%q)", i, exp.typ, tok.Type, tok.Literal)
				}
				if tok.Literal != exp.literal {
					t.Errorf("pos %d: expected literal %q, got %q", i, exp.literal, tok.Literal)
				}
			}
		})
	}
}
