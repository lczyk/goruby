package lexer

import (
	"testing"

	"github.com/lczyk/goruby/token"
)

func TestLexerNextToken(t *testing.T) {
	input := `five = 5
while x < y do
	x += x
end
seven,
# just comment
fifty = 5_0
ten = 10
?-
?\n
? foo : bar

def add(x, y)
	x + y
end
|
||

result = add(five, ten)
!-/*%5;
+= -= *= /= %=
5 < 10 > 5
return
if 5 < 10 then
	true
else
	false
end

begin
rescue
end

10 == 10
10 != 9
10 <= 9
10 >= 9
10 <=> 9
10 << 9
	10 >> 9
	5 === 5
	x <<= 2
	x >>= 2
	x &= 2
	x |= 2
	x ^= 2
""
"foobar"
'foobar'
"foo bar"
'foo bar'
:sym
:"sym"
:'sym'
.
&foo
&
&&
:dotAfter.

def nil?
end

def run!
end
[1, 2]
nil
self
Ten = 10
module Abc
end
class Abc
end
add { |x| x }
add do |x|
end
yield
while
A::B
=>
__FILE__
@
$foo,
$foo;
$Foo
$dotAfter.
$@
$a
	@@foo
`

	tests := []struct {
		expectedType    token.Type
		expectedLiteral string
	}{
		{token.IDENT, "five"},
		{token.ASSIGN, "="},
		{token.INT, "5"},
		{token.NEWLINE, "\n"},
		{token.WHILE, "while"},
		{token.IDENT, "x"},
		{token.LT, "<"},
		{token.IDENT, "y"},
		{token.DO, "do"},
		{token.NEWLINE, "\n"},
		{token.IDENT, "x"},
		{token.ADDASSIGN, "+="},
		{token.IDENT, "x"},
		{token.NEWLINE, "\n"},
		{token.END, "end"},
		{token.NEWLINE, "\n"},
		{token.IDENT, "seven"},
		{token.COMMA, ","},
		{token.NEWLINE, "\n"},
		{token.HASH, "#"},
		{token.STRING, " just comment"},
		{token.NEWLINE, "\n"},
		{token.IDENT, "fifty"},
		{token.ASSIGN, "="},
		{token.INT, "5_0"},
		{token.NEWLINE, "\n"},
		{token.IDENT, "ten"},
		{token.ASSIGN, "="},
		{token.INT, "10"},
		{token.NEWLINE, "\n"},
		{token.STRING, "-"},
		{token.NEWLINE, "\n"},
		{token.STRING, "\\n"},
		{token.NEWLINE, "\n"},
		{token.QMARK, "?"},
		{token.IDENT, "foo"},
		{token.COLON, ":"},
		{token.IDENT, "bar"},
		{token.NEWLINE, "\n"},
		{token.NEWLINE, "\n"},
		{token.DEF, "def"},
		{token.IDENT, "add"},
		{token.LPAREN, "("},
		{token.IDENT, "x"},
		{token.COMMA, ","},
		{token.IDENT, "y"},
		{token.RPAREN, ")"},
		{token.NEWLINE, "\n"},
		{token.IDENT, "x"},
		{token.PLUS, "+"},
		{token.IDENT, "y"},
		{token.NEWLINE, "\n"},
		{token.END, "end"},
		{token.NEWLINE, "\n"},
		{token.PIPE, "|"},
		{token.NEWLINE, "\n"},
		{token.LOGICALOR, "||"},
		{token.NEWLINE, "\n"},
		{token.NEWLINE, "\n"},
		{token.IDENT, "result"},
		{token.ASSIGN, "="},
		{token.IDENT, "add"},
		{token.LPAREN, "("},
		{token.IDENT, "five"},
		{token.COMMA, ","},
		{token.IDENT, "ten"},
		{token.RPAREN, ")"},
		{token.NEWLINE, "\n"},
		{token.BANG, "!"},
		{token.MINUS, "-"},
		{token.SLASH, "/"},
		{token.ASTERISK, "*"},
		{token.MODULO, "%"},
		{token.INT, "5"},
		{token.SEMICOLON, ";"},
		{token.NEWLINE, "\n"},
		{token.ADDASSIGN, "+="},
		{token.SUBASSIGN, "-="},
		{token.MULASSIGN, "*="},
		{token.DIVASSIGN, "/="},
		{token.MODASSIGN, "%="},
		{token.NEWLINE, "\n"},
		{token.INT, "5"},
		{token.LT, "<"},
		{token.INT, "10"},
		{token.GT, ">"},
		{token.INT, "5"},
		{token.NEWLINE, "\n"},
		{token.RETURN, "return"},
		{token.NEWLINE, "\n"},
		{token.IF, "if"},
		{token.INT, "5"},
		{token.LT, "<"},
		{token.INT, "10"},
		{token.THEN, "then"},
		{token.NEWLINE, "\n"},
		{token.TRUE, "true"},
		{token.NEWLINE, "\n"},
		{token.ELSE, "else"},
		{token.NEWLINE, "\n"},
		{token.FALSE, "false"},
		{token.NEWLINE, "\n"},
		{token.END, "end"},
		{token.NEWLINE, "\n"},
		{token.NEWLINE, "\n"},
		{token.BEGIN, "begin"},
		{token.NEWLINE, "\n"},
		{token.RESCUE, "rescue"},
		{token.NEWLINE, "\n"},
		{token.END, "end"},
		{token.NEWLINE, "\n"},
		{token.NEWLINE, "\n"},
		{token.INT, "10"},
		{token.EQ, "=="},
		{token.INT, "10"},
		{token.NEWLINE, "\n"},
		{token.INT, "10"},
		{token.NOTEQ, "!="},
		{token.INT, "9"},
		{token.NEWLINE, "\n"},
		{token.INT, "10"},
		{token.LTE, "<="},
		{token.INT, "9"},
		{token.NEWLINE, "\n"},
		{token.INT, "10"},
		{token.GTE, ">="},
		{token.INT, "9"},
		{token.NEWLINE, "\n"},
		{token.INT, "10"},
		{token.SPACESHIP, "<=>"},
		{token.INT, "9"},
		{token.NEWLINE, "\n"},
		{token.INT, "10"},
		{token.LSHIFT, "<<"},
		{token.INT, "9"},
		{token.NEWLINE, "\n"},
		{token.INT, "10"},
		{token.RSHIFT, ">>"},
		{token.INT, "9"},
		{token.NEWLINE, "\n"},
		{token.INT, "5"},
		{token.CASEEQ, "==="},
		{token.INT, "5"},
		{token.NEWLINE, "\n"},
		{token.IDENT, "x"},
		{token.LSHIFTASSIGN, "<<="},
		{token.INT, "2"},
		{token.NEWLINE, "\n"},
		{token.IDENT, "x"},
		{token.RSHIFTASSIGN, ">>="},
		{token.INT, "2"},
		{token.NEWLINE, "\n"},
		{token.IDENT, "x"},
		{token.ANDASSIGN_BITWISE, "&="},
		{token.INT, "2"},
		{token.NEWLINE, "\n"},
		{token.IDENT, "x"},
		{token.ORASSIGN_BITWISE, "|="},
		{token.INT, "2"},
		{token.NEWLINE, "\n"},
		{token.IDENT, "x"},
		{token.XORASSIGN, "^="},
		{token.INT, "2"},
		{token.NEWLINE, "\n"},
		{token.STRING_BEG, ""},
		{token.STRING_END, "\""},
		{token.NEWLINE, "\n"},
		{token.STRING_BEG, ""},
		{token.STRING_CONTENT, "foobar"},
		{token.STRING_END, "\""},
		{token.NEWLINE, "\n"},
		{token.STRING, "foobar"},
		{token.NEWLINE, "\n"},
		{token.STRING_BEG, ""},
		{token.STRING_CONTENT, "foo bar"},
		{token.STRING_END, "\""},
		{token.NEWLINE, "\n"},
		{token.STRING, "foo bar"},
		{token.NEWLINE, "\n"},
		{token.SYMBEG, ":"},
		{token.IDENT, "sym"},
		{token.NEWLINE, "\n"},
		{token.SYMBEG, ":"},
		{token.STRING_BEG, ""},
		{token.STRING_CONTENT, "sym"},
		{token.STRING_END, "\""},
		{token.NEWLINE, "\n"},
		{token.SYMBEG, ":"},
		{token.STRING, "sym"},
		{token.NEWLINE, "\n"},
		{token.DOT, "."},
		{token.NEWLINE, "\n"},
		{token.CAPTURE, "&"},
		{token.IDENT, "foo"},
		{token.NEWLINE, "\n"},
		{token.AND, "&"},
		{token.NEWLINE, "\n"},
		{token.LOGICALAND, "&&"},
		{token.NEWLINE, "\n"},
		{token.SYMBEG, ":"},
		{token.IDENT, "dotAfter"},
		{token.DOT, "."},
		{token.NEWLINE, "\n"},
		{token.NEWLINE, "\n"},
		{token.DEF, "def"},
		{token.IDENT, "nil?"},
		{token.NEWLINE, "\n"},
		{token.END, "end"},
		{token.NEWLINE, "\n"},
		{token.NEWLINE, "\n"},
		{token.DEF, "def"},
		{token.IDENT, "run!"},
		{token.NEWLINE, "\n"},
		{token.END, "end"},
		{token.NEWLINE, "\n"},
		{token.LBRACKET, "["},
		{token.INT, "1"},
		{token.COMMA, ","},
		{token.INT, "2"},
		{token.RBRACKET, "]"},
		{token.NEWLINE, "\n"},
		{token.NIL, "nil"},
		{token.NEWLINE, "\n"},
		{token.SELF, "self"},
		{token.NEWLINE, "\n"},
		{token.CONST, "Ten"},
		{token.ASSIGN, "="},
		{token.INT, "10"},
		{token.NEWLINE, "\n"},
		{token.MODULE, "module"},
		{token.CONST, "Abc"},
		{token.NEWLINE, "\n"},
		{token.END, "end"},
		{token.NEWLINE, "\n"},
		{token.CLASS, "class"},
		{token.CONST, "Abc"},
		{token.NEWLINE, "\n"},
		{token.END, "end"},
		{token.NEWLINE, "\n"},
		{token.IDENT, "add"},
		{token.LBRACE, "{"},
		{token.PIPE, "|"},
		{token.IDENT, "x"},
		{token.PIPE, "|"},
		{token.IDENT, "x"},
		{token.RBRACE, "}"},
		{token.NEWLINE, "\n"},
		{token.IDENT, "add"},
		{token.DO, "do"},
		{token.PIPE, "|"},
		{token.IDENT, "x"},
		{token.PIPE, "|"},
		{token.NEWLINE, "\n"},
		{token.END, "end"},
		{token.NEWLINE, "\n"},
		{token.YIELD, "yield"},
		{token.NEWLINE, "\n"},
		{token.WHILE, "while"},
		{token.NEWLINE, "\n"},
		{token.CONST, "A"},
		{token.SCOPE, "::"},
		{token.CONST, "B"},
		{token.NEWLINE, "\n"},
		{token.HASHROCKET, "=>"},
		{token.NEWLINE, "\n"},
		{token.KEYWORD__FILE__, "__FILE__"},
		{token.NEWLINE, "\n"},
		{token.AT, "@"},
		{token.NEWLINE, "\n"},
		{token.GLOBAL, "$foo"},
		{token.COMMA, ","},
		{token.NEWLINE, "\n"},
		{token.GLOBAL, "$foo"},
		{token.SEMICOLON, ";"},
		{token.NEWLINE, "\n"},
		{token.GLOBAL, "$Foo"},
		{token.NEWLINE, "\n"},
		{token.GLOBAL, "$dotAfter"},
		{token.DOT, "."},
		{token.NEWLINE, "\n"},
		{token.GLOBAL, "$@"},
		{token.NEWLINE, "\n"},
		{token.GLOBAL, "$a"},
		{token.NEWLINE, "\n"},
		{token.CLASS_VAR, "@@"},
		{token.IDENT, "foo"},
		{token.NEWLINE, "\n"},
		{token.EOF, ""},
	}

	lexer := New(input)

	for pos, testCase := range tests {
		if !lexer.HasNext() {
			t.Logf("Unexpected EOF at %d\n", lexer.pos)
			t.FailNow()
		}
		token := lexer.NextToken()

		if token.Type != testCase.expectedType {
			t.Logf("Expected token with type %q at position %d, got type %q\n", testCase.expectedType, pos, token.Type)
			t.Fail()
		}

		if token.Literal != testCase.expectedLiteral {
			t.Logf("Expected token with literal %q at position %d, got literal %q\n", testCase.expectedLiteral, pos, token.Literal)
			t.Fail()
		}
	}
}

