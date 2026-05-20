package lexer

import (
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
	"github.com/lczyk/goruby/token"
)

// --- escape edge case: \o{...} in string (consumeEscape braces-octal branch) ---

func TestLexerBraceOctalInString(t *testing.T) {
	input := "\"\\o{101}\""
	l := New(input)
	tok := l.NextToken()
	assert.Equal(t, tok.Type, token.STRING_BEG)
	tok = l.NextToken()
	assert.Equal(t, tok.Type, token.STRING_CONTENT)
	assert.Equal(t, l.Lit(tok), "\\o{101}")
}

// --- character literal remaining escape branches ---

func TestLexerCharLiteralRemainingEscapes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"meta-control backslash", "?\\M-\\C-a", "\\M-\\C-a"},
		{"octal without braces", "?\\o101", "\\o101"},
		{"control lowercase c", "?\\ca", "\\ca"},
		{"unicode without braces", "?\\u0041", "\\u0041"},
		{"meta of backslashed newline", "?\\M-\\n", "\\M-\\n"},
		{"control of backslashed tab", "?\\C-\\t", "\\C-\\t"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := New(tt.input)
			tok := l.NextToken()
			assert.Equal(t, tok.Type, token.STRING)
			assert.Equal(t, l.Lit(tok), tt.expected)
		})
	}
}

// --- percent content with \o{...} in various literal types ---

