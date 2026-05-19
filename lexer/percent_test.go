package lexer

import (
	"testing"

	"github.com/lczyk/goruby/token"
)

func TestLexerPercentLiteralAllDelimiters(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []struct {
			typ     token.Type
			literal string
		}
	}{
		// --- %q (non-interpolating string) with various delimiters ---
		{
			name:  "%q with parens",
			input: "%q(inner)",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING, "inner"},
			},
		},
		{
			name:  "%q with brackets",
			input: "%q[inner]",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING, "inner"},
			},
		},
		{
			name:  "%q with angle brackets",
			input: "%q<inner>",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING, "inner"},
			},
		},
		{
			name:  "%q with slash (non-paired)",
			input: "%q/inner/",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING, "inner"},
			},
		},
		{
			name:  "%q with pipe (non-paired)",
			input: "%q|inner|",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING, "inner"},
			},
		},
		{
			name:  "%q with bang (non-paired)",
			input: "%q!inner!",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING, "inner"},
			},
		},
		{
			name:  "%q nested paired delimiters",
			input: "%q(outer (inner) more)",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING, "outer (inner) more"},
			},
		},
		{
			name:  "%q empty body",
			input: "%q{}",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING, ""},
			},
		},

		// --- %w (word array) with various delimiters ---
		{
			name:  "%w with parens",
			input: "%w(foo bar)",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, "w"},
				{token.STRING_CONTENT, "foo bar"},
				{token.STRING_END, ""},
			},
		},
		{
			name:  "%w with brackets",
			input: "%w[foo bar]",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, "w"},
				{token.STRING_CONTENT, "foo bar"},
				{token.STRING_END, ""},
			},
		},
		{
			name:  "%w with angle brackets",
			input: "%w<foo bar>",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, "w"},
				{token.STRING_CONTENT, "foo bar"},
				{token.STRING_END, ""},
			},
		},
		{
			name:  "%w with slash (non-paired)",
			input: "%w/foo bar/",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, "w"},
				{token.STRING_CONTENT, "foo bar"},
				{token.STRING_END, ""},
			},
		},
		{
			name:  "%w empty body",
			input: "%w()",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, "w"},
				{token.STRING_END, ""},
			},
		},
		{
			name:  "%w nested paired delimiters",
			input: "%w(foo (bar) baz)",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, "w"},
				{token.STRING_CONTENT, "foo (bar) baz"},
				{token.STRING_END, ""},
			},
		},

		// --- %i (symbol array, non-interpolating) ---
		{
			name:  "%i with braces",
			input: "%i{foo bar baz}",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, "i"},
				{token.STRING_CONTENT, "foo bar baz"},
				{token.STRING_END, ""},
			},
		},
		{
			name:  "%i with parens",
			input: "%i(foo bar)",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, "i"},
				{token.STRING_CONTENT, "foo bar"},
				{token.STRING_END, ""},
			},
		},
		{
			name:  "%i with slash (non-paired)",
			input: "%i/foo bar/",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, "i"},
				{token.STRING_CONTENT, "foo bar"},
				{token.STRING_END, ""},
			},
		},
		{
			name:  "%i empty body",
			input: "%i[]",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, "i"},
				{token.STRING_END, ""},
			},
		},

		// --- %s (symbol literal, non-interpolating) ---
		{
			name:  "%s with braces",
			input: "%s{sym}",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, "s"},
				{token.STRING_CONTENT, "sym"},
				{token.STRING_END, ""},
			},
		},
		{
			name:  "%s with parens",
			input: "%s(sym)",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, "s"},
				{token.STRING_CONTENT, "sym"},
				{token.STRING_END, ""},
			},
		},
		{
			name:  "%s with slash (non-paired)",
			input: "%s/sym/",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, "s"},
				{token.STRING_CONTENT, "sym"},
				{token.STRING_END, ""},
			},
		},
		{
			name:  "%s empty body",
			input: "%s{}",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, "s"},
				{token.STRING_END, ""},
			},
		},

		// --- %Q (interpolating string) with various delimiters ---
		{
			name:  "%Q with parens",
			input: "%Q(hello #{name})",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, "Q"},
				{token.STRING_CONTENT, "hello "},
				{token.EMBEXPR_BEG, "#{"},
				{token.IDENT, "name"},
				{token.EMBEXPR_END, "}"},
				{token.STRING_END, ""},
			},
		},
		{
			name:  "%Q with brackets",
			input: "%Q[hello]",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, "Q"},
				{token.STRING_CONTENT, "hello"},
				{token.STRING_END, ""},
			},
		},
		{
			name:  "%Q with slash (non-paired)",
			input: "%Q/hello/",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, "Q"},
				{token.STRING_CONTENT, "hello"},
				{token.STRING_END, ""},
			},
		},
		{
			name:  "%Q with pipe (non-paired)",
			input: "%Q|hello|",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, "Q"},
				{token.STRING_CONTENT, "hello"},
				{token.STRING_END, ""},
			},
		},
		{
			name:  "%Q nested paired delimiters",
			input: "%Q(outer (inner) more)",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, "Q"},
				{token.STRING_CONTENT, "outer (inner) more"},
				{token.STRING_END, ""},
			},
		},
		{
			name:  "%Q with #$var interp",
			input: "%Q{hello #$foo}",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, "Q"},
				{token.STRING_CONTENT, "hello "},
				{token.GLOBAL, "$foo"},
				{token.STRING_END, ""},
			},
		},
		{
			name:  "%Q with #@var interp",
			input: "%Q{hello #@foo}",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, "Q"},
				{token.STRING_CONTENT, "hello "},
				{token.AT, "@"},
				{token.IDENT, "foo"},
				{token.STRING_END, ""},
			},
		},

		// --- %W (interpolating word array) with various delimiters ---
		{
			name:  "%W with parens",
			input: "%W(foo #{bar})",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, "W"},
				{token.STRING_CONTENT, "foo "},
				{token.EMBEXPR_BEG, "#{"},
				{token.IDENT, "bar"},
				{token.EMBEXPR_END, "}"},
				{token.STRING_END, ""},
			},
		},
		{
			name:  "%W with slash (non-paired)",
			input: "%W/foo bar/",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, "W"},
				{token.STRING_CONTENT, "foo bar"},
				{token.STRING_END, ""},
			},
		},
		{
			name:  "%W empty body with brackets",
			input: "%W[]",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, "W"},
				{token.STRING_END, ""},
			},
		},
		{
			name:  "%W with #$var interp",
			input: "%W{foo #$bar}",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, "W"},
				{token.STRING_CONTENT, "foo "},
				{token.GLOBAL, "$bar"},
				{token.STRING_END, ""},
			},
		},
		{
			name:  "%W with #@var interp",
			input: "%W{foo #@bar}",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, "W"},
				{token.STRING_CONTENT, "foo "},
				{token.AT, "@"},
				{token.IDENT, "bar"},
				{token.STRING_END, ""},
			},
		},

		// --- %I (interpolating symbol array) ---
		{
			name:  "%I with braces",
			input: "%I{foo #{bar}}",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, "I"},
				{token.STRING_CONTENT, "foo "},
				{token.EMBEXPR_BEG, "#{"},
				{token.IDENT, "bar"},
				{token.EMBEXPR_END, "}"},
				{token.STRING_END, ""},
			},
		},
		{
			name:  "%I with parens",
			input: "%I(foo bar)",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, "I"},
				{token.STRING_CONTENT, "foo bar"},
				{token.STRING_END, ""},
			},
		},
		{
			name:  "%I with slash (non-paired)",
			input: "%I/foo bar/",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, "I"},
				{token.STRING_CONTENT, "foo bar"},
				{token.STRING_END, ""},
			},
		},

		// --- %r (regex) with various delimiters ---
		{
			name:  "%r with parens",
			input: "%r(pattern)",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.REGEX_BEG, "r"},
				{token.STRING_CONTENT, "pattern"},
				{token.REGEX_END, ""},
			},
		},
		{
			name:  "%r with brackets",
			input: "%r[pattern]",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.REGEX_BEG, "r"},
				{token.STRING_CONTENT, "pattern"},
				{token.REGEX_END, ""},
			},
		},
		{
			name:  "%r with slash (non-paired)",
			input: "%r/pattern/",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.REGEX_BEG, "r"},
				{token.STRING_CONTENT, "pattern"},
				{token.REGEX_END, ""},
			},
		},
		{
			name:  "%r with pipe (non-paired)",
			input: "%r|pattern|",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.REGEX_BEG, "r"},
				{token.STRING_CONTENT, "pattern"},
				{token.REGEX_END, ""},
			},
		},
		{
			name:  "%r with #$var interp",
			input: "%r{foo #$bar}",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.REGEX_BEG, "r"},
				{token.STRING_CONTENT, "foo "},
				{token.GLOBAL, "$bar"},
				{token.REGEX_END, ""},
			},
		},

		// --- %x (backtick) with various delimiters ---
		{
			name:  "%x with parens",
			input: "%x(ls -la)",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.XSTR_BEG, "x"},
				{token.XSTR_CONTENT, "ls -la"},
				{token.XSTR_END, ""},
			},
		},
		{
			name:  "%x with brackets",
			input: "%x[ls -la]",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.XSTR_BEG, "x"},
				{token.XSTR_CONTENT, "ls -la"},
				{token.XSTR_END, ""},
			},
		},
		{
			name:  "%x with slash (non-paired)",
			input: "%x/ls -la/",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.XSTR_BEG, "x"},
				{token.XSTR_CONTENT, "ls -la"},
				{token.XSTR_END, ""},
			},
		},
		{
			name:  "%x with pipe (non-paired)",
			input: "%x|ls -la|",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.XSTR_BEG, "x"},
				{token.XSTR_CONTENT, "ls -la"},
				{token.XSTR_END, ""},
			},
		},
		{
			name:  "%x with #$var interp",
			input: "%x{ls #$dir}",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.XSTR_BEG, "x"},
				{token.XSTR_CONTENT, "ls "},
				{token.GLOBAL, "$dir"},
				{token.XSTR_END, ""},
			},
		},

		// --- Bare % (like %Q) with various delimiters ---
		{
			name:  "bare % with parens",
			input: "%(hello)",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, "Q"},
				{token.STRING_CONTENT, "hello"},
				{token.STRING_END, ""},
			},
		},
		{
			name:  "bare % with brackets",
			input: "%[hello]",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, "Q"},
				{token.STRING_CONTENT, "hello"},
				{token.STRING_END, ""},
			},
		},
		{
			name:  "bare % with slash (non-paired)",
			input: "%/hello/",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, "Q"},
				{token.STRING_CONTENT, "hello"},
				{token.STRING_END, ""},
			},
		},
		{
			name:  "bare % nested paired",
			input: "%(outer (inner) end)",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, "Q"},
				{token.STRING_CONTENT, "outer (inner) end"},
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

