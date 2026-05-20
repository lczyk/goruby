package lexer

import (
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
	"github.com/lczyk/goruby/token"
)

func TestLexerEscapes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []struct {
			typ     token.Type
			literal string
		}
	}{
		{
			name:  "\\u{XXXX} in string",
			input: "\"\\u{1F600}\"",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, ""},
				{token.STRING_CONTENT, "\\u{1F600}"},
				{token.STRING_END, "\""},
			},
		},
		{
			name:  "\\u{XXX YYY} multiple codepoints",
			input: "\"\\u{30e1 30bd}\"",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, ""},
				{token.STRING_CONTENT, "\\u{30e1 30bd}"},
				{token.STRING_END, "\""},
			},
		},
		{
			name:  "\\uXXXX legacy form",
			input: "\"\\u2665\"",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, ""},
				{token.STRING_CONTENT, "\\u2665"},
				{token.STRING_END, "\""},
			},
		},
		{
			name:  "\\xNN in string",
			input: "\"\\x41\"",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, ""},
				{token.STRING_CONTENT, "\\x41"},
				{token.STRING_END, "\""},
			},
		},
		{
			name:  "\\xN single hex digit",
			input: "\"\\xA\"",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, ""},
				{token.STRING_CONTENT, "\\xA"},
				{token.STRING_END, "\""},
			},
		},
		{
			name:  "\\C-x control char",
			input: "\"\\C-a\"",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, ""},
				{token.STRING_CONTENT, "\\C-a"},
				{token.STRING_END, "\""},
			},
		},
		{
			name:  "\\C-\\M-x control-meta",
			input: "\"\\C-\\M-a\"",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, ""},
				{token.STRING_CONTENT, "\\C-\\M-a"},
				{token.STRING_END, "\""},
			},
		},
		{
			name:  "\\M-x meta char",
			input: "\"\\M-a\"",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, ""},
				{token.STRING_CONTENT, "\\M-a"},
				{token.STRING_END, "\""},
			},
		},
		{
			name:  "\\M-\\C-x meta-control",
			input: "\"\\M-\\C-a\"",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, ""},
				{token.STRING_CONTENT, "\\M-\\C-a"},
				{token.STRING_END, "\""},
			},
		},
		{
			name:  "\\cx control char variant",
			input: "\"\\ca\"",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, ""},
				{token.STRING_CONTENT, "\\ca"},
				{token.STRING_END, "\""},
			},
		},
		{
			name:  "escaped # does not trigger interpolation",
			input: "\"\\#{foo}\"",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, ""},
				{token.STRING_CONTENT, "\\#{foo}"},
				{token.STRING_END, "\""},
			},
		},
		{
			name:  "\\M-\\\\ in regex (meta of escaped backslash)",
			input: "/\\M-\\\\/",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.REGEX_BEG, ""},
				{token.STRING_CONTENT, "\\M-\\\\"},
				{token.REGEX_END, ""},
			},
		},
		{
			name:  "\\M-\\n in regex (meta of newline)",
			input: "/\\M-\\n/",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.REGEX_BEG, ""},
				{token.STRING_CONTENT, "\\M-\\n"},
				{token.REGEX_END, ""},
			},
		},
		{
			name:  "\\C-\\\\ in regex (control of escaped backslash)",
			input: "/\\C-\\\\/",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.REGEX_BEG, ""},
				{token.STRING_CONTENT, "\\C-\\\\"},
				{token.REGEX_END, ""},
			},
		},
		{
			name:  "\\u{...} in %Q percent literal",
			input: "%Q{\\u{41}}",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, "Q"},
				{token.STRING_CONTENT, "\\u{41}"},
				{token.STRING_END, ""},
			},
		},
		{
			name:  "\\u{...} in %r percent regex",
			input: "%r{\\u{41}}",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.REGEX_BEG, "r"},
				{token.STRING_CONTENT, "\\u{41}"},
				{token.REGEX_END, ""},
			},
		},
		{
			name:  "\\u{...} in heredoc",
			input: "<<EOS\n\\u{41}\nEOS\n",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, "<<EOS"},
				{token.STRING_CONTENT, "\\u{41}\n"},
				{token.STRING_END, ""},
				{token.NEWLINE, "\n"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := New(tt.input)
			for i, exp := range tt.expected {
				require.That(t, l.HasNext(), "pos %d: unexpected EOF (expected %s %q)", i, exp.typ, exp.literal)
				tok := l.NextToken()
				assert.Equal(t, tok.Type, exp.typ)
				assert.Equal(t, l.Lit(tok), exp.literal)
			}
		})
	}
}