func TestLexerHeredoc(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []struct {
			typ     token.Type
			literal string
		}
	}{
		{
			name:  "mid-line chaining with .chop",
			input: "x = <<EOS.chop\ncontent\nEOS\n",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.IDENT, "x"},
				{token.ASSIGN, "="},
				{token.STRING_BEG, ""},
				{token.STRING_CONTENT, "content\n"},
				{token.STRING_END, ""},
				{token.DOT, "."},
				{token.IDENT, "chop"},
				{token.EOF, ""},
			},
		},
		{
			name:  "squiggy heredoc strips common indent",
			input: "<<~'EOS'\n  hello\n  world\nEOS\n",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING, "hello\nworld\n"},
				{token.EOF, ""},
			},
		},
		{
			name:  "indented heredoc <<-",
			input: "<<-EOS\n\tcontent\nEOS\n",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, ""},
				{token.STRING_CONTENT, "\tcontent\n"},
				{token.STRING_END, ""},
				{token.EOF, ""},
			},
		},
		{
			name:  "mid-line chaining with method call",
			input: "foo(<<EOS.strip)\ncontent\nEOS\n",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.IDENT, "foo"},
				{token.LPAREN, "("},
				{token.STRING_BEG, ""},
				{token.STRING_CONTENT, "content\n"},
				{token.STRING_END, ""},
				{token.DOT, "."},
				{token.IDENT, "strip"},
				{token.RPAREN, ")"},
				{token.EOF, ""},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := New(tt.input)
			for i, exp := range tt.expected {
				if !l.HasNext() {
					t.Fatalf("pos %d: unexpected EOF", i)
				}
				tok := l.NextToken()
				if tok.Type != exp.typ {
					t.Errorf("pos %d: expected type %s, got %s", i, exp.typ, tok.Type)
				}
				if tok.Literal != exp.literal {
					t.Errorf("pos %d: expected literal %q, got %q", i, exp.literal, tok.Literal)
				}
			}
		})
	}
}

