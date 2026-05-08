package lexer

import (
	"testing"

	"github.com/lczyk/goruby/token"
)

// --- escape edge case: \o{...} in string (consumeEscape braces-octal branch) ---

func TestLexerBraceOctalInString(t *testing.T) {
	input := "\"\\o{101}\""
	l := New(input)
	tok := l.NextToken()
	if tok.Type != token.STRING_BEG {
		t.Fatalf("expected STRING_BEG, got %s", tok.Type)
	}
	tok = l.NextToken()
	if tok.Type != token.STRING_CONTENT {
		t.Fatalf("expected STRING_CONTENT, got %s", tok.Type)
	}
	if tok.Literal != "\\o{101}" {
		t.Errorf("expected literal '\\o{101}', got %q", tok.Literal)
	}
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
		{"unicode without braces", "?\\u0041", "\\u"},
		{"meta of backslashed newline", "?\\M-\\n", "\\M-\\n"},
		{"control of backslashed tab", "?\\C-\\t", "\\C-\\t"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := New(tt.input)
			tok := l.NextToken()
			if tok.Type != token.STRING {
				t.Fatalf("expected STRING, got %s (%q)", tok.Type, tok.Literal)
			}
			if tok.Literal != tt.expected {
				t.Errorf("expected literal %q, got %q", tt.expected, tok.Literal)
			}
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
				{token.STRING_BEG, ""},
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
				{token.STRING_BEG, ""},
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
				{token.STRING_BEG, ""},
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
				{token.XSTR_BEG, ""},
				{token.XSTR_CONTENT, "cmd\n"},
				{token.XSTR_END, ""},
				{token.NEWLINE, "\n"},
			},
		},
		{
			name:  "squiggy interpolating heredoc",
			input: "<<~EOS\n  body\n  EOS\n",
			expected: []expTok{
				{token.STRING_BEG, ""},
				{token.STRING_CONTENT, "body\n"},
				{token.STRING_END, ""},
				{token.NEWLINE, "\n"},
			},
		},
		{
			name:  "squiggy heredoc with blank lines",
			input: "<<~EOS\n  line1\n\n  line2\n  EOS\n",
			expected: []expTok{
				{token.STRING_BEG, ""},
				{token.STRING_CONTENT, "line1\n\nline2\n"},
				{token.STRING_END, ""},
				{token.NEWLINE, "\n"},
			},
		},
		{
			name:  "heredoc with #{expr} in body",
			input: "<<EOS\nhello #{name}\nEOS\n",
			expected: []expTok{
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
		{
			name:  "heredoc empty body with trailer",
			input: "<<EOS.chop\nEOS\n",
			expected: []expTok{
				{token.STRING_BEG, ""},
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
				{token.STRING_BEG, ""},
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

// --- NextToken after exhaust ---

func TestLexerNextTokenAfterExhaust(t *testing.T) {
	input := "x"
	l := New(input)
	tok := l.NextToken()
	if tok.Type != token.IDENT {
		t.Fatalf("expected IDENT, got %s", tok.Type)
	}
	for i := 0; i < 3; i++ {
		tok := l.NextToken()
		if tok.Type != token.EOF {
			t.Errorf("call %d after exhaust: expected EOF, got %s", i, tok.Type)
		}
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

// --- single-quote string backslash edge case ---

func TestLexerSingleQuoteBackslashEdge(t *testing.T) {
	input := "'\\\\'"
	l := New(input)
	tok := l.NextToken()
	if tok.Type != token.STRING {
		t.Fatalf("expected STRING, got %s", tok.Type)
	}
	if tok.Literal != "\\\\" {
		t.Errorf("expected literal '\\\\\\\\', got %q", tok.Literal)
	}
}

// --- NextToken: cover channel-closed path (error then drain) ---

func TestLexerNextTokenAfterError(t *testing.T) {
	// Single backslash triggers errorf which sets state=nil, then channel closes after ILLEGAL drained.
	l := New("\\")
	tok := l.NextToken()
	if tok.Type != token.ILLEGAL {
		t.Fatalf("expected ILLEGAL, got %s", tok.Type)
	}
	// Channel should now be closed; next call reads from closed channel (ok=false path).
	tok = l.NextToken()
	if tok.Type != token.EOF {
		t.Errorf("expected EOF after error, got %s", tok.Type)
	}
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
			name:  "e without digit not exponent",
			input: "1e",
			expected: []expTok{
				{token.INT, "1e"},
			},
		},
		{
			name:  "E+ without digit not exponent",
			input: "1E+",
			expected: []expTok{
				{token.INT, "1E+"},
			},
		},
		{
			name:  "e- without digit not exponent",
			input: "1e-",
			expected: []expTok{
				{token.INT, "1e-"},
			},
		},
		{
			name:  "0e+x not exponent",
			input: "0e+x",
			expected: []expTok{
				{token.INT, "0"},
				{token.IDENT, "e"},
				{token.PLUS, "+"},
				{token.IDENT, "x"},
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
			name:  "float exponent with sign no digit",
			input: "1.5e+",
			expected: []expTok{
				{token.FLOAT, "1.5e+"},
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

// --- lexGlobal: edge cases for error paths ---

func TestLexerGlobalIllegalChar(t *testing.T) {
	// $ followed by whitespace should be an error
	l := New("$ ")
	tok := l.NextToken()
	if tok.Type != token.ILLEGAL {
		t.Errorf("expected ILLEGAL, got %s (%q)", tok.Type, tok.Literal)
	}
}

func TestLexerGlobalExprDelim(t *testing.T) {
	// $ followed by expression delimiter like newline
	l := New("$\n")
	tok := l.NextToken()
	if tok.Type != token.ILLEGAL {
		t.Errorf("expected ILLEGAL for $ at EOL, got %s (%q)", tok.Type, tok.Literal)
	}
}

// --- lexSingleQuoteString: unterminated ---

func TestLexerUnterminatedSingleQuote(t *testing.T) {
	l := New("'unterminated")
	tok := l.NextToken()
	if tok.Type != token.ILLEGAL {
		t.Errorf("expected ILLEGAL for unterminated string, got %s", tok.Type)
	}
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
				{token.STRING_BEG, ""},
				{token.STRING_CONTENT, "  body\n"},
				{token.STRING_END, ""},
				{token.NEWLINE, "\n"},
			},
		},
		{
			name:  "<<- with no indent on delim line",
			input: "<<-EOS\nbody\nEOS\n",
			expected: []expTok{
				{token.STRING_BEG, ""},
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
			if got != tt.want {
				t.Errorf("stripHeredocIndent(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// --- heredoc: <<~ with backtick quote ---

func TestLexerSquiggyBacktickHeredoc(t *testing.T) {
	l := New("<<~`EOS`\n  cmd\n  EOS\n")
	tok := l.NextToken()
	if tok.Type != token.XSTR_BEG {
		t.Fatalf("expected XSTR_BEG, got %s", tok.Type)
	}
	tok = l.NextToken()
	if tok.Type != token.XSTR_CONTENT {
		t.Fatalf("expected XSTR_CONTENT, got %s", tok.Type)
	}
	if tok.Literal != "cmd\n" {
		t.Errorf("expected 'cmd\\n', got %q", tok.Literal)
	}
	tok = l.NextToken()
	if tok.Type != token.XSTR_END {
		t.Fatalf("expected XSTR_END, got %s", tok.Type)
	}
}

// --- startLexer: __END__ marker ---

func TestLexerEndMarkerAfterNewline(t *testing.T) {
	// __END__ at line start consumes rest of input
	l := New("x\n__END__\nrest of file")
	// First: IDENT "x"
	tok := l.NextToken()
	if tok.Type != token.IDENT || tok.Literal != "x" {
		t.Fatalf("expected IDENT x, got %s %q", tok.Type, tok.Literal)
	}
	// Then NEWLINE
	tok = l.NextToken()
	if tok.Type != token.NEWLINE {
		t.Fatalf("expected NEWLINE, got %s", tok.Type)
	}
	// Then EOF (__END__ consumed rest)
	tok = l.NextToken()
	if tok.Type != token.EOF {
		t.Fatalf("expected EOF, got %s (%q)", tok.Type, tok.Literal)
	}
}

// --- startLexer: ? with whitespace emits QMARK ---

func TestLexerQMarkWhitespace(t *testing.T) {
	l := New("? foo")
	tok := l.NextToken()
	if tok.Type != token.QMARK {
		t.Fatalf("expected QMARK, got %s", tok.Type)
	}
}

// --- startLexer: ? with expression delimiter produces error ---

func TestLexerQMarkExprDelim(t *testing.T) {
	l := New("?\n")
	tok := l.NextToken()
	if tok.Type != token.ILLEGAL {
		t.Errorf("expected ILLEGAL for ? at EOL, got %s (%q)", tok.Type, tok.Literal)
	}
}

// --- startLexer: < at start ---

func TestLexerLTAngle(t *testing.T) {
	l := New("a < b")
	// IDENT "a"
	l.NextToken()
	// LT "<"
	tok := l.NextToken()
	if tok.Type != token.LT {
		t.Fatalf("expected LT, got %s", tok.Type)
	}
	if tok.Literal != "<" {
		t.Errorf("expected '<', got %q", tok.Literal)
	}
}

// --- startLexer: <<= (LShift assign) ---

func TestLexerLShiftAssign(t *testing.T) {
	l := New("x <<= 2")
	l.NextToken() // IDENT
	tok := l.NextToken()
	if tok.Type != token.LSHIFTASSIGN {
		t.Fatalf("expected LSHIFTASSIGN, got %s", tok.Type)
	}
	if tok.Literal != "<<=" {
		t.Errorf("expected '<<=', got %q", tok.Literal)
	}
}

// --- startLexer: >> and >>= ---

func TestLexerRShiftOps(t *testing.T) {
	l := New(">>")
	tok := l.NextToken()
	if tok.Type != token.RSHIFT {
		t.Fatalf("expected RSHIFT, got %s", tok.Type)
	}
	l2 := New(">>= 2")
	tok2 := l2.NextToken()
	if tok2.Type != token.RSHIFTASSIGN {
		t.Fatalf("expected RSHIFTASSIGN, got %s (%q)", tok2.Type, tok2.Literal)
	}
}

// --- startLexer: >= ---

func TestLexerGTE(t *testing.T) {
	l := New(">=")
	tok := l.NextToken()
	if tok.Type != token.GTE {
		t.Fatalf("expected GTE, got %s", tok.Type)
	}
}

// --- startLexer: > ---

func TestLexerGT(t *testing.T) {
	l := New("a > b")
	l.NextToken()
	tok := l.NextToken()
	if tok.Type != token.GT {
		t.Fatalf("expected GT, got %s", tok.Type)
	}
}

// --- startLexer: lshift/<< non-heredoc with non-alpha after ---

func TestLexerLShiftNonHeredoc(t *testing.T) {
	// << followed by non-letter, non-underscore (not a heredoc)
	l := New("<<2")
	tok := l.NextToken()
	if tok.Type != token.LSHIFT {
		t.Fatalf("expected LSHIFT, got %s", tok.Type)
	}
	// Then INT 2
	tok = l.NextToken()
	if tok.Type != token.INT {
		t.Fatalf("expected INT, got %s", tok.Type)
	}
}

// --- startLexer: class << expr (singleton class, not heredoc) ---

func TestLexerClassLShift(t *testing.T) {
	l := New("class << self\nend")
	tok := l.NextToken()
	if tok.Type != token.CLASS {
		t.Fatalf("expected CLASS, got %s", tok.Type)
	}
	tok = l.NextToken()
	if tok.Type != token.LSHIFT {
		t.Fatalf("expected LSHIFT, got %s (%q)", tok.Type, tok.Literal)
	}
}

// --- startLexer: <<~ followed by non-alpha (should be lshift, not heredoc) ---

func TestLexerSquigNotHeredoc(t *testing.T) {
	// <<~ followed by a digit: not a heredoc, just << and ~ and digit
	l := New("<<~2")
	tok := l.NextToken()
	if tok.Type != token.LSHIFT {
		t.Fatalf("expected LSHIFT, got %s", tok.Type)
	}
	tok = l.NextToken()
	if tok.Type != token.TILDE {
		t.Fatalf("expected TILDE, got %s", tok.Type)
	}
}

// --- startLexer: <<- followed by non-alpha (not heredoc) ---

func TestLexerIndentNotHeredoc(t *testing.T) {
	l := New("<<-2")
	tok := l.NextToken()
	if tok.Type != token.LSHIFT {
		t.Fatalf("expected LSHIFT, got %s", tok.Type)
	}
	tok = l.NextToken()
	if tok.Type != token.MINUS {
		t.Fatalf("expected MINUS, got %s", tok.Type)
	}
}

// --- startLexer: illegal char ---

func TestLexerIllegalChar(t *testing.T) {
	// \ without newline should error
	l := New("\\x")
	tok := l.NextToken()
	if tok.Type != token.ILLEGAL {
		t.Errorf("expected ILLEGAL for lone backslash, got %s (%q)", tok.Type, tok.Literal)
	}
}

// --- startLexer: bare % as modulo ---

func TestLexerModulo(t *testing.T) {
	l := New("a%2")
	l.NextToken() // IDENT
	tok := l.NextToken()
	if tok.Type != token.MODULO {
		t.Fatalf("expected MODULO, got %s", tok.Type)
	}
}

// --- lexStringContent: #$ followed by @ in string ---

func TestLexerStringHashAt(t *testing.T) {
	l := New("\"#@foo\"")
	tok := l.NextToken() // STRING_BEG
	tok = l.NextToken()  // AT
	if tok.Type != token.AT {
		t.Fatalf("expected AT, got %s (%q)", tok.Type, tok.Literal)
	}
	tok = l.NextToken() // IDENT "foo"
	if tok.Type != token.IDENT || tok.Literal != "foo" {
		t.Fatalf("expected IDENT foo, got %s %q", tok.Type, tok.Literal)
	}
}

// --- lexStringContent: #$var in string (already tested, test #@@var) ---

func TestLexerStringHashClassVar(t *testing.T) {
	l := New("\"#@@cvar\"")
	tok := l.NextToken() // STRING_BEG
	tok = l.NextToken()  // CLASS_VAR
	if tok.Type != token.CLASS_VAR {
		t.Fatalf("expected CLASS_VAR, got %s (%q)", tok.Type, tok.Literal)
	}
	tok = l.NextToken() // IDENT
	if tok.Type != token.IDENT || tok.Literal != "cvar" {
		t.Fatalf("expected IDENT cvar, got %s %q", tok.Type, tok.Literal)
	}
}

// --- lexPercentContent: #@@var interpolation ---

func TestLexerPercentContentHashClassVar(t *testing.T) {
	l := New("%Q{hello #@@cvar}")
	tok := l.NextToken() // STRING_BEG "Q"
	tok = l.NextToken()  // STRING_CONTENT "hello "
	tok = l.NextToken()  // CLASS_VAR
	if tok.Type != token.CLASS_VAR {
		t.Fatalf("expected CLASS_VAR, got %s (%q)", tok.Type, tok.Literal)
	}
}

// --- lexPercentContent: #@var interpolation in %r regex ---

func TestLexerPercentRegexHashAt(t *testing.T) {
	l := New("%r{foo #@bar}")
	tok := l.NextToken() // REGEX_BEG
	tok = l.NextToken()  // STRING_CONTENT "foo "
	tok = l.NextToken()  // AT
	if tok.Type != token.AT {
		t.Fatalf("expected AT, got %s (%q)", tok.Type, tok.Literal)
	}
	tok = l.NextToken() // IDENT
	if tok.Type != token.IDENT || tok.Literal != "bar" {
		t.Fatalf("expected IDENT bar, got %s %q", tok.Type, tok.Literal)
	}
}

// --- lexPercentLiteral: %r with #@@var interpolation ---

func TestLexerPercentRegexHashClassVar(t *testing.T) {
	l := New("%r{foo #@@bar}")
	tok := l.NextToken() // REGEX_BEG
	tok = l.NextToken()  // STRING_CONTENT
	tok = l.NextToken()  // CLASS_VAR
	if tok.Type != token.CLASS_VAR {
		t.Fatalf("expected CLASS_VAR, got %s (%q)", tok.Type, tok.Literal)
	}
	tok = l.NextToken() // IDENT
	if tok.Type != token.IDENT || tok.Literal != "bar" {
		t.Fatalf("expected IDENT bar, got %s %q", tok.Type, tok.Literal)
	}
}

// --- lexRegexContent: #$var interpolation ---

func TestLexerRegexHashDollar(t *testing.T) {
	l := New("/foo #$bar/")
	tok := l.NextToken() // REGEX_BEG
	tok = l.NextToken()  // STRING_CONTENT "foo "
	tok = l.NextToken()  // GLOBAL
	if tok.Type != token.GLOBAL {
		t.Fatalf("expected GLOBAL, got %s (%q)", tok.Type, tok.Literal)
	}
	if tok.Literal != "$bar" {
		t.Errorf("expected $bar, got %q", tok.Literal)
	}
}

// --- lexRegexContent: #@var and #@@var ---

func TestLexerRegexHashAt(t *testing.T) {
	l := New("/foo #@bar/")
	tok := l.NextToken() // REGEX_BEG
	tok = l.NextToken()  // STRING_CONTENT "foo "
	tok = l.NextToken()  // AT
	if tok.Type != token.AT {
		t.Fatalf("expected AT, got %s", tok.Type)
	}
	tok = l.NextToken() // IDENT
	if tok.Type != token.IDENT || tok.Literal != "bar" {
		t.Fatalf("expected IDENT bar, got %s %q", tok.Type, tok.Literal)
	}
}

func TestLexerRegexHashClassVar(t *testing.T) {
	l := New("/foo #@@bar/")
	tok := l.NextToken() // REGEX_BEG
	tok = l.NextToken()  // STRING_CONTENT "foo "
	tok = l.NextToken()  // CLASS_VAR
	if tok.Type != token.CLASS_VAR {
		t.Fatalf("expected CLASS_VAR, got %s (%q)", tok.Type, tok.Literal)
	}
}

// --- lexRegexContent: escaped char in regex ---

func TestLexerRegexEscaped(t *testing.T) {
	l := New("/\\//")
	tok := l.NextToken() // REGEX_BEG
	tok = l.NextToken()  // STRING_CONTENT "\\/"
	if tok.Type != token.STRING_CONTENT {
		t.Fatalf("expected STRING_CONTENT, got %s", tok.Type)
	}
	tok = l.NextToken() // REGEX_END
	if tok.Type != token.REGEX_END {
		t.Fatalf("expected REGEX_END, got %s", tok.Type)
	}
}

// --- lexPercentLiteral: bare % after newline (regex context) ---

func TestLexerPercentAfterNewline(t *testing.T) {
	// After NEWLINE, isRegexBeginContext returns true, so % starts a percent literal
	l := New("x\n%(bar)")
	l.NextToken()        // IDENT x
	l.NextToken()        // NEWLINE
	tok := l.NextToken() // STRING_BEG "Q"
	if tok.Type != token.STRING_BEG {
		t.Fatalf("expected STRING_BEG, got %s (%q)", tok.Type, tok.Literal)
	}
}

// --- lexHeredocBody: squiggy literal heredoc ---

func TestLexerHeredocBodySquigLiteral(t *testing.T) {
	// <<~'EOS' is a literal squiggy heredoc - single STRING token with stripped indent
	l := New("<<~'EOS'\n  hello\n  EOS\n")
	tok := l.NextToken()
	if tok.Type != token.STRING {
		t.Fatalf("expected STRING, got %s", tok.Type)
	}
	if tok.Literal != "hello\n" {
		t.Errorf("expected 'hello\\n', got %q", tok.Literal)
	}
}

// --- lexHeredocBody: partial delimiter match ---

func TestLexerHeredocBodyPartialDelimMatch(t *testing.T) {
	// "ENDING" starts with "END" -- matching loop stops at the prefix, treating it as delim.
	// The "ING" part is consumed as a trailer on the delim line.
	l := New("<<'END'\ndata\nENDING\nEND\n")
	tok := l.NextToken()
	if tok.Type != token.STRING {
		t.Fatalf("expected STRING, got %s", tok.Type)
	}
	// "data\n" is content. "ENDING" matches "END" prefix, rest is trailer.
	if tok.Literal != "data\n" {
		t.Errorf("expected 'data\\n', got %q", tok.Literal)
	}
}

// --- lexHeredocBody: unterminated ---

func TestLexerHeredocBodyUnterminated(t *testing.T) {
	l := New("<<'EOS'\nbody without closing delim")
	tok := l.NextToken()
	if tok.Type != token.ILLEGAL {
		t.Errorf("expected ILLEGAL for unterminated heredoc, got %s (%q)", tok.Type, tok.Literal)
	}
}

// --- lexHeredocContent: partial delimiter match in interpolating heredoc ---

func TestLexerHeredocContentPartialDelimMatch(t *testing.T) {
	// "EOST" starts with "EOS" -- matching loop stops at the prefix, treats it as delim.
	l := New("<<EOS\nline\nEOST\nEOS\n")
	tok := l.NextToken() // STRING_BEG
	tok = l.NextToken()  // STRING_CONTENT "line\n"
	if tok.Literal != "line\n" {
		t.Errorf("expected 'line\\n', got %q", tok.Literal)
	}
	// "EOST" matches as delim, "T\n" consumed as trailer. Then EOS\n matches properly.
	tok = l.NextToken() // STRING_CONTENT or STRING_END
	// The lexer emits STRING_CONTENT "" then STRING_END
	if tok.Type == token.STRING_CONTENT && tok.Literal == "" {
		tok = l.NextToken() // skip empty content
	}
	if tok.Type != token.STRING_END {
		t.Fatalf("expected STRING_END, got %s (%q)", tok.Type, tok.Literal)
	}
}

// --- lexHeredocContent: escape in interpolating heredoc ---

func TestLexerHeredocContentEscape(t *testing.T) {
	l := New("<<EOS\nline with \\x41 hex\nEOS\n")
	tok := l.NextToken() // STRING_BEG
	tok = l.NextToken()  // STRING_CONTENT
	if tok.Literal != "line with \\x41 hex\n" {
		t.Errorf("expected 'line with \\\\x41 hex\\n', got %q", tok.Literal)
	}
}

// --- lexHeredocStart: <<~ with indent but no squig (indent mode without squig) ---

func TestLexerHeredocStartIndentOnly(t *testing.T) {
	// <<- without squig but with indent
	l := New("<<-EOS\n  body\n  EOS\n")
	tok := l.NextToken() // STRING_BEG
	if tok.Type != token.STRING_BEG {
		t.Fatalf("expected STRING_BEG, got %s", tok.Type)
	}
	tok = l.NextToken() // STRING_CONTENT
	if tok.Literal != "  body\n" {
		t.Errorf("expected '  body\\n', got %q", tok.Literal)
	}
}

// --- matchHeredocDelimLine: edge cases ---

func TestLexerMatchHeredocDelimLine(t *testing.T) {
	// Constructor creates lexer, but matchHeredocDelimLine works on positions.
	// We test indirectly via heredoc lexing.

	// Non-indented heredoc: delimiter at line start after other content
	l := New("<<EOS\nline\nEOS\n")
	l.NextToken() // STRING_BEG
	tok := l.NextToken()
	if tok.Literal != "line\n" {
		t.Errorf("expected 'line\\n', got %q", tok.Literal)
	}

	// Indented heredoc with tabs
	l2 := New("<<-EOS\n\tcontent\n\tEOS\n")
	l2.NextToken() // STRING_BEG
	tok = l2.NextToken()
	if tok.Literal != "\tcontent\n" {
		t.Errorf("expected '\\tcontent\\n', got %q", tok.Literal)
	}
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
	if tok.Type != token.ILLEGAL {
		t.Errorf("expected ILLEGAL for unterminated interpolating heredoc, got %s", tok.Type)
	}
}

// --- lexHeredocContent: #@@var in heredoc ---

func TestLexerHeredocContentHashClassVar(t *testing.T) {
	l := New("<<EOS\nhello #@@var\nEOS\n")
	l.NextToken()        // STRING_BEG
	tok := l.NextToken() // STRING_CONTENT "hello "
	if tok.Type != token.STRING_CONTENT || tok.Literal != "hello " {
		t.Fatalf("expected STRING_CONTENT 'hello ', got %s %q", tok.Type, tok.Literal)
	}
	tok = l.NextToken() // CLASS_VAR
	if tok.Type != token.CLASS_VAR {
		t.Fatalf("expected CLASS_VAR, got %s (%q)", tok.Type, tok.Literal)
	}
}

// --- lexHeredocContent: #@var in heredoc ---

func TestLexerHeredocContentHashAt(t *testing.T) {
	l := New("<<EOS\nhello #@name\nEOS\n")
	l.NextToken()        // STRING_BEG
	tok := l.NextToken() // STRING_CONTENT "hello "
	if tok.Type != token.STRING_CONTENT || tok.Literal != "hello " {
		t.Fatalf("expected STRING_CONTENT 'hello ', got %s %q", tok.Type, tok.Literal)
	}
	tok = l.NextToken() // AT
	if tok.Type != token.AT {
		t.Fatalf("expected AT, got %s (%q)", tok.Type, tok.Literal)
	}
}

// --- lexPercentLiteral: %s with non-paired delimiter ---

func TestLexerPercentS(t *testing.T) {
	l := New("%s/sym/")
	tok := l.NextToken() // STRING_BEG "s"
	if tok.Type != token.STRING_BEG {
		t.Fatalf("expected STRING_BEG, got %s", tok.Type)
	}
	tok = l.NextToken() // STRING_CONTENT
	if tok.Type != token.STRING_CONTENT {
		t.Fatalf("expected STRING_CONTENT, got %s", tok.Type)
	}
	tok = l.NextToken() // STRING_END
	if tok.Type != token.STRING_END {
		t.Fatalf("expected STRING_END, got %s", tok.Type)
	}
}

// --- consumeEscape: \C-\\ in string ---

func TestLexerEscapeControlBackslash(t *testing.T) {
	// \C-\\ in string: control escapes target a backslash
	l := New("\"\\C-\\\\\"")
	tok := l.NextToken() // STRING_BEG
	tok = l.NextToken()  // STRING_CONTENT
	if tok.Literal != "\\C-\\\\" {
		t.Errorf("expected '\\\\C-\\\\\\\\', got %q", tok.Literal)
	}
}

// --- consumeEscape: \M-\\ in string ---

func TestLexerEscapeMetaBackslash(t *testing.T) {
	l := New("\"\\M-\\\\\"")
	tok := l.NextToken() // STRING_BEG
	tok = l.NextToken()  // STRING_CONTENT
	if tok.Literal != "\\M-\\\\" {
		t.Errorf("expected '\\\\M-\\\\\\\\', got %q", tok.Literal)
	}
}

// --- lexDigit: trailing underscore and zero-underscore forms ---

func TestLexerDigitTrailingUnderscore(t *testing.T) {
	// Integer with trailing underscore includes it in the literal.
	l := New("1_")
	tok := l.NextToken()
	if tok.Type != token.INT {
		t.Fatalf("expected INT, got %s", tok.Type)
	}
	if tok.Literal != "1_" {
		t.Errorf("expected '1_', got %q", tok.Literal)
	}
}

func TestLexerDigitZeroUnderscore(t *testing.T) {
	// 0_1: zero followed by underscore then digit (octal-like but decimal).
	l := New("0_1")
	tok := l.NextToken()
	if tok.Type != token.INT {
		t.Fatalf("expected INT, got %s", tok.Type)
	}
	if tok.Literal != "0_1" {
		t.Errorf("expected '0_1', got %q", tok.Literal)
	}
}

func TestLexerDigitFloatTrailingUnderscore(t *testing.T) {
	// Float with trailing underscore includes it.
	l := New("1.5_")
	tok := l.NextToken()
	if tok.Type != token.FLOAT {
		t.Fatalf("expected FLOAT, got %s", tok.Type)
	}
	if tok.Literal != "1.5_" {
		t.Errorf("expected '1.5_', got %q", tok.Literal)
	}
}

func TestLexerDigitFloatExpNegativeNoDigit(t *testing.T) {
	// 1.5e- : negative exponent sign without digit still enters exponent path
	// because lexFloatFraction has `|| p == '-'`
	l := New("1.5e-")
	tok := l.NextToken()
	if tok.Type != token.FLOAT {
		t.Fatalf("expected FLOAT, got %s (%q)", tok.Type, tok.Literal)
	}
	if tok.Literal != "1.5e-" {
		t.Errorf("expected '1.5e-', got %q", tok.Literal)
	}
}

func TestLexerDigitFloatExpThenNonDigit(t *testing.T) {
	// 1e10x: exponent with digit then non-digit
	l := New("1e10x")
	tok := l.NextToken()
	if tok.Type != token.FLOAT {
		t.Fatalf("expected FLOAT, got %s (%q)", tok.Type, tok.Literal)
	}
	if tok.Literal != "1e10" {
		t.Errorf("expected '1e10', got %q", tok.Literal)
	}
	tok = l.NextToken()
	if tok.Type != token.IDENT || tok.Literal != "x" {
		t.Errorf("expected IDENT x, got %s %q", tok.Type, tok.Literal)
	}
}

// --- lexHeredocBody: squiggy literal with indented delim ---

func TestLexerHeredocBodySquigIndentedDelim(t *testing.T) {
	// <<~'EOS' with indented closing delimiter
	l := New("<<~'EOS'\n  line\n  EOS\n")
	tok := l.NextToken()
	if tok.Type != token.STRING {
		t.Fatalf("expected STRING, got %s", tok.Type)
	}
	if tok.Literal != "line\n" {
		t.Errorf("expected 'line\\n', got %q", tok.Literal)
	}
}

// --- lexHeredocBody: unterminated at delim line without newline ---

func TestLexerHeredocBodyNoTrailingNewline(t *testing.T) {
	l := New("<<'EOS'\nbody\nEOS")
	tok := l.NextToken()
	if tok.Type != token.STRING {
		t.Fatalf("expected STRING, got %s", tok.Type)
	}
	if tok.Literal != "body\n" {
		t.Errorf("expected 'body\\n', got %q", tok.Literal)
	}
}

// --- matchHeredocDelimLine: indented with tabs ---

func TestLexerHeredocMatchIndentedTabs(t *testing.T) {
	l := New("<<-EOS\nline\n\t\tEOS\n")
	l.NextToken()        // STRING_BEG
	tok := l.NextToken() // STRING_CONTENT
	if tok.Literal != "line\n" {
		t.Errorf("expected 'line\\n', got %q", tok.Literal)
	}
	tok = l.NextToken() // STRING_END
	if tok.Type != token.STRING_END {
		t.Fatalf("expected STRING_END, got %s", tok.Type)
	}
}

// --- stripSquigInterpBody: delim not found (no closing delim) ---

func TestLexerStripSquigInterpBodyNoDelim(t *testing.T) {
	// stripSquigInterpBody with no matching delim returns early.
	// Since no delim is found, content is never emitted - just errors.
	l := New("<<~EOS\n  line\n  NOMATCH\n")
	tok := l.NextToken() // STRING_BEG
	if tok.Type != token.STRING_BEG {
		t.Fatalf("expected STRING_BEG, got %s", tok.Type)
	}
	// No closing delim found, so content accumulates and then errors.
	tok = l.NextToken()
	if tok.Type != token.ILLEGAL {
		t.Fatalf("expected ILLEGAL (unterminated), got %s (%q)", tok.Type, tok.Literal)
	}
}

// --- stripSquigInterpBody: min indent from blank lines ---

func TestLexerStripSquigAllBlank(t *testing.T) {
	// All lines blank except last - min indent comes from non-blank
	l := New("<<~EOS\n\n\n  line\n  EOS\n")
	l.NextToken()        // STRING_BEG
	tok := l.NextToken() // STRING_CONTENT
	if tok.Literal != "\n\nline\n" {
		t.Errorf("expected '\\n\\nline\\n', got %q", tok.Literal)
	}
}

// --- lexCharacterLiteral: control char with backslash target ---

func TestLexerCharLiteralControlBackslashTarget(t *testing.T) {
	l := New("?\\C-\\\\")
	tok := l.NextToken()
	if tok.Type != token.STRING {
		t.Fatalf("expected STRING, got %s", tok.Type)
	}
	if tok.Literal != "\\C-\\\\" {
		t.Errorf("expected '\\\\C-\\\\\\\\', got %q", tok.Literal)
	}
}

// --- lexCharacterLiteral: meta char with backslash target ---

func TestLexerCharLiteralMetaBackslashTarget(t *testing.T) {
	l := New("?\\M-\\\\")
	tok := l.NextToken()
	if tok.Type != token.STRING {
		t.Fatalf("expected STRING, got %s", tok.Type)
	}
	if tok.Literal != "\\M-\\\\" {
		t.Errorf("expected '\\\\M-\\\\\\\\', got %q", tok.Literal)
	}
}

// --- startLexer: ** operator (POWER) ---

func TestLexerPowerOperator(t *testing.T) {
	l := New("a ** b")
	l.NextToken() // IDENT a
	tok := l.NextToken()
	if tok.Type != token.POWER {
		t.Fatalf("expected POWER, got %s (%q)", tok.Type, tok.Literal)
	}
	if tok.Literal != "**" {
		t.Errorf("expected '**', got %q", tok.Literal)
	}
}

// --- startLexer: &&= (ANDASSIGN) ---

func TestLexerAndAssign(t *testing.T) {
	l := New("x &&= y")
	l.NextToken() // IDENT x
	tok := l.NextToken()
	if tok.Type != token.ANDASSIGN {
		t.Fatalf("expected ANDASSIGN, got %s (%q)", tok.Type, tok.Literal)
	}
}

// --- startLexer: ||= (ORASSIGN) ---

func TestLexerOrAssign(t *testing.T) {
	l := New("x ||= y")
	l.NextToken() // IDENT x
	tok := l.NextToken()
	if tok.Type != token.ORASSIGN {
		t.Fatalf("expected ORASSIGN, got %s (%q)", tok.Type, tok.Literal)
	}
}

// --- lexDigit: 0.5 (zero-prefix float fraction) ---

func TestLexerZeroDotDigit(t *testing.T) {
	l := New("0.5")
	tok := l.NextToken()
	if tok.Type != token.FLOAT {
		t.Fatalf("expected FLOAT, got %s", tok.Type)
	}
	if tok.Literal != "0.5" {
		t.Errorf("expected '0.5', got %q", tok.Literal)
	}
}

// --- lexDigit: integer exponent with sign ---

func TestLexerIntExpWithSign(t *testing.T) {
	l := New("1e+10")
	tok := l.NextToken()
	if tok.Type != token.FLOAT {
		t.Fatalf("expected FLOAT, got %s", tok.Type)
	}
	if tok.Literal != "1e+10" {
		t.Errorf("expected '1e+10', got %q", tok.Literal)
	}
}

func TestLexerIntExpNegWithSign(t *testing.T) {
	l := New("1e-10")
	tok := l.NextToken()
	if tok.Type != token.FLOAT {
		t.Fatalf("expected FLOAT, got %s", tok.Type)
	}
	if tok.Literal != "1e-10" {
		t.Errorf("expected '1e-10', got %q", tok.Literal)
	}
}

// --- lexCharacterLiteral: space as character (error) ---

func TestLexerCharLiteralSpace(t *testing.T) {
	// ? followed by space emits QMARK (via startLexer), not reaching lexCharacterLiteral.
	// The error branch in lexCharacterLiteral is unreachable through normal lexing
	// because startLexer catches all whitespace before delegating.
	l := New("? ")
	tok := l.NextToken()
	if tok.Type != token.QMARK {
		t.Errorf("expected QMARK, got %s (%q)", tok.Type, tok.Literal)
	}
}

// --- lexCharacterLiteral: unterminated unicode escape ---

func TestLexerCharLiteralUnterminatedUnicode(t *testing.T) {
	l := New("?\\u{41")
	tok := l.NextToken()
	if tok.Type != token.ILLEGAL {
		t.Errorf("expected ILLEGAL for unterminated unicode escape, got %s (%q)", tok.Type, tok.Literal)
	}
}

// --- lexCharacterLiteral: unterminated octal escape ---

func TestLexerCharLiteralUnterminatedOctal(t *testing.T) {
	l := New("?\\o{77")
	tok := l.NextToken()
	if tok.Type != token.ILLEGAL {
		t.Errorf("expected ILLEGAL for unterminated octal escape, got %s (%q)", tok.Type, tok.Literal)
	}
}

// --- lexStringContent: unterminated string ---

func TestLexerUnterminatedString(t *testing.T) {
	l := New("\"no closing quote")
	tok := l.NextToken() // STRING_BEG
	tok = l.NextToken()  // scan until error
	for l.HasNext() && tok.Type != token.ILLEGAL {
		tok = l.NextToken()
	}
	if tok.Type != token.ILLEGAL {
		t.Errorf("expected ILLEGAL for unterminated string, got %s", tok.Type)
	}
}

// --- lexPercentLiteral: unknown type ---

func TestLexerPercentUnknownType(t *testing.T) {
	// %z: 'z' is not a percent-type char, treated as bare % with 'z' as delimiter.
	// The default case in lexPercentLiteral's switch is unreachable because
	// isPercentTypeChar covers all valid types and bare-% maps to typ=0.
	l := New("%z{content}")
	tok := l.NextToken()
	if tok.Type != token.STRING_BEG {
		t.Errorf("expected STRING_BEG (bare %% treated as %%Q), got %s (%q)", tok.Type, tok.Literal)
	}
}

// --- lexPercentLiteralBody: unterminated ---

func TestLexerPercentLiteralUnterminated(t *testing.T) {
	l := New("%q(no closing")
	tok := l.NextToken()
	if tok.Type != token.ILLEGAL {
		t.Errorf("expected ILLEGAL for unterminated percent literal, got %s (%q)", tok.Type, tok.Literal)
	}
}

// --- lexPercentLiteralBodyEnd: unterminated ---

func TestLexerPercentBodyEndUnterminated(t *testing.T) {
	l := New("%w(no closing")
	tok := l.NextToken() // STRING_BEG
	tok = l.NextToken()  // scan until ILLEGAL
	for l.HasNext() && tok.Type != token.ILLEGAL {
		tok = l.NextToken()
	}
	if tok.Type != token.ILLEGAL {
		t.Errorf("expected ILLEGAL, got %s", tok.Type)
	}
}

// --- lexPercentContent: unterminated ---

func TestLexerPercentContentUnterminated(t *testing.T) {
	l := New("%Q{no closing")
	tok := l.NextToken() // STRING_BEG
	tok = l.NextToken()  // scan until ILLEGAL
	for l.HasNext() && tok.Type != token.ILLEGAL {
		tok = l.NextToken()
	}
	if tok.Type != token.ILLEGAL {
		t.Errorf("expected ILLEGAL, got %s", tok.Type)
	}
}

// --- lexBacktickContent: unterminated command literal ---

func TestLexerBacktickUnterminated(t *testing.T) {
	l := New("`no closing backtick")
	tok := l.NextToken() // XSTR_BEG
	tok = l.NextToken()  // scan until ILLEGAL
	for l.HasNext() && tok.Type != token.ILLEGAL {
		tok = l.NextToken()
	}
	if tok.Type != token.ILLEGAL {
		t.Errorf("expected ILLEGAL for unterminated command, got %s", tok.Type)
	}
}

// --- lexHeredocStart: heredoc without trailing newline ---

func TestLexerHeredocNoTrailingNewline(t *testing.T) {
	l := New("<<EOS")
	tok := l.NextToken() // STRING_BEG
	if tok.Type != token.STRING_BEG {
		t.Fatalf("expected STRING_BEG, got %s", tok.Type)
	}
}

// --- lexHeredocBody: empty body with squiggy ---

func TestLexerHeredocBodyEmptySquiggy(t *testing.T) {
	l := New("<<~'EOS'\nEOS\n")
	tok := l.NextToken()
	if tok.Type != token.STRING {
		t.Fatalf("expected STRING, got %s", tok.Type)
	}
	if tok.Literal != "" {
		t.Errorf("expected empty string, got %q", tok.Literal)
	}
}

// --- lexHeredocBody: unterminated at contentEnd ---

func TestLexerHeredocBodyUnterminatedMidBody(t *testing.T) {
	l := New("<<'EOS'\nline\n")
	tok := l.NextToken()
	if tok.Type != token.ILLEGAL {
		t.Errorf("expected ILLEGAL, got %s (%q)", tok.Type, tok.Literal)
	}
}

// --- lexHeredocBody: indented unterminated ---

func TestLexerHeredocBodyIndentedUnterminated(t *testing.T) {
	l := New("<<-'EOS'\nline\n  ")
	tok := l.NextToken()
	if tok.Type != token.ILLEGAL {
		t.Errorf("expected ILLEGAL, got %s (%q)", tok.Type, tok.Literal)
	}
}

// --- lexHeredocBody: eof during delim char matching ---

func TestLexerHeredocBodyEOFDuringDelim(t *testing.T) {
	l := New("<<'EOS'\nline\nE")
	tok := l.NextToken()
	if tok.Type != token.ILLEGAL {
		t.Errorf("expected ILLEGAL, got %s (%q)", tok.Type, tok.Literal)
	}
}

// --- lexHeredocContent: eof during indented check ---

func TestLexerHeredocContentIndentedUnterminated(t *testing.T) {
	l := New("<<-EOS\nline\n  ")
	tok := l.NextToken() // STRING_BEG
	tok = l.NextToken()  // scan
	for l.HasNext() && tok.Type != token.ILLEGAL {
		tok = l.NextToken()
	}
	if tok.Type != token.ILLEGAL {
		t.Errorf("expected ILLEGAL, got %s", tok.Type)
	}
}

// --- lexHeredocContent: eof during delim char matching ---

func TestLexerHeredocContentEOFDuringDelim(t *testing.T) {
	l := New("<<-EOS\nline\nE")
	tok := l.NextToken() // STRING_BEG
	tok = l.NextToken()  // scan
	for l.HasNext() && tok.Type != token.ILLEGAL {
		tok = l.NextToken()
	}
	if tok.Type != token.ILLEGAL {
		t.Errorf("expected ILLEGAL, got %s", tok.Type)
	}
}

// --- lexHeredocContent: matched delim at lineStart (empty content) ---

func TestLexerHeredocContentMatchAtLineStart(t *testing.T) {
	l := New("<<EOS\nEOS\n")
	l.NextToken()        // STRING_BEG
	tok := l.NextToken() // STRING_CONTENT (empty) or STRING_END
	if tok.Type != token.STRING_CONTENT && tok.Type != token.STRING_END {
		t.Fatalf("expected STRING_CONTENT or STRING_END, got %s (%q)", tok.Type, tok.Literal)
	}
}

// --- matchHeredocDelimLine: extra chars after delim ---

func TestLexerMatchHeredocDelimLineExtraChars(t *testing.T) {
	l := New("<<'EOS'\nEOSextra\nEOS\n")
	tok := l.NextToken() // STRING
	if tok.Literal != "EOSextra\n" {
		t.Errorf("expected 'EOSextra\\n', got %q", tok.Literal)
	}
}

// --- stripHeredocIndent: minIndent == 0 returns original ---

func TestLexerStripHeredocIndentZero(t *testing.T) {
	got := stripHeredocIndent("a\n  b\n  c")
	if got != "a\n  b\n  c" {
		t.Errorf("expected no change, got %q", got)
	}
}

// --- lexRegexContent: unterminated regex ---

func TestLexerRegexUnterminated(t *testing.T) {
	l := New("/no closing slash")
	tok := l.NextToken() // REGEX_BEG
	tok = l.NextToken()  // scan until ILLEGAL
	for l.HasNext() && tok.Type != token.ILLEGAL {
		tok = l.NextToken()
	}
	if tok.Type != token.ILLEGAL {
		t.Errorf("expected ILLEGAL for unterminated regex, got %s", tok.Type)
	}
}

// --- consumeEscape: unterminated \u{ returns on eof ---

func TestLexerEscapeUnterminatedUnicode(t *testing.T) {
	// \u{ without closing } consumes everything including the closing "
	// consumeEscape returns via eof branch, then string scanner reports unterminated.
	l := New("\"\\u{41\"")
	tok := l.NextToken() // STRING_BEG
	if tok.Type != token.STRING_BEG {
		t.Fatalf("expected STRING_BEG, got %s", tok.Type)
	}
	tok = l.NextToken() // ILLEGAL (unterminated string)
	if tok.Type != token.ILLEGAL {
		t.Fatalf("expected ILLEGAL, got %s (%q)", tok.Type, tok.Literal)
	}
}

func TestLexerEscapeUnterminatedOctal(t *testing.T) {
	// Same as above for \o{ without closing }
	l := New("\"\\o{77\"")
	tok := l.NextToken() // STRING_BEG
	if tok.Type != token.STRING_BEG {
		t.Fatalf("expected STRING_BEG, got %s", tok.Type)
	}
	tok = l.NextToken() // ILLEGAL (unterminated string)
	if tok.Type != token.ILLEGAL {
		t.Fatalf("expected ILLEGAL, got %s (%q)", tok.Type, tok.Literal)
	}
}

// --- stripSquigInterpBody: eol at end of input (last line no \n) ---

func TestLexerSquigBodyLastLineNoNL(t *testing.T) {
	// Body has a line without trailing \n and no closing delim.
	// stripSquigInterpBody hits eol >= len(l.input) and returns early.
	l := New("<<~EOS\n  line")
	tok := l.NextToken() // STRING_BEG
	if tok.Type != token.STRING_BEG {
		t.Fatalf("expected STRING_BEG, got %s", tok.Type)
	}
	// Body content is "  line" (no newline at end, no matching delim).
	// The lexer keeps scanning and eventually errors.
	tok = l.NextToken()
	if tok.Type != token.ILLEGAL {
		t.Fatalf("expected ILLEGAL, got %s (%q)", tok.Type, tok.Literal)
	}
}

// --- stripSquigInterpBody: all-blank lines before delim (minIndent < 0) ---

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
	// stripSquigInterpBody sets minIndent = 0.
	l := New("<<~EOS\n\n\nEOS\n")
	tok := l.NextToken() // STRING_BEG
	if tok.Type != token.STRING_BEG {
		t.Fatalf("expected STRING_BEG, got %s", tok.Type)
	}
	tok = l.NextToken() // STRING_CONTENT
	if tok.Type != token.STRING_CONTENT {
		t.Fatalf("expected STRING_CONTENT, got %s (%q)", tok.Type, tok.Literal)
	}
	if tok.Literal != "\n\n" {
		t.Errorf("expected '\\n\\n', got %q", tok.Literal)
	}
}