func TestLexerCharacterLiteral(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []struct {
			typ     token.Type
			literal string
		}
	}{
		{
			name:  "simple char",
			input: "?a",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING, "a"},
			},
		},
		{
			name:  "escaped newline",
			input: "?\\n",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING, "\\n"},
			},
		},
		{
			name:  "escaped tab",
			input: "?\\t",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING, "\\t"},
			},
		},
		{
			name:  "unicode escape",
			input: "?\\u{41}",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING, "\\u{41}"},
			},
		},
		{
			name:  "hex escape",
			input: "?\\x41",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING, "\\x41"},
			},
		},
		{
			name:  "control escape",
			input: "?\\C-a",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING, "\\C-a"},
			},
		},
		{
			name:  "meta escape",
			input: "?\\M-a",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING, "\\M-a"},
			},
		},
		{
			name:  "control-meta escape",
			input: "?\\C-\\M-a",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING, "\\C-\\M-a"},
			},
		},
		{
			name:  "octal in braces",
			input: "?\\o{101}",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING, "\\o{101}"},
			},
		},
		{
			name:  "plain dash (negative)",
			input: "?-",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING, "-"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := New(tt.input)
			for i, exp := range tt.expected {
				require.That(t, l.HasNext(), "pos %d: unexpected EOF (expected %s %q)", i, exp.typ, exp.literal)
				tok := l.NextToken()
				assert.Equal(t, tok.Type, exp.typ)
				assert.Equal(t, l.Lit(tok), exp.literal)
			}
		})
	}
}

func TestLexerBacktick(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []struct {
			typ     token.Type
			literal string
		}
	}{
		{
			name:  "simple command",
			input: "`ls`",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.XSTR_BEG, ""},
				{token.XSTR_CONTENT, "ls"},
				{token.XSTR_END, "`"},
			},
		},
		{
			name:  "command with interpolation",
			input: "`echo #{name}`",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.XSTR_BEG, ""},
				{token.XSTR_CONTENT, "echo "},
				{token.EMBEXPR_BEG, "#{"},
				{token.IDENT, "name"},
				{token.EMBEXPR_END, "}"},
				{token.XSTR_END, "`"},
			},
		},
		{
			name:  "command with #$var interpolation",
			input: "`echo #$foo`",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.XSTR_BEG, ""},
				{token.XSTR_CONTENT, "echo "},
				{token.GLOBAL, "$foo"},
				{token.XSTR_END, "`"},
			},
		},
		{
			name:  "command with unicode escape",
			input: "`echo \\u{41}`",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.XSTR_BEG, ""},
				{token.XSTR_CONTENT, "echo \\u{41}"},
				{token.XSTR_END, "`"},
			},
		},
		{
			name:  "empty command",
			input: "``",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.XSTR_BEG, ""},
				{token.XSTR_END, "`"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := New(tt.input)
			for i, exp := range tt.expected {
				require.That(t, l.HasNext(), "pos %d: unexpected EOF (expected %s %q)", i, exp.typ, exp.literal)
				tok := l.NextToken()
				assert.Equal(t, tok.Type, exp.typ)
				assert.Equal(t, l.Lit(tok), exp.literal)
			}
		})
	}
}

func TestLexerEscapesEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []struct {
			typ     token.Type
			literal string
		}
	}{
		{
			name:  "\\oNNN octal without braces",
			input: "\"\\o101\"",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, ""},
				{token.STRING_CONTENT, "\\o101"},
				{token.STRING_END, "\""},
			},
		},
		{
			name:  "\\u without braces in string",
			input: "\"\\u2665\"",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, ""},
				{token.STRING_CONTENT, "\\u2665"},
				{token.STRING_END, "\""},
			},
		},
		{
			name:  "escaped newline in single-quote string",
			input: "'line \\\n'",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING, "line \\\n"},
			},
		},
		{
			name:  "multiple escapes in string",
			input: "\"\\n\\t\\r\"",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, ""},
				{token.STRING_CONTENT, "\\n\\t\\r"},
				{token.STRING_END, "\""},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := New(tt.input)
			for i, exp := range tt.expected {
				require.That(t, l.HasNext(), "pos %d: unexpected EOF (expected %s %q)", i, exp.typ, exp.literal)
				tok := l.NextToken()
				assert.Equal(t, tok.Type, exp.typ)
				assert.Equal(t, l.Lit(tok), exp.literal)
			}
		})
	}
}