func TestLexerStringInterpolation(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []struct {
			typ     token.Type
			literal string
		}
	}{
		{
			name:  "simple interpolation #{name}",
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
			name:  "variable interpolation #$foo",
			input: "\"hello #$foo world\"",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, ""},
				{token.STRING_CONTENT, "hello "},
				{token.GLOBAL, "$foo"},
				{token.STRING_CONTENT, " world"},
				{token.STRING_END, "\""},
			},
		},
		{
			name:  "variable interpolation #@foo",
			input: "\"hello #@foo world\"",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, ""},
				{token.STRING_CONTENT, "hello "},
				{token.AT, "@"},
				{token.IDENT, "foo"},
				{token.STRING_CONTENT, " world"},
				{token.STRING_END, "\""},
			},
		},
		{
			name:  "nested braces in interpolation",
			input: "\"x=#{ {a: 1} }\"",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, ""},
				{token.STRING_CONTENT, "x="},
				{token.EMBEXPR_BEG, "#{"},
				{token.LBRACE, "{"},
				{token.IDENT, "a"},
				{token.COLON, ":"},
				{token.INT, "1"},
				{token.RBRACE, "}"},
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

func TestLexerHeredocInterpolation(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []struct {
			typ     token.Type
			literal string
		}
	}{
		{
			name:  "heredoc with simple interpolation",
			input: "<<EOS\nhello #{name} world\nEOS\n",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, ""},
				{token.STRING_CONTENT, "hello "},
				{token.EMBEXPR_BEG, "#{"},
				{token.IDENT, "name"},
				{token.EMBEXPR_END, "}"},
				{token.STRING_CONTENT, " world\n"},
				{token.STRING_END, ""},
			},
		},
		{
			name:  "heredoc with variable interpolation #$foo",
			input: "<<EOS\nhello #$foo world\nEOS\n",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, ""},
				{token.STRING_CONTENT, "hello "},
				{token.GLOBAL, "$foo"},
				{token.STRING_CONTENT, " world\n"},
				{token.STRING_END, ""},
			},
		},
		{
			name:  "backtick heredoc",
			input: "<<`CMD`\nls -la\nCMD\n",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.XSTR_BEG, ""},
				{token.XSTR_CONTENT, "ls -la\n"},
				{token.XSTR_END, ""},
			},
		},
		{
			name:  "backtick heredoc with interpolation",
			input: "<<`CMD`\nls #{path}\nCMD\n",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.XSTR_BEG, ""},
				{token.XSTR_CONTENT, "ls "},
				{token.EMBEXPR_BEG, "#{"},
				{token.IDENT, "path"},
				{token.EMBEXPR_END, "}"},
				{token.XSTR_CONTENT, "\n"},
				{token.XSTR_END, ""},
			},
		},
		{
			name:  "squiggy heredoc with interpolation",
			input: "<<~EOS\n  hello #{name}\n  world\n  EOS\n",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, ""},
				{token.STRING_CONTENT, "hello "},
				{token.EMBEXPR_BEG, "#{"},
				{token.IDENT, "name"},
				{token.EMBEXPR_END, "}"},
				{token.STRING_CONTENT, "\nworld\n"},
				{token.STRING_END, ""},
			},
		},
		{
			name:  "squiggy heredoc with mixed indent",
			input: "<<~EOS\n    a\n  b\n    #{x}\n  EOS\n",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, ""},
				{token.STRING_CONTENT, "  a\nb\n  "},
				{token.EMBEXPR_BEG, "#{"},
				{token.IDENT, "x"},
				{token.EMBEXPR_END, "}"},
				{token.STRING_CONTENT, "\n"},
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

