package lexer

import (
	"testing"

	"github.com/lczyk/goruby/token"
)

func TestLexerIdentifiers(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []struct {
			typ     token.Type
			literal string
		}
	}{
		{
			name:  "simple identifier",
			input: "foo",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.IDENT, "foo"},
			},
		},
		{
			name:  "identifier with digits",
			input: "foo123",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.IDENT, "foo123"},
			},
		},
		{
			name:  "identifier with underscores",
			input: "foo_bar",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.IDENT, "foo_bar"},
			},
		},
		{
			name:  "leading underscore",
			input: "_foo",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.IDENT, "_foo"},
			},
		},
		{
			name:  "leading underscore with digits",
			input: "_123",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.IDENT, "_123"},
			},
		},
		{
			name:  "method name with ? suffix",
			input: "nil?",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.IDENT, "nil?"},
			},
		},
		{
			name:  "method name with ! suffix",
			input: "run!",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.IDENT, "run!"},
			},
		},
		{
			name:  "method name with ? followed by space",
			input: "valid? bar",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.IDENT, "valid?"},
				{token.IDENT, "bar"},
			},
		},
		{
			name:  "method name with ! terminated by newline",
			input: "save!\n",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.IDENT, "save!"},
				{token.NEWLINE, "\n"},
			},
		},
		{
			name:  "CONST starting with uppercase",
			input: "Foo",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.CONST, "Foo"},
			},
		},
		{
			name:  "all-caps CONST",
			input: "FOO",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.CONST, "FOO"},
			},
		},
		{
			name:  "mixed-case CONST",
			input: "FooBar",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.CONST, "FooBar"},
			},
		},
		{
			name:  "identifier next to operator without space",
			input: "foo+bar",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.IDENT, "foo"},
				{token.PLUS, "+"},
				{token.IDENT, "bar"},
			},
		},
		{
			name:  "identifier before equals",
			input: "x=y",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.IDENT, "x"},
				{token.ASSIGN, "="},
				{token.IDENT, "y"},
			},
		},
		{
			name:  "identifier before dot",
			input: "obj.method",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.IDENT, "obj"},
				{token.DOT, "."},
				{token.IDENT, "method"},
			},
		},
		{
			name:  "near-keyword: ifx is an identifier",
			input: "ifx",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.IDENT, "ifx"},
			},
		},
		{
			name:  "near-keyword: defx is an identifier",
			input: "defx",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.IDENT, "defx"},
			},
		},
		{
			name:  "keyword def is a keyword not identifier",
			input: "def",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.DEF, "def"},
			},
		},
		{
			name:  "keyword if is a keyword not identifier",
			input: "if",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.IF, "if"},
			},
		},
		{
			name:  "multiple identifiers with spaces",
			input: "foo bar baz",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.IDENT, "foo"},
				{token.IDENT, "bar"},
				{token.IDENT, "baz"},
			},
		},
		{
			name:  "identifier followed by parentheses",
			input: "foo(bar)",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.IDENT, "foo"},
				{token.LPAREN, "("},
				{token.IDENT, "bar"},
				{token.RPAREN, ")"},
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

func TestLexerUnicodeIdentifiers(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []struct {
			typ     token.Type
			literal string
		}
	}{
		{
			name:  "latin-1 accented letter (de\\u0301f)",
			input: "d\u00E9f",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.IDENT, "d\u00E9f"},
			},
		},
		{
			name:  "cyrillic letters (\\u043C\\u0435\\u0442\\u043E\\u0434)",
			input: "\u043C\u0435\u0442\u043E\u0434",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.IDENT, "\u043C\u0435\u0442\u043E\u0434"},
			},
		},
		{
			name:  "CJK ideographs (\\u5909\\u6570)",
			input: "\u5909\u6570",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.IDENT, "\u5909\u6570"},
			},
		},
		{
			name:  "mixed ASCII and Unicode",
			input: "hello_\u043C\u0438\u0440",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.IDENT, "hello_\u043C\u0438\u0440"},
			},
		},
		{
			name:  "Unicode letter starting with underscore",
			input: "_\u043C\u0435\u0442\u043E\u0434",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.IDENT, "_\u043C\u0435\u0442\u043E\u0434"},
			},
		},
		{
			// NOTE: non-ASCII uppercase letters are NOT detected as CONST.
			// LookupIdent uses ASCII-only byte comparison (A-Z) for speed.
			name:  "Greek capital letter is IDENT not CONST (ASCII-only check)",
			input: "\u0394elta",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.IDENT, "\u0394elta"},
			},
		},
		{
			name:  "Unicode identifier next to operator",
			input: "\u043C\u0435\u0442\u043E\u0434+\u5909\u6570",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.IDENT, "\u043C\u0435\u0442\u043E\u0434"},
				{token.PLUS, "+"},
				{token.IDENT, "\u5909\u6570"},
			},
		},
		{
			name:  "Unicode method with ? suffix",
			input: "\u0432\u0430\u043B\u0438\u0434?",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.IDENT, "\u0432\u0430\u043B\u0438\u0434?"},
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

func TestLexerIdentifierInContexts(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []struct {
			typ     token.Type
			literal string
		}
	}{
		{
			name:  "identifier inside #{} interpolation",
			input: "\"hello #{name} world\"",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, ""},
				{token.STRING_CONTENT, "hello "},
				{token.EMBEXPR_BEG, "#{"},
				{token.IDENT, "name"},
				{token.EMBEXPR_END, "}"},
				{token.STRING_CONTENT, " world"},
				{token.STRING_END, "\""},
			},
		},
		{
			name:  "CONST inside #{} interpolation",
			input: "\"#{Foo}\"",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, ""},
				{token.EMBEXPR_BEG, "#{"},
				{token.CONST, "Foo"},
				{token.EMBEXPR_END, "}"},
				{token.STRING_END, "\""},
			},
		},
		{
			name:  "identifier after $ global prefix",
			input: "$foo",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.GLOBAL, "$foo"},
			},
		},
		{
			name:  "identifier after @ ivar prefix",
			input: "@foo",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.AT, "@"},
				{token.IDENT, "foo"},
			},
		},
		{
			name:  "identifier after @@ cvar prefix",
			input: "@@foo",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.CLASS_VAR, "@@"},
				{token.IDENT, "foo"},
			},
		},
		{
			name:  "identifier inside heredoc",
			input: "<<EOS\nhello #{name}\nEOS\n",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, ""},
				{token.STRING_CONTENT, "hello "},
				{token.EMBEXPR_BEG, "#{"},
				{token.IDENT, "name"},
				{token.EMBEXPR_END, "}"},
				{token.STRING_CONTENT, "\n"},
				{token.STRING_END, ""},
				{token.NEWLINE, "\n"},
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
