package lexer

import (
	"testing"

	"github.com/lczyk/assert"
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
				{token.STRING_BEG, "<<'EOS'"},
				{token.STRING_CONTENT, "hello\nworld\n"},
				{token.STRING_END, ""},
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
				{token.STRING_BEG, "<<~'EOS'"},
				{token.STRING_CONTENT, "hello\nworld\n"},
				{token.STRING_END, ""},
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
				{token.STRING_BEG, "<<-'EOS'"},
				{token.STRING_CONTENT, "\tcontent\n"},
				{token.STRING_END, ""},
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
				{token.STRING_BEG, "<<'EOS'"},
				{token.STRING_CONTENT, ""},
				{token.STRING_END, ""},
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
				{token.STRING_BEG, "<<~'EOS'"},
				{token.STRING_CONTENT, "\ncontent\n\n"},
				{token.STRING_END, ""},
				{token.NEWLINE, "\n"},
				{token.EOF, ""},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := New(tt.input)
			for i, exp := range tt.expected {
				assert.That(t, l.HasNext(), "pos %d: unexpected EOF (expected %s %q)", i, exp.typ, exp.literal)
				tok := l.NextToken()
				assert.Equal(t, tok.Type, exp.typ)
				assert.Equal(t, l.Lit(tok), exp.literal)
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
				assert.That(t, l.HasNext(), "pos %d: unexpected EOF (expected %s %q)", i, exp.typ, exp.literal)
				tok := l.NextToken()
				assert.Equal(t, tok.Type, exp.typ)
				assert.Equal(t, l.Lit(tok), exp.literal)
			}
		})
	}
}

func TestHelperIsExpressionEnd(t *testing.T) {
	for _, tt := range []struct {
		tok  token.Type
		want bool
	}{
		{token.IDENT, true},
		{token.CONST, true},
		{token.GLOBAL, true},
		{token.CLASS_VAR, true},
		{token.INT, true},
		{token.STRING, true},
		{token.REGEX, true},
		{token.XSTR, true},
		{token.RPAREN, true},
		{token.RBRACKET, true},
		{token.RBRACE, true},
		{token.TRUE, true},
		{token.FALSE, true},
		{token.NIL, true},
		{token.SELF, true},
		{token.END, true},
		{token.LPAREN, false},
		{token.EOF, false},
		{token.ASSIGN, false},
	} {
		assert.Equal(t, isExpressionEnd(tt.tok), tt.want)
	}
}

func TestHelperIsMethodCallTarget(t *testing.T) {
	for _, tt := range []struct {
		tok  token.Type
		want bool
	}{
		{token.IDENT, true},
		{token.CONST, true},
		{token.GLOBAL, true},
		{token.RPAREN, true},
		{token.RBRACKET, true},
		{token.RBRACE, true},
		{token.END, true},
		{token.LPAREN, false},
		{token.EOF, false},
		{token.ASSIGN, false},
		{token.INT, false},
	} {
		assert.Equal(t, isMethodCallTarget(tt.tok), tt.want)
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
				assert.That(t, l.HasNext(), "pos %d: unexpected EOF (expected %s %q)", i, exp.typ, exp.literal)
				tok := l.NextToken()
				assert.Equal(t, tok.Type, exp.typ)
				assert.Equal(t, l.Lit(tok), exp.literal)
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
				assert.That(t, l.HasNext(), "pos %d: unexpected EOF (expected %s %q)", i, exp.typ, exp.literal)
				tok := l.NextToken()
				assert.Equal(t, tok.Type, exp.typ)
				assert.Equal(t, l.Lit(tok), exp.literal)
			}
		})
	}
}