func TestLexerNumbers(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []struct {
			typ     token.Type
			literal string
		}
	}{
		{
			name:  "decimal integer",
			input: "42",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.INT, "42"},
			},
		},
		{
			name:  "integer with underscores",
			input: "1_000_000",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.INT, "1_000_000"},
			},
		},
		{
			name:  "hexadecimal 0xFF",
			input: "0xFF",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.INT, "0xFF"},
			},
		},
		{
			name:  "hexadecimal 0XFF",
			input: "0XFF",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.INT, "0XFF"},
			},
		},
		{
			name:  "octal 0o77",
			input: "0o77",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.INT, "0o77"},
			},
		},
		{
			name:  "octal 0O77",
			input: "0O77",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.INT, "0O77"},
			},
		},
		{
			name:  "binary 0b11",
			input: "0b1010",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.INT, "0b1010"},
			},
		},
		{
			name:  "binary 0B11",
			input: "0B1010",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.INT, "0B1010"},
			},
		},
		{
			name:  "decimal prefix 0d99",
			input: "0d99",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.INT, "0d99"},
			},
		},
		{
			name:  "float with fraction",
			input: "1.5",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.FLOAT, "1.5"},
			},
		},
		{
			name:  "leading-dot float",
			input: ".5",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.FLOAT, ".5"},
			},
		},
		{
			name:  "float with exponent",
			input: "1e10",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.FLOAT, "1e10"},
			},
		},
		{
			name:  "float with positive exponent",
			input: "1.5e+10",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.FLOAT, "1.5e+10"},
			},
		},
		{
			name:  "float with negative exponent",
			input: "1.5e-10",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.FLOAT, "1.5e-10"},
			},
		},
		{
			name:  "integer with rational suffix",
			input: "1r",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.INT, "1r"},
			},
		},
		{
			name:  "integer with complex suffix",
			input: "2i",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.INT, "2i"},
			},
		},
		{
			name:  "float with rational suffix",
			input: "1.5r",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.FLOAT, "1.5r"},
			},
		},
		{
			name:  "float with complex suffix",
			input: "1.5i",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.FLOAT, "1.5i"},
			},
		},
		{
			name:  "zero with decimal point but no fraction",
			input: "0.method",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.INT, "0"},
				{token.DOT, "."},
				{token.IDENT, "method"},
			},
		},
		{
			name:  "float with exponent and rational suffix",
			input: "1.5e10r",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.FLOAT, "1.5e10r"},
			},
		},
		{
			name:  "float with exponent and complex suffix",
			input: "1.5e10i",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.FLOAT, "1.5e10i"},
			},
		},
		{
			name:  "float with underscores",
			input: "1_234.567_890",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.FLOAT, "1_234.567_890"},
			},
		},
		{
			name:  "integer leading zero then newline",
			input: "0\nx",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.INT, "0"},
				{token.NEWLINE, "\n"},
				{token.IDENT, "x"},
			},
		},
		{
			name:  "float exponent then non-digit is ident",
			input: "1ex",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.INT, "1"},
				{token.IDENT, "ex"},
			},
		},
		{
			name:  "hex with underscores",
			input: "0xff_ff",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.INT, "0xff_ff"},
			},
		},
		{
			name:  "octal with underscores",
			input: "0o77_77",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.INT, "0o77_77"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := New(tt.input)
			for i, exp := range tt.expected {
				require.That(t, l.HasNext(), "pos %d: unexpected EOF (expected %s %q)", i, exp.typ, exp.literal)
				tok := l.NextToken()
				assert.Equal(t, tok.Type, exp.typ)
				assert.Equal(t, l.Lit(tok), exp.literal)
			}
		})
	}
}