func TestLexerPercentAsMethodArg(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []struct {
			typ     token.Type
			literal string
		}
	}{
		{
			name:  "%w after space-separated identifier",
			input: "foo %w[a b]",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.IDENT, "foo"},
				{token.STRING_BEG, "w"},
				{token.STRING_CONTENT, "a b"},
				{token.STRING_END, ""},
			},
		},
		{
			name:  "%q after space-separated identifier",
			input: "foo %q[text]",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.IDENT, "foo"},
				{token.STRING, "text"},
			},
		},
		{
			name:  "%Q after space-separated CONST",
			input: "Foo %Q{text}",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.CONST, "Foo"},
				{token.STRING_BEG, "Q"},
				{token.STRING_CONTENT, "text"},
				{token.STRING_END, ""},
			},
		},
		{
			name:  "% without whitespace is modulo",
			input: "x%2",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.IDENT, "x"},
				{token.MODULO, "%"},
				{token.INT, "2"},
			},
		},
		{
			name:  "% after rparen via method chaining",
			input: "obj.method %w[a b]",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.IDENT, "obj"},
				{token.DOT, "."},
				{token.IDENT, "method"},
				{token.STRING_BEG, "w"},
				{token.STRING_CONTENT, "a b"},
				{token.STRING_END, ""},
			},
		},
		{
			name:  "% after method call with parens",
			input: "foo() %w[a b]",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.IDENT, "foo"},
				{token.LPAREN, "("},
				{token.RPAREN, ")"},
				{token.STRING_BEG, "w"},
				{token.STRING_CONTENT, "a b"},
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

func TestLexerEscapedCharInPercentLiteral(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []struct {
			typ     token.Type
			literal string
		}
	}{
		{
			name:  "escaped closing delim in %q",
			input: "%q(hello \\) there)",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING, "hello \\) there"},
			},
		},
		{
			// MRI strips the escape from `\<delim>` in %w/%i/%s bodies
			// (the `\)` here becomes a literal `)`), matching Ruby's
			// non-interpolating percent-literal escape semantics.
			name:  "escaped closer in non-interpolating %w",
			input: "%w(foo \\) bar)",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, "w"},
				{token.STRING_CONTENT, "foo ) bar"},
				{token.STRING_END, ""},
			},
		},
		{
			name:  "escaped closer in non-paired %q",
			input: "%q/foo \\/ bar/",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING, "foo \\/ bar"},
			},
		},
		{
			name:  "escaped closer in non-paired %w",
			input: "%w/foo \\/ bar/",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, "w"},
				{token.STRING_CONTENT, "foo / bar"},
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