func TestLexerPercentContentOctBrace(t *testing.T) {
	type expTok struct {
		typ     token.Type
		literal string
	}
	tests := []struct {
		name     string
		input    string
		expected []expTok
	}{
		{
			name:  "\\o{...} in %Q",
			input: "%Q{\\o{101}}",
			expected: []expTok{
				{token.STRING_BEG, "Q"},
				{token.STRING_CONTENT, "\\o{101}"},
				{token.STRING_END, ""},
			},
		},
		{
			name:  "\\o{...} in %r regex",
			input: "%r{\\o{77}}",
			expected: []expTok{
				{token.REGEX_BEG, "r"},
				{token.STRING_CONTENT, "\\o{77}"},
				{token.REGEX_END, ""},
			},
		},
		{
			name:  "\\o{...} in %x",
			input: "%x{\\o{101}}",
			expected: []expTok{
				{token.XSTR_BEG, "x"},
				{token.XSTR_CONTENT, "\\o{101}"},
				{token.XSTR_END, ""},
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

// --- backtick content with \o{...} ---

func TestLexerBacktickContentOctBrace(t *testing.T) {
	input := "`\\o{101}`"
	l := New(input)
	type expTok struct {
		typ     token.Type
		literal string
	}
	expected := []expTok{
		{token.XSTR_BEG, ""},
		{token.XSTR_CONTENT, "\\o{101}"},
		{token.XSTR_END, "`"},
	}
	for i, exp := range expected {
		require.That(t, l.HasNext(), "pos %d: unexpected EOF (expected %s %q)", i, exp.typ, exp.literal)
		tok := l.NextToken()
		assert.Equal(t, tok.Type, exp.typ)
		assert.Equal(t, l.Lit(tok), exp.literal)
	}
}

// --- regex content with \o{...}, options ---

func TestLexerRegexOctBraceAndOptions(t *testing.T) {
	type expTok struct {
		typ     token.Type
		literal string
	}
	tests := []struct {
		name     string
		input    string
		expected []expTok
	}{
		{
			name:  "\\o{...} in regex",
			input: "/\\o{77}/",
			expected: []expTok{
				{token.REGEX_BEG, ""},
				{token.STRING_CONTENT, "\\o{77}"},
				{token.REGEX_END, ""},
			},
		},
		{
			name:  "regex with i option",
			input: "/foo/i",
			expected: []expTok{
				{token.REGEX_BEG, ""},
				{token.STRING_CONTENT, "foo"},
				{token.REGEX_END, "i"},
			},
		},
		{
			name:  "regex with multiple options",
			input: "/foo/imx",
			expected: []expTok{
				{token.REGEX_BEG, ""},
				{token.STRING_CONTENT, "foo"},
				{token.REGEX_END, "imx"},
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

// --- heredoc edge cases ---

func TestLexerHeredocEdgeCases(t *testing.T) {
	type expTok struct {
		typ     token.Type
		literal string
	}
	tests := []struct {
		name     string
		input    string
		expected []expTok
	}{
		{
			name:  "heredoc with trailer",
			input: "<<EOS.chop\nbody\nEOS\n",
			expected: []expTok{
				{token.STRING_BEG, "<<EOS"},
				{token.STRING_CONTENT, "body\n"},
				{token.STRING_END, ""},
				{token.DOT, "."},
				{token.IDENT, "chop"},
				{token.NEWLINE, "\n"},
			},
		},
		{
			name:  "interpolating heredoc with #$var",
			input: "<<EOS\nhello #$name\nEOS\n",
			expected: []expTok{
				{token.STRING_BEG, "<<EOS"},
				{token.STRING_CONTENT, "hello "},
				{token.GLOBAL, "$name"},
				{token.STRING_CONTENT, "\n"},
				{token.STRING_END, ""},
				{token.NEWLINE, "\n"},
			},
		},
		{
			name:  "interpolating heredoc with #@var",
			input: "<<EOS\nhello #@name\nEOS\n",
			expected: []expTok{
				{token.STRING_BEG, "<<EOS"},
				{token.STRING_CONTENT, "hello "},
				{token.AT, "@"},
				{token.IDENT, "name"},
				{token.STRING_CONTENT, "\n"},
				{token.STRING_END, ""},
				{token.NEWLINE, "\n"},
			},
		},
		{
			name:  "backtick heredoc",
			input: "<<`EOS`\ncmd\nEOS\n",
			expected: []expTok{
				{token.XSTR_BEG, "<<`EOS`"},
				{token.XSTR_CONTENT, "cmd\n"},
				{token.XSTR_END, ""},
				{token.NEWLINE, "\n"},
			},
		},
		{
			name:  "squiggy interpolating heredoc",
			input: "<<~EOS\n  body\n  EOS\n",
			expected: []expTok{
				{token.STRING_BEG, "<<~EOS"},
				{token.STRING_CONTENT, "body\n"},
				{token.STRING_END, ""},
				{token.NEWLINE, "\n"},
			},
		},
		{
			name:  "squiggy heredoc with blank lines",
			input: "<<~EOS\n  line1\n\n  line2\n  EOS\n",
			expected: []expTok{
				{token.STRING_BEG, "<<~EOS"},
				{token.STRING_CONTENT, "line1\n\nline2\n"},
				{token.STRING_END, ""},
				{token.NEWLINE, "\n"},
			},
		},
		{
			name:  "heredoc with #{expr} in body",
			input: "<<EOS\nhello #{name}\nEOS\n",
			expected: []expTok{
				{token.STRING_BEG, "<<EOS"},
				{token.STRING_CONTENT, "hello "},
				{token.EMBEXPR_BEG, "#{"},
				{token.IDENT, "name"},
				{token.EMBEXPR_END, "}"},
				{token.STRING_CONTENT, "\n"},
				{token.STRING_END, ""},
				{token.NEWLINE, "\n"},
			},
		},
		{
			name:  "heredoc empty body with trailer",
			input: "<<EOS.chop\nEOS\n",
			expected: []expTok{
				{token.STRING_BEG, "<<EOS"},
				{token.STRING_CONTENT, ""},
				{token.STRING_END, ""},
				{token.DOT, "."},
				{token.IDENT, "chop"},
				{token.NEWLINE, "\n"},
			},
		},
		{
			name:  "quoted heredoc double-quote",
			input: "<<\"EOS\"\nhello\nEOS\n",
			expected: []expTok{
				{token.STRING_BEG, "<<\"EOS\""},
				{token.STRING_CONTENT, "hello\n"},
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

// --- global variable edge cases (punct and digit globals) ---

func TestLexerGlobalEdgeCases(t *testing.T) {
	type expTok struct {
		typ     token.Type
		literal string
	}
	tests := []struct {
		name     string
		input    string
		expected []expTok
	}{
		{"global punct dot", "$.", []expTok{{token.GLOBAL, "$."}}},
		{"global punct bang", "$!", []expTok{{token.GLOBAL, "$!"}}},
		{"global punct tilde", "$~", []expTok{{token.GLOBAL, "$~"}}},
		{"global semicolon", "$;", []expTok{{token.GLOBAL, "$;"}}},
		{"global digit zero", "$0", []expTok{{token.GLOBAL, "$0"}}},
		{"global digit", "$1", []expTok{{token.GLOBAL, "$1"}}},
		{"global colon", "$:", []expTok{{token.GLOBAL, "$:"}}},
		{"global double quote", "$\"", []expTok{{token.GLOBAL, "$\""}}},
		{"global less than", "$<", []expTok{{token.GLOBAL, "$<"}}},
		{"global greater than", "$>", []expTok{{token.GLOBAL, "$>"}}},
		{"global backslash", "$\\", []expTok{{token.GLOBAL, "$\\"}}},
		{"global forward slash", "$/", []expTok{{token.GLOBAL, "$/"}}},
		{"global ampersand", "$&", []expTok{{token.GLOBAL, "$&"}}},
		{"global asterisk", "$*", []expTok{{token.GLOBAL, "$*"}}},
		{"global single quote", "$'", []expTok{{token.GLOBAL, "$'"}}},
		{"global plus", "$+", []expTok{{token.GLOBAL, "$+"}}},
		{"global minus", "$-", []expTok{{token.GLOBAL, "$-"}}},
		{"global equals", "$=", []expTok{{token.GLOBAL, "$="}}},
		{"global dollar", "$$", []expTok{{token.GLOBAL, "$$"}}},
		{"global backtick", "$`", []expTok{{token.GLOBAL, "$`"}}},
		{"global comma", "$,", []expTok{{token.GLOBAL, "$,"}}},
		{"global alpha", "$foo", []expTok{{token.GLOBAL, "$foo"}}},
		{"global alpha underscore", "$foo_bar", []expTok{{token.GLOBAL, "$foo_bar"}}},
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

// --- digit edge cases ---

func TestLexerDigitEdgeCases(t *testing.T) {
	type expTok struct {
		typ     token.Type
		literal string
	}
	tests := []struct {
		name     string
		input    string
		expected []expTok
	}{
		{"0D uppercase decimal prefix", "0D99", []expTok{{token.INT, "0D99"}}},
		{"hex with underscore", "0xAB_CD", []expTok{{token.INT, "0xAB_CD"}}},
		{"oct with underscores", "0o77_11", []expTok{{token.INT, "0o77_11"}}},
		{"bin with underscore", "0b10_10", []expTok{{token.INT, "0b10_10"}}},
		{"decimal prefix with underscore", "0d9_9", []expTok{{token.INT, "0d9_9"}}},
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

// --- string content edge cases ---

func TestLexerStringContentEdgeCases(t *testing.T) {
	type expTok struct {
		typ     token.Type
		literal string
	}
	tests := []struct {
		name     string
		input    string
		expected []expTok
	}{
		{
			name:  "#$ followed by digit",
			input: "\"#$1\"",
			expected: []expTok{
				{token.STRING_BEG, ""},
				{token.GLOBAL, "$1"},
				{token.STRING_END, "\""},
			},
		},
		{
			name:  "plain # not interpolation",
			input: "\"foo#bar\"",
			expected: []expTok{
				{token.STRING_BEG, ""},
				{token.STRING_CONTENT, "foo#bar"},
				{token.STRING_END, "\""},
			},
		},
		{
			name:  "interpolation at start of string",
			input: "\"#{x}\"",
			expected: []expTok{
				{token.STRING_BEG, ""},
				{token.EMBEXPR_BEG, "#{"},
				{token.IDENT, "x"},
				{token.EMBEXPR_END, "}"},
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

// --- percent literal more edge cases ---

func TestLexerPercentLiteralMoreEdgeCases(t *testing.T) {
	type expTok struct {
		typ     token.Type
		literal string
	}
	tests := []struct {
		name     string
		input    string
		expected []expTok
	}{
		{
			name:  "%i with asterisk delim",
			input: "%i*foo*",
			expected: []expTok{
				{token.STRING_BEG, "i"},
				{token.STRING_CONTENT, "foo"},
				{token.STRING_END, ""},
			},
		},
		{
			name:  "bare % after ident via method call context",
			input: "foo %(bar)",
			expected: []expTok{
				{token.IDENT, "foo"},
				{token.STRING_BEG, "Q"},
				{token.STRING_CONTENT, "bar"},
				{token.STRING_END, ""},
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

// --- NextToken after exhaust ---

func TestLexerNextTokenAfterExhaust(t *testing.T) {
	input := "x"
	l := New(input)
	tok := l.NextToken()
	assert.Equal(t, tok.Type, token.IDENT)
	for i := 0; i < 3; i++ {
		tok := l.NextToken()
		assert.Equal(t, tok.Type, token.EOF)
	}
	// HasNext stays true: startLexer loops on EOF, caller stops on EOF token.
	if !l.HasNext() {
		t.Error("HasNext should return true (lexer never self-terminates)")
	}
}

// --- startLexer edge cases ---

func TestLexerStartLexerEdgeCases(t *testing.T) {
	type expTok struct {
		typ     token.Type
		literal string
	}
	tests := []struct {
		name     string
		input    string
		expected []expTok
	}{
		{
			name:  "line continuation backslash newline",
			input: "foo \\\nbar",
			expected: []expTok{
				{token.IDENT, "foo"},
				{token.IDENT, "bar"},
			},
		},
		{
			name:  "leading dot method after newline",
			input: "x\n  .foo",
			expected: []expTok{
				{token.IDENT, "x"},
				{token.DOT, "."},
				{token.IDENT, "foo"},
			},
		},
		{
			name:  "leading &. after newline",
			input: "x\n  &.foo",
			expected: []expTok{
				{token.IDENT, "x"},
				{token.LONELY, "&."},
				{token.IDENT, "foo"},
			},
		},
		{
			name:  "range .. after newline not suppressed",
			input: "x\n..10",
			expected: []expTok{
				{token.IDENT, "x"},
				{token.NEWLINE, "\n"},
				{token.RANGE, ".."},
				{token.INT, "10"},
			},
		},
		{
			name:  "range ... after newline not suppressed",
			input: "x\n...10",
			expected: []expTok{
				{token.IDENT, "x"},
				{token.NEWLINE, "\n"},
				{token.RANGEEX, "..."},
				{token.INT, "10"},
			},
		},
		{
			name:  "case equality ===",
			input: "===",
			expected: []expTok{
				{token.CASEEQ, "==="},
			},
		},
		{
			name:  "hashrocket =>",
			input: "=>",
			expected: []expTok{
				{token.HASHROCKET, "=>"},
			},
		},
		{
			name:  "lambda literal ->",
			input: "->",
			expected: []expTok{
				{token.LAMBDA, "->"},
			},
		},
		{
			name:  "power assign **=",
			input: "**=",
			expected: []expTok{
				{token.POWERASSIGN, "**="},
			},
		},
		{
			name:  "spaceship <=>",
			input: "<=>",
			expected: []expTok{
				{token.SPACESHIP, "<=>"},
			},
		},
		{
			name:  "scope resolution ::",
			input: "::",
			expected: []expTok{
				{token.SCOPE, "::"},
			},
		},
		{
			name:  "ident with question mark",
			input: "nil?",
			expected: []expTok{
				{token.IDENT, "nil?"},
			},
		},
		{
			name:  "ident with bang",
			input: "run!",
			expected: []expTok{
				{token.IDENT, "run!"},
			},
		},
		{
			name:  "label key",
			input: "foo:",
			expected: []expTok{
				{token.LABEL, "foo:"},
			},
		},
		{
			name:  "symbol colon then whitespace",
			input: ": sym",
			expected: []expTok{
				{token.COLON, ":"},
				{token.IDENT, "sym"},
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

// --- single-quote string backslash edge case ---

func TestLexerSingleQuoteBackslashEdge(t *testing.T) {
	input := "'\\\\'"
	l := New(input)
	tok := l.NextToken()
	assert.Equal(t, tok.Type, token.STRING)
	assert.Equal(t, l.Lit(tok), "\\\\")
}

// --- NextToken: cover channel-closed path (error then drain) ---

func TestLexerNextTokenAfterError(t *testing.T) {
	// Single backslash triggers errorf which sets state=nil, then channel closes after ILLEGAL drained.
	l := New("\\")
	tok := l.NextToken()
	assert.Equal(t, tok.Type, token.ILLEGAL)
	// Channel should now be closed; next call reads from closed channel (ok=false path).
	tok = l.NextToken()
	assert.Equal(t, tok.Type, token.EOF)
}

// --- lexDigit: exponent edge cases ---

func TestLexerDigitExponentEdgeCases(t *testing.T) {
	type expTok struct {
		typ     token.Type
		literal string
	}
	tests := []struct {
		name     string
		input    string
		expected []expTok
	}{
		{
			name:  "e without digit rejected",
			input: "1e",
			expected: []expTok{
				{token.ILLEGAL, "trailing 'e' in number"},
			},
		},
		{
			name:  "E+ without digit rejected",
			input: "1E+",
			expected: []expTok{
				{token.ILLEGAL, "trailing 'E' in number"},
			},
		},
		{
			name:  "e- without digit rejected",
			input: "1e-",
			expected: []expTok{
				{token.ILLEGAL, "trailing 'e' in number"},
			},
		},
		{
			name:  "0e+x rejected",
			input: "0e+x",
			expected: []expTok{
				{token.ILLEGAL, "trailing 'e' in number"},
			},
		},
		{
			name:  "leading zero then letter not hex prefix",
			input: "0g",
			expected: []expTok{
				{token.INT, "0"},
				{token.IDENT, "g"},
			},
		},
		{
			name:  "float exponent with sign no digit rejected",
			input: "1.5e+",
			expected: []expTok{
				{token.ILLEGAL, "trailing 'e' in number"},
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

// --- lexGlobal: edge cases for error paths ---

func TestLexerGlobalIllegalChar(t *testing.T) {
	// $ followed by whitespace should be an error
	l := New("$ ")
	tok := l.NextToken()
	assert.Equal(t, tok.Type, token.ILLEGAL)
}

func TestLexerGlobalExprDelim(t *testing.T) {
	// $ followed by expression delimiter like newline
	l := New("$\n")
	tok := l.NextToken()
	assert.Equal(t, tok.Type, token.ILLEGAL)
}

// --- lexSingleQuoteString: unterminated ---

func TestLexerUnterminatedSingleQuote(t *testing.T) {
	l := New("'unterminated")
	tok := l.NextToken()
	assert.Equal(t, tok.Type, token.ILLEGAL)
}

// --- heredoc: indented delimiter matching edge cases ---

func TestLexerHeredocIndentedMatch(t *testing.T) {
	type expTok struct {
		typ     token.Type
		literal string
	}
	tests := []struct {
		name     string
		input    string
		expected []expTok
	}{
		{
			name:  "<<- with mixed whitespace indent before delim",
			input: "<<-EOS\n  body\n  \tEOS\n",
			expected: []expTok{
				{token.STRING_BEG, "<<-EOS"},
				{token.STRING_CONTENT, "  body\n"},
				{token.STRING_END, ""},
				{token.NEWLINE, "\n"},
			},
		},
		{
			name:  "<<- with no indent on delim line",
			input: "<<-EOS\nbody\nEOS\n",
			expected: []expTok{
				{token.STRING_BEG, "<<-EOS"},
				{token.STRING_CONTENT, "body\n"},
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

// --- heredoc: squiggy indent stripping ---

func TestLexerStripHeredocIndent(t *testing.T) {
	// Test stripHeredocIndent with varying whitespace
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"min indent 1 strips both", "  a\n b", " a\nb"},
		{"common 2-space indent", "  a\n  b", "a\nb"},
		{"empty lines ignored", "  a\n\n  b", "a\n\nb"},
		{"tab indent", "\ta\n\tb", "a\nb"},
		{"mixed indent takes min", "  a\n\tb", " a\nb"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripHeredocIndent(tt.input)
			assert.Equal(t, got, tt.want)
		})
	}
}

// --- heredoc: <<~ with backtick quote ---

func TestLexerSquiggyBacktickHeredoc(t *testing.T) {
	l := New("<<~`EOS`\n  cmd\n  EOS\n")
	tok := l.NextToken()
	assert.Equal(t, tok.Type, token.XSTR_BEG)
	tok = l.NextToken()
	assert.Equal(t, tok.Type, token.XSTR_CONTENT)
	assert.Equal(t, l.Lit(tok), "cmd\n")
	tok = l.NextToken()
	assert.Equal(t, tok.Type, token.XSTR_END)
}

// --- startLexer: __END__ marker ---

func TestLexerEndMarkerAfterNewline(t *testing.T) {
	// __END__ at line start consumes rest of input
	l := New("x\n__END__\nrest of file")
	// First: IDENT "x"
	tok := l.NextToken()
	if tok.Type != token.IDENT || l.Lit(tok) != "x" {
		t.Fatalf("expected IDENT x, got %s %q", tok.Type, l.Lit(tok))
	}
	// Then NEWLINE
	tok = l.NextToken()
	assert.Equal(t, tok.Type, token.NEWLINE)
	// Then EOF (__END__ consumed rest)
	tok = l.NextToken()
	assert.Equal(t, tok.Type, token.EOF)
}

// --- startLexer: ? with whitespace emits QMARK ---

func TestLexerQMarkWhitespace(t *testing.T) {
	l := New("? foo")
	tok := l.NextToken()
	assert.Equal(t, tok.Type, token.QMARK)
}

// --- startLexer: ? with expression delimiter produces error ---

func TestLexerQMarkExprDelim(t *testing.T) {
	l := New("?\n")
	tok := l.NextToken()
	assert.Equal(t, tok.Type, token.ILLEGAL)
}

// --- startLexer: < at start ---

func TestLexerLTAngle(t *testing.T) {
	l := New("a < b")
	// IDENT "a"
	l.NextToken()
	// LT "<"
	tok := l.NextToken()
	assert.Equal(t, tok.Type, token.LT)
	assert.Equal(t, l.Lit(tok), "<")
}

// --- startLexer: <<= (LShift assign) ---

func TestLexerLShiftAssign(t *testing.T) {
	l := New("x <<= 2")
	l.NextToken() // IDENT
	tok := l.NextToken()
	assert.Equal(t, tok.Type, token.LSHIFTASSIGN)
	assert.Equal(t, l.Lit(tok), "<<=")
}

// --- startLexer: >> and >>= ---

func TestLexerRShiftOps(t *testing.T) {
	l := New(">>")
	tok := l.NextToken()
	assert.Equal(t, tok.Type, token.RSHIFT)
	l2 := New(">>= 2")
	tok2 := l2.NextToken()
	assert.That(t, !(tok2.Type != token.RSHIFTASSIGN), "expected RSHIFTASSIGN, got %s (%q)", tok2.Type, l2.Lit(tok2))
}

// --- startLexer: >= ---

func TestLexerGTE(t *testing.T) {
	l := New(">=")
	tok := l.NextToken()
	assert.Equal(t, tok.Type, token.GTE)
}

// --- startLexer: > ---

func TestLexerGT(t *testing.T) {
	l := New("a > b")
	l.NextToken()
	tok := l.NextToken()
	assert.Equal(t, tok.Type, token.GT)
}

// --- startLexer: lshift/<< non-heredoc with non-alpha after ---

func TestLexerLShiftNonHeredoc(t *testing.T) {
	// << followed by non-letter, non-underscore (not a heredoc)
	l := New("<<2")
	tok := l.NextToken()
	assert.Equal(t, tok.Type, token.LSHIFT)
	// Then INT 2
	tok = l.NextToken()
	assert.Equal(t, tok.Type, token.INT)
}

// --- startLexer: class << expr (singleton class, not heredoc) ---

func TestLexerClassLShift(t *testing.T) {
	l := New("class << self\nend")
	tok := l.NextToken()
	assert.Equal(t, tok.Type, token.CLASS)
	tok = l.NextToken()
	assert.Equal(t, tok.Type, token.LSHIFT)
}

// --- startLexer: <<~ followed by non-alpha (should be lshift, not heredoc) ---

func TestLexerSquigNotHeredoc(t *testing.T) {
	// <<~ followed by a digit: not a heredoc, just << and ~ and digit
	l := New("<<~2")
	tok := l.NextToken()
	assert.Equal(t, tok.Type, token.LSHIFT)
	tok = l.NextToken()
	assert.Equal(t, tok.Type, token.TILDE)
}

// --- startLexer: <<- followed by non-alpha (not heredoc) ---

func TestLexerIndentNotHeredoc(t *testing.T) {
	l := New("<<-2")
	tok := l.NextToken()
	assert.Equal(t, tok.Type, token.LSHIFT)
	tok = l.NextToken()
	assert.Equal(t, tok.Type, token.MINUS)
}

// --- startLexer: illegal char ---

func TestLexerIllegalChar(t *testing.T) {
	// \ without newline should error
	l := New("\\x")
	tok := l.NextToken()
	assert.Equal(t, tok.Type, token.ILLEGAL)
}

// --- startLexer: bare % as modulo ---

func TestLexerModulo(t *testing.T) {
	l := New("a%2")
	l.NextToken() // IDENT
	tok := l.NextToken()
	assert.Equal(t, tok.Type, token.MODULO)
}

// --- lexStringContent: #$ followed by @ in string ---

func TestLexerStringHashAt(t *testing.T) {
	l := New("\"#@foo\"")
	tok := l.NextToken() // STRING_BEG
	tok = l.NextToken()  // AT
	assert.Equal(t, tok.Type, token.AT)
	tok = l.NextToken() // IDENT "foo"
	if tok.Type != token.IDENT || l.Lit(tok) != "foo" {
		t.Fatalf("expected IDENT foo, got %s %q", tok.Type, l.Lit(tok))
	}
}

// --- lexStringContent: #$var in string (already tested, test #@@var) ---

func TestLexerStringHashClassVar(t *testing.T) {
	l := New("\"#@@cvar\"")
	tok := l.NextToken() // STRING_BEG
	tok = l.NextToken()  // CLASS_VAR
	assert.Equal(t, tok.Type, token.CLASS_VAR)
	tok = l.NextToken() // IDENT
	if tok.Type != token.IDENT || l.Lit(tok) != "cvar" {
		t.Fatalf("expected IDENT cvar, got %s %q", tok.Type, l.Lit(tok))
	}
}

// --- lexPercentContent: #@@var interpolation ---

func TestLexerPercentContentHashClassVar(t *testing.T) {
	l := New("%Q{hello #@@cvar}")
	tok := l.NextToken() // STRING_BEG "Q"
	tok = l.NextToken()  // STRING_CONTENT "hello "
	tok = l.NextToken()  // CLASS_VAR
	assert.Equal(t, tok.Type, token.CLASS_VAR)
}

// --- lexPercentContent: #@var interpolation in %r regex ---

func TestLexerPercentRegexHashAt(t *testing.T) {
	l := New("%r{foo #@bar}")
	tok := l.NextToken() // REGEX_BEG
	tok = l.NextToken()  // STRING_CONTENT "foo "
	tok = l.NextToken()  // AT
	assert.Equal(t, tok.Type, token.AT)
	tok = l.NextToken() // IDENT
	if tok.Type != token.IDENT || l.Lit(tok) != "bar" {
		t.Fatalf("expected IDENT bar, got %s %q", tok.Type, l.Lit(tok))
	}
}

// --- lexPercentLiteral: %r with #@@var interpolation ---

func TestLexerPercentRegexHashClassVar(t *testing.T) {
	l := New("%r{foo #@@bar}")
	tok := l.NextToken() // REGEX_BEG
	tok = l.NextToken()  // STRING_CONTENT
	tok = l.NextToken()  // CLASS_VAR
	assert.Equal(t, tok.Type, token.CLASS_VAR)
	tok = l.NextToken() // IDENT
	if tok.Type != token.IDENT || l.Lit(tok) != "bar" {
		t.Fatalf("expected IDENT bar, got %s %q", tok.Type, l.Lit(tok))
	}
}

// --- lexRegexContent: #$var interpolation ---

func TestLexerRegexHashDollar(t *testing.T) {
	l := New("/foo #$bar/")
	tok := l.NextToken() // REGEX_BEG
	tok = l.NextToken()  // STRING_CONTENT "foo "
	tok = l.NextToken()  // GLOBAL
	assert.Equal(t, tok.Type, token.GLOBAL)
	assert.Equal(t, l.Lit(tok), "$bar")
}

// --- lexRegexContent: #@var and #@@var ---

func TestLexerRegexHashAt(t *testing.T) {
	l := New("/foo #@bar/")
	tok := l.NextToken() // REGEX_BEG
	tok = l.NextToken()  // STRING_CONTENT "foo "
	tok = l.NextToken()  // AT
	assert.Equal(t, tok.Type, token.AT)
	tok = l.NextToken() // IDENT
	if tok.Type != token.IDENT || l.Lit(tok) != "bar" {
		t.Fatalf("expected IDENT bar, got %s %q", tok.Type, l.Lit(tok))
	}
}

func TestLexerRegexHashClassVar(t *testing.T) {
	l := New("/foo #@@bar/")
	tok := l.NextToken() // REGEX_BEG
	tok = l.NextToken()  // STRING_CONTENT "foo "
	tok = l.NextToken()  // CLASS_VAR
	assert.Equal(t, tok.Type, token.CLASS_VAR)
}

// --- lexRegexContent: escaped char in regex ---

func TestLexerRegexEscaped(t *testing.T) {
	l := New("/\\//")
	tok := l.NextToken() // REGEX_BEG
	tok = l.NextToken()  // STRING_CONTENT "\\/"
	assert.Equal(t, tok.Type, token.STRING_CONTENT)
	tok = l.NextToken() // REGEX_END
	assert.Equal(t, tok.Type, token.REGEX_END)
}

// --- lexPercentLiteral: bare % after newline (regex context) ---

func TestLexerPercentAfterNewline(t *testing.T) {
	// After NEWLINE, isRegexBeginContext returns true, so % starts a percent literal
	l := New("x\n%(bar)")
	l.NextToken()        // IDENT x
	l.NextToken()        // NEWLINE
	tok := l.NextToken() // STRING_BEG "Q"
	assert.Equal(t, tok.Type, token.STRING_BEG)
}

// --- lexHeredocBody: squiggy literal heredoc ---

func TestLexerHeredocBodySquigLiteral(t *testing.T) {
	// <<~'EOS' is a literal squiggy heredoc - STRING_BEG + STRING_CONTENT + STRING_END
	l := New("<<~'EOS'\n  hello\n  EOS\n")
	tok := l.NextToken()
	assert.Equal(t, tok.Type, token.STRING_BEG)
	assert.Equal(t, l.Lit(tok), "<<~'EOS'")
	tok = l.NextToken()
	assert.Equal(t, tok.Type, token.STRING_CONTENT)
	assert.Equal(t, l.Lit(tok), "hello\n")
	tok = l.NextToken()
	assert.Equal(t, tok.Type, token.STRING_END)
}

// --- lexHeredocBody: partial delimiter match ---

func TestLexerHeredocBodyPartialDelimMatch(t *testing.T) {
	// Per MRI, `ENDING` is body content -- only `END` on its own line is the
	// delim. The earlier behaviour matched `END` as a prefix and treated
	// `ING` as a trailer, which contradicts MRI and broke nested heredocs
	// whose body text happened to start a line with a delim-prefix.
	l := New("<<'END'\ndata\nENDING\nEND\n")
	tok := l.NextToken()
	assert.Equal(t, tok.Type, token.STRING_BEG)
	tok = l.NextToken()
	assert.Equal(t, tok.Type, token.STRING_CONTENT)
	assert.Equal(t, l.Lit(tok), "data\nENDING\n")
}

// --- lexHeredocBody: unterminated ---

func TestLexerHeredocBodyUnterminated(t *testing.T) {
	l := New("<<'EOS'\nbody without closing delim")
	tok := l.NextToken()
	assert.Equal(t, tok.Type, token.STRING_BEG)
	tok = l.NextToken()
	assert.Equal(t, tok.Type, token.ILLEGAL)
}

// --- lexHeredocContent: partial delimiter match in interpolating heredoc ---

func TestLexerHeredocContentPartialDelimMatch(t *testing.T) {
	// "EOST" starts with "EOS" -- matching loop stops at the prefix, treats it as delim.
	l := New("<<EOS\nline\nEOST\nEOS\n")
	tok := l.NextToken() // STRING_BEG
	tok = l.NextToken()  // STRING_CONTENT "line\n"
	assert.Equal(t, l.Lit(tok), "line\n")
	// "EOST" matches as delim, "T\n" consumed as trailer. Then EOS\n matches properly.
	tok = l.NextToken() // STRING_CONTENT or STRING_END
	// The lexer emits STRING_CONTENT "" then STRING_END
	if tok.Type == token.STRING_CONTENT && l.Lit(tok) == "" {
		tok = l.NextToken() // skip empty content
	}
	assert.Equal(t, tok.Type, token.STRING_END)
}

// --- lexHeredocContent: escape in interpolating heredoc ---

func TestLexerHeredocContentEscape(t *testing.T) {
	l := New("<<EOS\nline with \\x41 hex\nEOS\n")
	tok := l.NextToken() // STRING_BEG
	tok = l.NextToken()  // STRING_CONTENT
	assert.Equal(t, l.Lit(tok), "line with \\x41 hex\n")
}

// --- lexHeredocStart: <<~ with indent but no squig (indent mode without squig) ---

func TestLexerHeredocStartIndentOnly(t *testing.T) {
	// <<- without squig but with indent
	l := New("<<-EOS\n  body\n  EOS\n")
	tok := l.NextToken() // STRING_BEG
	assert.Equal(t, tok.Type, token.STRING_BEG)
	tok = l.NextToken() // STRING_CONTENT
	assert.Equal(t, l.Lit(tok), "  body\n")
}

// --- matchHeredocDelimLine: edge cases ---

func TestLexerMatchHeredocDelimLine(t *testing.T) {
	// Constructor creates lexer, but matchHeredocDelimLine works on positions.
	// We test indirectly via heredoc lexing.

	// Non-indented heredoc: delimiter at line start after other content
	l := New("<<EOS\nline\nEOS\n")
	l.NextToken() // STRING_BEG
	tok := l.NextToken()
	assert.Equal(t, l.Lit(tok), "line\n")

	// Indented heredoc with tabs
	l2 := New("<<-EOS\n\tcontent\n\tEOS\n")
	l2.NextToken() // STRING_BEG
	tok = l2.NextToken()
	assert.Equal(t, l2.Lit(tok), "\tcontent\n")
}

// --- lexHeredocContent: unterminated ---

func TestLexerHeredocContentUnterminated(t *testing.T) {
	l := New("<<EOS\nbody without end")
	tok := l.NextToken() // STRING_BEG
	tok = l.NextToken()  // STRING_CONTENT ... or ILLEGAL
	// Should eventually produce ILLEGAL
	for l.HasNext() && tok.Type != token.ILLEGAL && tok.Type != token.EOF {
		tok = l.NextToken()
	}
	assert.Equal(t, tok.Type, token.ILLEGAL)
}

// --- lexHeredocContent: #@@var in heredoc ---

func TestLexerHeredocContentHashClassVar(t *testing.T) {
	l := New("<<EOS\nhello #@@var\nEOS\n")
	l.NextToken()        // STRING_BEG
	tok := l.NextToken() // STRING_CONTENT "hello "
	if tok.Type != token.STRING_CONTENT || l.Lit(tok) != "hello " {
		t.Fatalf("expected STRING_CONTENT 'hello ', got %s %q", tok.Type, l.Lit(tok))
	}
	tok = l.NextToken() // CLASS_VAR
	assert.Equal(t, tok.Type, token.CLASS_VAR)
}

// --- lexHeredocContent: #@var in heredoc ---

func TestLexerHeredocContentHashAt(t *testing.T) {
	l := New("<<EOS\nhello #@name\nEOS\n")
	l.NextToken()        // STRING_BEG
	tok := l.NextToken() // STRING_CONTENT "hello "
	if tok.Type != token.STRING_CONTENT || l.Lit(tok) != "hello " {
		t.Fatalf("expected STRING_CONTENT 'hello ', got %s %q", tok.Type, l.Lit(tok))
	}
	tok = l.NextToken() // AT
	assert.Equal(t, tok.Type, token.AT)
}

// --- lexPercentLiteral: %s with non-paired delimiter ---

func TestLexerPercentS(t *testing.T) {
	l := New("%s/sym/")
	tok := l.NextToken() // STRING_BEG "s"
	assert.Equal(t, tok.Type, token.STRING_BEG)
	tok = l.NextToken() // STRING_CONTENT
	assert.Equal(t, tok.Type, token.STRING_CONTENT)
	tok = l.NextToken() // STRING_END
	assert.Equal(t, tok.Type, token.STRING_END)
}

// --- consumeEscape: \C-\\ in string ---

func TestLexerEscapeControlBackslash(t *testing.T) {
	// \C-\\ in string: control escapes target a backslash
	l := New("\"\\C-\\\\\"")
	tok := l.NextToken() // STRING_BEG
	tok = l.NextToken()  // STRING_CONTENT
	assert.Equal(t, l.Lit(tok), "\\C-\\\\")
}

// --- consumeEscape: \M-\\ in string ---

func TestLexerEscapeMetaBackslash(t *testing.T) {
	l := New("\"\\M-\\\\\"")
	tok := l.NextToken() // STRING_BEG
	tok = l.NextToken()  // STRING_CONTENT
	assert.Equal(t, l.Lit(tok), "\\M-\\\\")
}

// --- lexDigit: trailing underscore and zero-underscore forms ---

func TestLexerDigitTrailingUnderscore(t *testing.T) {
	// Integer with trailing underscore includes it in the literal.
	l := New("1_")
	tok := l.NextToken()
	assert.Equal(t, tok.Type, token.INT)
	assert.Equal(t, l.Lit(tok), "1_")
}

func TestLexerDigitZeroUnderscore(t *testing.T) {
	// 0_1: zero followed by underscore then digit (octal-like but decimal).
	l := New("0_1")
	tok := l.NextToken()
	assert.Equal(t, tok.Type, token.INT)
	assert.Equal(t, l.Lit(tok), "0_1")
}

func TestLexerDigitFloatTrailingUnderscore(t *testing.T) {
	// Float with trailing underscore includes it.
	l := New("1.5_")
	tok := l.NextToken()
	assert.Equal(t, tok.Type, token.FLOAT)
	assert.Equal(t, l.Lit(tok), "1.5_")
}

func TestLexerDigitFloatExpNegativeNoDigit(t *testing.T) {
	// 1.5e- : MRI rejects as syntax error (trailing 'e' in number).
	l := New("1.5e-")
	tok := l.NextToken()
	assert.Equal(t, tok.Type, token.ILLEGAL)
	assert.Equal(t, l.Lit(tok), "trailing 'e' in number")
}

func TestLexerDigitFloatExpThenNonDigit(t *testing.T) {
	// 1e10x: exponent with digit then non-digit
	l := New("1e10x")
	tok := l.NextToken()
	assert.Equal(t, tok.Type, token.FLOAT)
	assert.Equal(t, l.Lit(tok), "1e10")
	tok = l.NextToken()
	if tok.Type != token.IDENT || l.Lit(tok) != "x" {
		t.Errorf("expected IDENT x, got %s %q", tok.Type, l.Lit(tok))
	}
}

// --- lexHeredocBody: squiggy literal with indented delim ---

func TestLexerHeredocBodySquigIndentedDelim(t *testing.T) {
	// <<~'EOS' with indented closing delimiter
	l := New("<<~'EOS'\n  line\n  EOS\n")
	tok := l.NextToken()
	assert.Equal(t, tok.Type, token.STRING_BEG)
	tok = l.NextToken()
	assert.Equal(t, tok.Type, token.STRING_CONTENT)
	assert.Equal(t, l.Lit(tok), "line\n")
	tok = l.NextToken()
	assert.Equal(t, tok.Type, token.STRING_END)
}

// --- lexHeredocBody: unterminated at delim line without newline ---

func TestLexerHeredocBodyNoTrailingNewline(t *testing.T) {
	l := New("<<'EOS'\nbody\nEOS")
	tok := l.NextToken()
	assert.Equal(t, tok.Type, token.STRING_BEG)
	tok = l.NextToken()
	assert.Equal(t, tok.Type, token.STRING_CONTENT)
	assert.Equal(t, l.Lit(tok), "body\n")
	tok = l.NextToken()
	assert.Equal(t, tok.Type, token.STRING_END)
}

// --- matchHeredocDelimLine: indented with tabs ---

func TestLexerHeredocMatchIndentedTabs(t *testing.T) {
	l := New("<<-EOS\nline\n\t\tEOS\n")
	l.NextToken()        // STRING_BEG
	tok := l.NextToken() // STRING_CONTENT
	assert.Equal(t, l.Lit(tok), "line\n")
	tok = l.NextToken() // STRING_END
	assert.Equal(t, tok.Type, token.STRING_END)
}

// --- setupSquigBodyBuffer: delim not found (no closing delim) ---

func TestLexerStripSquigInterpBodyNoDelim(t *testing.T) {
	// setupSquigBodyBuffer with no matching delim returns early.
	// Since no delim is found, content is never emitted - just errors.
	l := New("<<~EOS\n  line\n  NOMATCH\n")
	tok := l.NextToken() // STRING_BEG
	assert.Equal(t, tok.Type, token.STRING_BEG)
	// No closing delim found, so content accumulates and then errors.
	tok = l.NextToken()
	assert.Equal(t, tok.Type, token.ILLEGAL)
}

// --- setupSquigBodyBuffer: min indent from blank lines ---

func TestLexerStripSquigAllBlank(t *testing.T) {
	// All lines blank except last - min indent comes from non-blank
	l := New("<<~EOS\n\n\n  line\n  EOS\n")
	l.NextToken()        // STRING_BEG
	tok := l.NextToken() // STRING_CONTENT
	assert.Equal(t, l.Lit(tok), "\n\nline\n")
}

// --- lexCharacterLiteral: control char with backslash target ---

func TestLexerCharLiteralControlBackslashTarget(t *testing.T) {
	l := New("?\\C-\\\\")
	tok := l.NextToken()
	assert.Equal(t, tok.Type, token.STRING)
	assert.Equal(t, l.Lit(tok), "\\C-\\\\")
}

// --- lexCharacterLiteral: meta char with backslash target ---

func TestLexerCharLiteralMetaBackslashTarget(t *testing.T) {
	l := New("?\\M-\\\\")
	tok := l.NextToken()
	assert.Equal(t, tok.Type, token.STRING)
	assert.Equal(t, l.Lit(tok), "\\M-\\\\")
}

// --- startLexer: ** operator (POWER) ---

func TestLexerPowerOperator(t *testing.T) {
	l := New("a ** b")
	l.NextToken() // IDENT a
	tok := l.NextToken()
	assert.Equal(t, tok.Type, token.POWER)
	assert.Equal(t, l.Lit(tok), "**")
}

// --- startLexer: &&= (ANDASSIGN) ---

func TestLexerAndAssign(t *testing.T) {
	l := New("x &&= y")
	l.NextToken() // IDENT x
	tok := l.NextToken()
	assert.Equal(t, tok.Type, token.ANDASSIGN)
}

// --- startLexer: ||= (ORASSIGN) ---

func TestLexerOrAssign(t *testing.T) {
	l := New("x ||= y")
	l.NextToken() // IDENT x
	tok := l.NextToken()
	assert.Equal(t, tok.Type, token.ORASSIGN)
}

// --- lexDigit: 0.5 (zero-prefix float fraction) ---

func TestLexerZeroDotDigit(t *testing.T) {
	l := New("0.5")
	tok := l.NextToken()
	assert.Equal(t, tok.Type, token.FLOAT)
	assert.Equal(t, l.Lit(tok), "0.5")
}

// --- lexDigit: integer exponent with sign ---

func TestLexerIntExpWithSign(t *testing.T) {
	l := New("1e+10")
	tok := l.NextToken()
	assert.Equal(t, tok.Type, token.FLOAT)
	assert.Equal(t, l.Lit(tok), "1e+10")
}

func TestLexerIntExpNegWithSign(t *testing.T) {
	l := New("1e-10")
	tok := l.NextToken()
	assert.Equal(t, tok.Type, token.FLOAT)
	assert.Equal(t, l.Lit(tok), "1e-10")
}

// --- lexCharacterLiteral: space as character (error) ---

func TestLexerCharLiteralSpace(t *testing.T) {
	// ? followed by space emits QMARK (via startLexer), not reaching lexCharacterLiteral.
	// The error branch in lexCharacterLiteral is unreachable through normal lexing
	// because startLexer catches all whitespace before delegating.
	l := New("? ")
	tok := l.NextToken()
	assert.Equal(t, tok.Type, token.QMARK)
}

// --- lexCharacterLiteral: unterminated unicode escape ---

func TestLexerCharLiteralUnterminatedUnicode(t *testing.T) {
	l := New("?\\u{41")
	tok := l.NextToken()
	assert.Equal(t, tok.Type, token.ILLEGAL)
}

// --- lexCharacterLiteral: unterminated octal escape ---

func TestLexerCharLiteralUnterminatedOctal(t *testing.T) {
	l := New("?\\o{77")
	tok := l.NextToken()
	assert.Equal(t, tok.Type, token.ILLEGAL)
}

// --- lexStringContent: unterminated string ---

func TestLexerUnterminatedString(t *testing.T) {
	l := New("\"no closing quote")
	tok := l.NextToken() // STRING_BEG
	tok = l.NextToken()  // scan until error
	for l.HasNext() && tok.Type != token.ILLEGAL {
		tok = l.NextToken()
	}
	assert.Equal(t, tok.Type, token.ILLEGAL)
}

// --- lexPercentLiteral: unknown type ---

func TestLexerPercentUnknownType(t *testing.T) {
	// %z: 'z' is not a percent-type char, treated as bare % with 'z' as delimiter.
	// The default case in lexPercentLiteral's switch is unreachable because
	// isPercentTypeChar covers all valid types and bare-% maps to typ=0.
	l := New("%z{content}")
	tok := l.NextToken()
	assert.Equal(t, tok.Type, token.STRING_BEG)
}

// --- lexPercentLiteralBody: unterminated ---

func TestLexerPercentLiteralUnterminated(t *testing.T) {
	l := New("%q(no closing")
	tok := l.NextToken()
	assert.Equal(t, tok.Type, token.ILLEGAL)
}

// --- lexPercentLiteralBodyEnd: unterminated ---

func TestLexerPercentBodyEndUnterminated(t *testing.T) {
	l := New("%w(no closing")
	tok := l.NextToken() // STRING_BEG
	tok = l.NextToken()  // scan until ILLEGAL
	for l.HasNext() && tok.Type != token.ILLEGAL {
		tok = l.NextToken()
	}
	assert.Equal(t, tok.Type, token.ILLEGAL)
}

// --- lexPercentContent: unterminated ---

func TestLexerPercentContentUnterminated(t *testing.T) {
	l := New("%Q{no closing")
	tok := l.NextToken() // STRING_BEG
	tok = l.NextToken()  // scan until ILLEGAL
	for l.HasNext() && tok.Type != token.ILLEGAL {
		tok = l.NextToken()
	}
	assert.Equal(t, tok.Type, token.ILLEGAL)
}

// --- lexBacktickContent: unterminated command literal ---

func TestLexerBacktickUnterminated(t *testing.T) {
	l := New("`no closing backtick")
	tok := l.NextToken() // XSTR_BEG
	tok = l.NextToken()  // scan until ILLEGAL
	for l.HasNext() && tok.Type != token.ILLEGAL {
		tok = l.NextToken()
	}
	assert.Equal(t, tok.Type, token.ILLEGAL)
}

// --- lexHeredocStart: heredoc without trailing newline ---

func TestLexerHeredocNoTrailingNewline(t *testing.T) {
	l := New("<<EOS")
	tok := l.NextToken() // STRING_BEG
	assert.Equal(t, tok.Type, token.STRING_BEG)
}

// --- lexHeredocBody: empty body with squiggy ---

func TestLexerHeredocBodyEmptySquiggy(t *testing.T) {
	l := New("<<~'EOS'\nEOS\n")
	tok := l.NextToken()
	assert.Equal(t, tok.Type, token.STRING_BEG)
	tok = l.NextToken()
	assert.Equal(t, tok.Type, token.STRING_CONTENT)
	assert.Equal(t, l.Lit(tok), "")
	tok = l.NextToken()
	assert.Equal(t, tok.Type, token.STRING_END)
}

// --- lexHeredocBody: unterminated at contentEnd ---

func TestLexerHeredocBodyUnterminatedMidBody(t *testing.T) {
	l := New("<<'EOS'\nline\n")
	tok := l.NextToken()
	assert.Equal(t, tok.Type, token.STRING_BEG)
	tok = l.NextToken()
	assert.Equal(t, tok.Type, token.ILLEGAL)
}

// --- lexHeredocBody: indented unterminated ---

func TestLexerHeredocBodyIndentedUnterminated(t *testing.T) {
	l := New("<<-'EOS'\nline\n  ")
	tok := l.NextToken()
	assert.Equal(t, tok.Type, token.STRING_BEG)
	tok = l.NextToken()
	assert.Equal(t, tok.Type, token.ILLEGAL)
}

// --- lexHeredocBody: eof during delim char matching ---

func TestLexerHeredocBodyEOFDuringDelim(t *testing.T) {
	l := New("<<'EOS'\nline\nE")
	tok := l.NextToken()
	assert.Equal(t, tok.Type, token.STRING_BEG)
	tok = l.NextToken()
	assert.Equal(t, tok.Type, token.ILLEGAL)
}

// --- lexHeredocContent: eof during indented check ---

func TestLexerHeredocContentIndentedUnterminated(t *testing.T) {
	l := New("<<-EOS\nline\n  ")
	tok := l.NextToken() // STRING_BEG
	tok = l.NextToken()  // scan
	for l.HasNext() && tok.Type != token.ILLEGAL {
		tok = l.NextToken()
	}
	assert.Equal(t, tok.Type, token.ILLEGAL)
}

// --- lexHeredocContent: eof during delim char matching ---

func TestLexerHeredocContentEOFDuringDelim(t *testing.T) {
	l := New("<<-EOS\nline\nE")
	tok := l.NextToken() // STRING_BEG
	tok = l.NextToken()  // scan
	for l.HasNext() && tok.Type != token.ILLEGAL {
		tok = l.NextToken()
	}
	assert.Equal(t, tok.Type, token.ILLEGAL)
}

// --- lexHeredocContent: matched delim at lineStart (empty content) ---

func TestLexerHeredocContentMatchAtLineStart(t *testing.T) {
	l := New("<<EOS\nEOS\n")
	l.NextToken()        // STRING_BEG
	tok := l.NextToken() // STRING_CONTENT (empty) or STRING_END
	if tok.Type != token.STRING_CONTENT && tok.Type != token.STRING_END {
		t.Fatalf("expected STRING_CONTENT or STRING_END, got %s (%q)", tok.Type, l.Lit(tok))
	}
}

// --- matchHeredocDelimLine: extra chars after delim ---

func TestLexerMatchHeredocDelimLineExtraChars(t *testing.T) {
	l := New("<<'EOS'\nEOSextra\nEOS\n")
	tok := l.NextToken() // STRING_BEG
	assert.Equal(t, tok.Type, token.STRING_BEG)
	tok = l.NextToken() // STRING_CONTENT
	assert.Equal(t, tok.Type, token.STRING_CONTENT)
	assert.Equal(t, l.Lit(tok), "EOSextra\n")
}

// --- stripHeredocIndent: minIndent == 0 returns original ---

func TestLexerStripHeredocIndentZero(t *testing.T) {
	got := stripHeredocIndent("a\n  b\n  c")
	assert.Equal(t, got, "a\n  b\n  c")
}

// --- lexRegexContent: unterminated regex ---

func TestLexerRegexUnterminated(t *testing.T) {
	l := New("/no closing slash")
	tok := l.NextToken() // REGEX_BEG
	tok = l.NextToken()  // scan until ILLEGAL
	for l.HasNext() && tok.Type != token.ILLEGAL {
		tok = l.NextToken()
	}
	assert.Equal(t, tok.Type, token.ILLEGAL)
}

// --- consumeEscape: unterminated \u{ returns on eof ---

func TestLexerEscapeUnterminatedUnicode(t *testing.T) {
	// \u{ without closing } consumes everything including the closing "
	// consumeEscape returns via eof branch, then string scanner reports unterminated.
	l := New("\"\\u{41\"")
	tok := l.NextToken() // STRING_BEG
	assert.Equal(t, tok.Type, token.STRING_BEG)
	tok = l.NextToken() // ILLEGAL (unterminated string)
	assert.Equal(t, tok.Type, token.ILLEGAL)
}

func TestLexerEscapeUnterminatedOctal(t *testing.T) {
	// Same as above for \o{ without closing }
	l := New("\"\\o{77\"")
	tok := l.NextToken() // STRING_BEG
	assert.Equal(t, tok.Type, token.STRING_BEG)
	tok = l.NextToken() // ILLEGAL (unterminated string)
	assert.Equal(t, tok.Type, token.ILLEGAL)
}

// --- setupSquigBodyBuffer: eol at end of input (last line no \n) ---

func TestLexerSquigBodyLastLineNoNL(t *testing.T) {
	// Body has a line without trailing \n and no closing delim.
	// setupSquigBodyBuffer hits eol >= len(l.input) and returns early.
	l := New("<<~EOS\n  line")
	tok := l.NextToken() // STRING_BEG
	assert.Equal(t, tok.Type, token.STRING_BEG)
	// Body content is "  line" (no newline at end, no matching delim).
	// The lexer keeps scanning and eventually errors.
	tok = l.NextToken()
	assert.Equal(t, tok.Type, token.ILLEGAL)
}

// --- setupSquigBodyBuffer: all-blank lines before delim (minIndent < 0) ---

// --- direct call to lexCharacterLiteral to hit whitespace error path ---

func TestLexerCharLiteralWhitespaceErrorDirect(t *testing.T) {
	// The whitespace error in lexCharacterLiteral (line 884) is unreachable
	// through normal lexing because startLexer catches all whitespace before
	// delegating. Call lexCharacterLiteral directly with a lexer whose next
	// character is a space.
	l := New(" ")
	// Simulate: ? has been consumed and ignored, now at the space.
	l.ignore() // skip ?
	l.state = nil
	// Directly call lexCharacterLiteral
	state := lexCharacterLiteral(l)
	if state != nil {
		t.Error("expected nil state (error)")
	}
}

func TestLexerSquigBodyAllBlankBeforeDelim(t *testing.T) {
	// All lines before the delimiter are blank, so minIndent stays at -1.
	// setupSquigBodyBuffer sets minIndent = 0.
	l := New("<<~EOS\n\n\nEOS\n")
	tok := l.NextToken() // STRING_BEG
	assert.Equal(t, tok.Type, token.STRING_BEG)
	tok = l.NextToken() // STRING_CONTENT
	assert.Equal(t, tok.Type, token.STRING_CONTENT)
	assert.Equal(t, l.Lit(tok), "\n\n")
}

func TestLexerRegressions(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		checks []struct {
			typ token.Type
			lit string
		}
	}{
		{
			"backtick after def",
			"def `(cmd)",
			[]struct {
				typ token.Type
				lit string
			}{
				{token.DEF, "def"},
				{token.IDENT, "`"},
				{token.LPAREN, "("},
				{token.IDENT, "cmd"},
				{token.RPAREN, ")"},
			},
		},
		{
			"backtick as symbol",
			":`",
			[]struct {
				typ token.Type
				lit string
			}{
				{token.SYMBEG, ":"},
				{token.IDENT, "`"},
			},
		},
		{
			"multi-digit global",
			"$12",
			[]struct {
				typ token.Type
				lit string
			}{
				{token.GLOBAL, "$12"},
			},
		},
		{
			"bang label",
			"save!:",
			[]struct {
				typ token.Type
				lit string
			}{
				{token.LABEL, "save!:"},
			},
		},
		{
			"question label",
			"valid?:",
			[]struct {
				typ token.Type
				lit string
			}{
				{token.LABEL, "valid?:"},
			},
		},
		{
			"no label after dot",
			".foo:",
			[]struct {
				typ token.Type
				lit string
			}{
				{token.DOT, "."},
				{token.IDENT, "foo"},
				{token.SYMBEG, ":"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := New(tt.input)
			for _, check := range tt.checks {
				tok := l.NextToken()
				assert.That(t, !(tok.Type != check.typ), "expected %s, got %s (%q)", check.typ, tok.Type, l.Lit(tok))
				assert.That(t, !(l.Lit(tok) != check.lit), "expected literal %q, got %q", check.lit, l.Lit(tok))
			}
		})
	}
}

// TestLexerHeredocNestedSquigInSquigInterp exercises a latent code path
// flagged in HEREDOC_PLAN.md: an outer squig heredoc whose body contains
// `#{}` interpolation that itself starts another squig heredoc.
// setupSquigBodyBuffer writes heredocSavedInput/SavedSegEnd/SquigRestorePos
// without stacking, so an inner squig strip would overwrite the outer's
// saved state. Test currently passes because the inner squig heredoc body
// region lives inside the outer's already-stripped buffer; if the layout
// of the swap fields changes in future work this canary catches the regression.
func TestLexerHeredocNestedSquigInSquigInterp(t *testing.T) {
	src := "x = <<~OUTER\n  before #{<<~INNER\n  inner body\n  INNER\n} after\nOUTER\n"
	l := New(src)
	got := []token.Type{}
	for l.HasNext() {
		tok := l.NextToken()
		got = append(got, tok.Type)
		if tok.Type == token.EOF {
			break
		}
	}
	// Minimal correctness check: expect STRING_BEG ... STRING_END pair for each
	// heredoc tag (outer and inner), with the inner pair fully nested inside
	// the outer's body emit sequence. Don't assert exact token positions --
	// just structural sanity.
	stringBegs := 0
	stringEnds := 0
	for _, tt := range got {
		switch tt {
		case token.STRING_BEG:
			stringBegs++
		case token.STRING_END:
			stringEnds++
		}
	}
	assert.Equal(t, stringBegs, 2)
	assert.Equal(t, stringEnds, 2)
}
