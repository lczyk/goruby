package lexer

import (
	"os"
	"strings"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/goruby/token"
)

// raise "foo" unless x < 10
type expected struct {
	typ token.Type
	lit string
}

func expect(t *testing.T) func(tk string, literal string) expected {
	return func(tk string, literal string) expected {
		typ := token.ToType(tk)
		if typ == token.ILLEGAL {
			t.Fatalf("expect: %q is not a valid token type", tk)
		}
		return expected{
			typ: typ,
			lit: literal,
		}
	}
}

var NL = expect(nil)("NEWLINE", "\n")

func preprocessLines(lines string) string {
	slines := strings.Split(lines, "\n")
	new_slines := make([]string, 0)
	for _, line := range slines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		new_slines = append(new_slines, line)
	}
	return strings.Join(new_slines, "\n")
}

func allTokens(lexer *Lexer) []token.Token {
	tokens := []token.Token{}
	for {
		tk := lexer.NextToken()
		if tk.Type == token.EOF {
			break
		}
		tokens = append(tokens, tk)
	}
	return tokens
}

type test struct {
	desc  string
	lines string
	exp   []expected
}

func TestLex(t *testing.T) {
	tests := []test{
		{
			desc: "numbers",
			lines: `
				5
				5_0
				1.0
				123.456
				123.to_s
			`,
			exp: []expected{
				expect(t)("INT", "5"),
				NL,
				expect(t)("INT", "5_0"),
				NL,
				expect(t)("FLOAT", "1.0"),
				NL,
				expect(t)("FLOAT", "123.456"),
				NL,
				expect(t)("INT", "123"),
				expect(t)("DOT", "."),
				expect(t)("IDENT", "to_s"),
			},
		},
		{
			desc: "assignments",
			lines: `
				five = 5
				Ten = 10
			`,
			exp: []expected{
				expect(t)("IDENT", "five"),
				expect(t)("ASSIGN", "="),
				expect(t)("INT", "5"),
				NL,
				expect(t)("IDENT", "Ten"),
				expect(t)("ASSIGN", "="),
				expect(t)("INT", "10"),
			},
		},
		{
			desc: "strings",
			lines: `
				""
				"foobar"
				'foobar'
				"foo bar"
				'foo bar'
				"\\"
				/\//
				/a/
			`,
			exp: []expected{
				expect(t)("STRING", ""),
				NL,
				expect(t)("STRING", "foobar"),
				NL,
				expect(t)("STRING", "foobar"),
				NL,
				expect(t)("STRING", "foo bar"),
				NL,
				expect(t)("STRING", "foo bar"),
				NL,
				expect(t)("STRING", "\\\\"),
				NL,
				expect(t)("STRING", "\\/"),
				NL,
				expect(t)("STRING", "a"),
			},
		},
		{
			desc: "symbols",
			lines: `
				:sym
				:dotAfter.
				:special?
				:special!
				:notSpecial*
			`,
			exp: []expected{
				expect(t)("SYMBOL", ":sym"),
				NL,
				expect(t)("SYMBOL", ":dotAfter"),
				expect(t)("DOT", "."),
				NL,
				expect(t)("SYMBOL", ":special?"),
				NL,
				expect(t)("SYMBOL", ":special!"),
				NL,
				expect(t)("SYMBOL", ":notSpecial"),
				expect(t)("ASTERISK", "*"),
			},
		},
		{
			desc: "ampersand",
			lines: `
				&foo
				&
				&&
			`,
			exp: []expected{
				expect(t)("AND", "&"),
				expect(t)("IDENT", "foo"),
				NL,
				expect(t)("AND", "&"),
				NL,
				expect(t)("LOGICALAND", "&&"),
			},
		},
		{
			desc: "operators",
			lines: `
				!-/ *%5;
				+= -= *= /= %=
				5 < 10 > 5
				10 == 10
				10 != 9
				10 <= 9
				10 >= 9
				10 <=> 9
				10 << 9
			`,
			exp: []expected{
				expect(t)("BANG", "!"),
				expect(t)("MINUS", "-"),
				expect(t)("SLASH", "/"),
				expect(t)("ASTERISK", "*"),
				expect(t)("MODULO", "%"),
				expect(t)("INT", "5"),
				expect(t)("SEMICOLON", ";"),
				NL,
				expect(t)("ADDASSIGN", "+="),
				expect(t)("SUBASSIGN", "-="),
				expect(t)("MULASSIGN", "*="),
				expect(t)("DIVASSIGN", "/="),
				expect(t)("MODASSIGN", "%="),
				NL,
				expect(t)("INT", "5"),
				expect(t)("LT", "<"),
				expect(t)("INT", "10"),
				expect(t)("GT", ">"),
				expect(t)("INT", "5"),
				NL,
				expect(t)("INT", "10"),
				expect(t)("EQ", "=="),
				expect(t)("INT", "10"),
				NL,
				expect(t)("INT", "10"),
				expect(t)("NOTEQ", "!="),
				expect(t)("INT", "9"),
				NL,
				expect(t)("INT", "10"),
				expect(t)("LTE", "<="),
				expect(t)("INT", "9"),
				NL,
				expect(t)("INT", "10"),
				expect(t)("GTE", ">="),
				expect(t)("INT", "9"),
				NL,
				expect(t)("INT", "10"),
				expect(t)("SPACESHIP", "<=>"),
				expect(t)("INT", "9"),
				NL,
				expect(t)("INT", "10"),
				expect(t)("LSHIFT", "<<"),
				expect(t)("INT", "9"),
			},
		},
		{
			desc: "while",
			lines: `
				while x < y do
					x += x
				end
			`,
			exp: []expected{
				expect(t)("IDENT", "while"),
				expect(t)("IDENT", "x"),
				expect(t)("LT", "<"),
				expect(t)("IDENT", "y"),
				expect(t)("IDENT", "do"),
				NL,
				expect(t)("IDENT", "x"),
				expect(t)("ADDASSIGN", "+="),
				expect(t)("IDENT", "x"),
				NL,
				expect(t)("END", "end"),
			},
		},
		{
			desc: "loop",
			lines: `
				loop {
					break unless x < 10
				}
			`,
			exp: []expected{
				expect(t)("LOOP", "loop"),
				expect(t)("LBRACE", "{"),
				NL,
				expect(t)("BREAK", "break"),
				expect(t)("UNLESS", "unless"),
				expect(t)("IDENT", "x"),
				expect(t)("LT", "<"),
				expect(t)("INT", "10"),
				NL,
				expect(t)("RBRACE", "}"),
			},
		},
		{
			desc: "if",
			lines: `
				if x < y then
					true
				else
					false
				end
			`,
			exp: []expected{
				expect(t)("IF", "if"),
				expect(t)("IDENT", "x"),
				expect(t)("LT", "<"),
				expect(t)("IDENT", "y"),
				expect(t)("THEN", "then"),
				NL,
				expect(t)("TRUE", "true"),
				NL,
				expect(t)("ELSE", "else"),
				NL,
				expect(t)("FALSE", "false"),
				NL,
				expect(t)("END", "end"),
			},
		},
		{
			desc: "qmark",
			lines: `
				foo.bar?
				foo.bar ? 1 : 2
				?-
				?\n
				? foo : bar
				`,
			exp: []expected{
				expect(t)("IDENT", "foo"),
				expect(t)("DOT", "."),
				expect(t)("IDENT", "bar?"),
				NL,
				expect(t)("IDENT", "foo"),
				expect(t)("DOT", "."),
				expect(t)("IDENT", "bar"),
				expect(t)("QMARK", " ?"),
				expect(t)("INT", "1"),
				expect(t)("COLON", ":"),
				expect(t)("INT", "2"),
				NL,
				expect(t)("STRING", "-"),
				NL,
				expect(t)("STRING", "\\n"),
				NL,
				expect(t)("QMARK", "?"),
				expect(t)("IDENT", "foo"),
				expect(t)("COLON", ":"),
				expect(t)("IDENT", "bar"),
			},
		},
		{
			desc: "globals",
			lines: `
				$foo,
				$foo;
				$Foo
				$dotAfter.
			`,
			exp: []expected{
				expect(t)("IDENT", "$foo"),
				expect(t)("COMMA", ","),
				NL,
				expect(t)("IDENT", "$foo"),
				expect(t)("SEMICOLON", ";"),
				NL,
				expect(t)("IDENT", "$Foo"),
				NL,
				expect(t)("IDENT", "$dotAfter"),
				expect(t)("DOT", "."),
			},
		},
		{
			desc: "defs",
			lines: `
				def add(x, y)
					return x + y
				end
				def nil?
				end
				def run!
				end
				module Abc
				end
				class Abc
				end
			`,
			exp: []expected{
				expect(t)("DEF", "def"),
				expect(t)("IDENT", "add"),
				expect(t)("LPAREN", "("),
				expect(t)("IDENT", "x"),
				expect(t)("COMMA", ","),
				expect(t)("IDENT", "y"),
				expect(t)("RPAREN", ")"),
				NL,
				expect(t)("RETURN", "return"),
				expect(t)("IDENT", "x"),
				expect(t)("PLUS", "+"),
				expect(t)("IDENT", "y"),
				NL,
				expect(t)("END", "end"),
				NL,
				expect(t)("DEF", "def"),
				expect(t)("IDENT", "nil?"),
				NL,
				expect(t)("END", "end"),
				NL,
				expect(t)("DEF", "def"),
				expect(t)("IDENT", "run!"),
				NL,
				expect(t)("END", "end"),
				NL,
				expect(t)("IDENT", "module"),
				expect(t)("IDENT", "Abc"),
				NL,
				expect(t)("END", "end"),
				NL,
				expect(t)("COMMENT", "class Abc"),
				NL,
				expect(t)("END", "end"),
			},
		},
		{
			desc: "blocks",
			lines: `
				begin
					rescue
				end
				add { |x| x }
				add do |x|
				end
			`,
			exp: []expected{
				expect(t)("IDENT", "begin"), // NOTE: ident, not keyword
				NL,
				expect(t)("IDENT", "rescue"), // NOTE: ident, not keyword
				NL,
				expect(t)("END", "end"),
				NL,
				expect(t)("IDENT", "add"),
				expect(t)("LBRACE", "{"),
				expect(t)("PIPE", "|"),
				expect(t)("IDENT", "x"),
				expect(t)("PIPE", "|"),
				expect(t)("IDENT", "x"),
				expect(t)("RBRACE", "}"),
				NL,
				expect(t)("IDENT", "add"),
				expect(t)("IDENT", "do"),
				expect(t)("PIPE", "|"),
				expect(t)("IDENT", "x"),
				expect(t)("PIPE", "|"),
				NL,
				expect(t)("END", "end"),
			},
		},
		{
			desc: "comments",
			lines: `
				# just comment
				#
			`,
			exp: []expected{
				expect(t)("COMMENT", "# just comment"),
				NL,
				expect(t)("COMMENT", "#"),
			},
		},
		{
			desc: "logical",
			lines: `
				a || b
				a or b
				a && b
				a and b
				`,
			exp: []expected{
				expect(t)("IDENT", "a"),
				expect(t)("LOGICALOR", "||"),
				expect(t)("IDENT", "b"),
				NL,
				expect(t)("IDENT", "a"),
				expect(t)("LOGICALOR", "or"),
				expect(t)("IDENT", "b"),
				NL,
				expect(t)("IDENT", "a"),
				expect(t)("LOGICALAND", "&&"),
				expect(t)("IDENT", "b"),
				NL,
				expect(t)("IDENT", "a"),
				expect(t)("LOGICALAND", "and"),
				expect(t)("IDENT", "b"),
			},
		},
		{
			desc: "pipe",
			lines: `
				|
				||
			`,
			exp: []expected{
				expect(t)("PIPE", "|"),
				NL,
				expect(t)("LOGICALOR", "||"),
			},
		},
		{
			desc: "misc",
			lines: `
				=>
				->
				__FILE__
				self
				nil
				yield
			`,
			exp: []expected{
				expect(t)("HASHROCKET", "=>"),
				NL,
				expect(t)("LAMBDAROCKET", "->"),
				NL,
				expect(t)("IDENT", "__FILE__"),
				NL,
				expect(t)("IDENT", "self"),
				NL,
				expect(t)("NIL", "nil"),
				NL,
				expect(t)("IDENT", "yield"),
			},
		},
		{
			desc: "range",
			lines: `
				1..2
				1...2	
				..
				...
			`,
			exp: []expected{
				expect(t)("INT", "1"),
				expect(t)("DDOT", ".."),
				expect(t)("INT", "2"),
				NL,
				expect(t)("INT", "1"),
				expect(t)("DDDOT", "..."),
				expect(t)("INT", "2"),
				NL,
				expect(t)("DDOT", ".."),
				NL,
				expect(t)("DDDOT", "..."),
			},
		},
		{
			desc: "brackets",
			lines: `
				[1, 2]
				a[1]
				a [1]
			`,
			exp: []expected{
				expect(t)("LBRACKET", "["),
				expect(t)("INT", "1"),
				expect(t)("COMMA", ","),
				expect(t)("INT", "2"),
				expect(t)("RBRACKET", "]"),
				NL,
				expect(t)("IDENT", "a"),
				expect(t)("LBRACKET", "["),
				expect(t)("INT", "1"),
				expect(t)("RBRACKET", "]"),
				NL,
				expect(t)("IDENT", "a"),
				expect(t)("SLBRACKET", " ["),
				expect(t)("INT", "1"),
				expect(t)("RBRACKET", "]"),
			},
		},
		{
			desc: "hash_of_lambdas",
			lines: `
				"\"" => -> (a) { val_to_str a }
			`,
			exp: []expected{
				expect(t)("STRING", "\\\""),
				expect(t)("HASHROCKET", "=>"),
				expect(t)("LAMBDAROCKET", "->"),
				expect(t)("LPAREN", "("),
				expect(t)("IDENT", "a"),
				expect(t)("RPAREN", ")"),
				expect(t)("LBRACE", "{"),
				expect(t)("IDENT", "val_to_str"),
				expect(t)("IDENT", "a"),
				expect(t)("RBRACE", "}"),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			lexer := New(preprocessLines(test.lines))
			tokens := allTokens(lexer)
			assert.Equal(t, len(tokens), len(test.exp))
			for i, exp := range test.exp {
				assert.Equal(t, tokens[i].Type, exp.typ)
				assert.Equal(t, tokens[i].Literal, exp.lit)
			}
		})
	}
}

// Tests that the lexer can handle the source of pyra.rb
// https://github.com/ConorOBrien-Foxx/Pyramid-Scheme/blob/master/pyra.rb
func TestLexPyraRb(t *testing.T) {
	filename := "../pyra.rb"
	file, err := os.ReadFile(filename)
	if err != nil {
		t.Skip("Skipping test, file not found:", filename)
	}
	lexer := New(string(file))

	for {
		tk := lexer.NextToken()
		if tk.Type == token.EOF {
			break
		}
	}
}