func TestLexerRegex(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []struct {
			typ     token.Type
			literal string
		}
	}{
		{
			name:  "simple regex",
			input: "/foo/",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.REGEX_BEG, ""},
				{token.STRING_CONTENT, "foo"},
				{token.REGEX_END, ""},
			},
		},
		{
			name:  "regex with flags",
			input: "/foo/im",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.REGEX_BEG, ""},
				{token.STRING_CONTENT, "foo"},
				{token.REGEX_END, "im"},
			},
		},
		{
			name:  "regex with interpolation",
			input: "/foo #{x}/",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.REGEX_BEG, ""},
				{token.STRING_CONTENT, "foo "},
				{token.EMBEXPR_BEG, "#{"},
				{token.IDENT, "x"},
				{token.EMBEXPR_END, "}"},
				{token.REGEX_END, ""},
			},
		},
		{
			name:  "regex with escaped slash",
			input: "/foo\\/bar/",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.REGEX_BEG, ""},
				{token.STRING_CONTENT, "foo\\/bar"},
				{token.REGEX_END, ""},
			},
		},
		{
			name:  "empty regex",
			input: "//",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.REGEX_BEG, ""},
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

func TestLexerPercentLiteral(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []struct {
			typ     token.Type
			literal string
		}
	}{
		{
			name:  "%q literal string",
			input: "%q{hello world}",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING, "hello world"},
			},
		},
		{
			name:  "%Q interpolating string",
			input: "%Q{hello #{name}}",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, ""},
				{token.STRING_CONTENT, "hello "},
				{token.EMBEXPR_BEG, "#{"},
				{token.IDENT, "name"},
				{token.EMBEXPR_END, "}"},
				{token.STRING_END, ""},
			},
		},
		{
			name:  "bare % interpolating string",
			input: "%{hello}",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, ""},
				{token.STRING_CONTENT, "hello"},
				{token.STRING_END, ""},
			},
		},
		{
			name:  "%r regex literal",
			input: "%r{pattern}",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.REGEX_BEG, ""},
				{token.STRING_CONTENT, "pattern"},
				{token.REGEX_END, ""},
			},
		},
		{
			name:  "%x backtick literal",
			input: "%x{ls -la}",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.XSTR_BEG, ""},
				{token.XSTR_CONTENT, "ls -la"},
				{token.XSTR_END, ""},
			},
		},
		{
			name:  "%w literal word array",
			input: "%w{foo bar baz}",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING, "foo bar baz"},
			},
		},
		{
			name:  "%W interpolating word array",
			input: "%W{foo #{bar}}",
			expected: []struct {
				typ     token.Type
				literal string
			}{
				{token.STRING_BEG, ""},
				{token.STRING_CONTENT, "foo "},
				{token.EMBEXPR_BEG, "#{"},
				{token.IDENT, "bar"},
				{token.EMBEXPR_END, "}"},
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
