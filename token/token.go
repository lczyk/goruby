package token

import (
	"strconv"
)

//go:generate stringer -type=Type

// Recognized token types
const (
	ILLEGAL Type = iota // An illegal/unknown character
	EOF                 // end of input

	// Identifier + literals
	literal_beg
	IDENT
	CONST
	GLOBAL
	CLASS_VAR // @@variable
	INT
	FLOAT
	STRING
	REGEX          // /pattern/
	XSTR           // `command`
	STRING_BEG     // " -- start of a double-quoted string
	STRING_CONTENT // literal content within a string
	STRING_END     // " -- closing quote of a string
	XSTR_BEG       // ` -- start of a backtick command string
	XSTR_CONTENT   // literal content within a backtick string
	XSTR_END       // ` -- closing backtick of a command string
	REGEX_BEG      // / -- start of a regex literal
	REGEX_END      // / -- end of a regex literal (literal carries flags)
	literal_end

	// Operators
	operator_beg
	operator_assign_beg
	ASSIGN            // =
	ADDASSIGN         // +=
	SUBASSIGN         // -=
	MULASSIGN         // *=
	DIVASSIGN         // /=
	MODASSIGN         // %=
	POWERASSIGN       // **=
	ORASSIGN          // ||=
	ANDASSIGN         // &&=
	LSHIFTASSIGN      // <<=
	RSHIFTASSIGN      // >>=
	ANDASSIGN_BITWISE // &=
	ORASSIGN_BITWISE  // |=
	XORASSIGN         // ^=
	operator_assign_end

	PLUS       // +
	MINUS      // -
	BANG       // !
	ASTERISK   // *
	SLASH      // /
	MODULO     // %
	XOR        // ^
	POWER      // **
	LONELY     // &.
	RANGE      // ..
	RANGEEX    // ...
	LAMBDA     // ->
	AND        // &
	LOGICALAND // &&
	PIPE       // |
	LOGICALOR  // ||

	LT        // <
	LTE       // <=
	GT        // >
	GTE       // >=
	EQ        // ==
	NOTEQ     // !=
	SPACESHIP // <=>
	LSHIFT    // <<

	TILDE  // ~
	MATCH  // =~
	NMATCH // !~
	RSHIFT // >>
	CASEEQ // ===
	operator_end

	HASHROCKET // =>

	// Delimiters

	NEWLINE // \n
	COMMA
	SEMICOLON
	HASH // #

	CAPTURE  // &
	DOT      // .
	COLON    // :
	LPAREN   // (
	RPAREN   // )
	LBRACE   // {
	RBRACE   // }
	LBRACKET // [
	RBRACKET // ]

	EMBEXPR_BEG // #{
	EMBEXPR_END // }

	SCOPE // ::
	AT    // @

	QMARK  // ?
	SYMBEG // :
	LABEL  // identifier: (label key for hashes/keyword args)

	// Keywords
	keyword_beg
	DEF
	SELF
	END
	IF
	THEN
	ELSE
	UNLESS
	TRUE
	FALSE
	RETURN
	NIL
	MODULE
	CLASS
	DO
	YIELD
	BEGIN
	RESCUE
	WHILE
	UNTIL
	CASE
	WHEN
	BREAK
	NEXT
	KW_UNDEF
	KW_SUPER
	KW_RETRY
	KW_REDO
	KW_OR
	KW_NOT
	KW_IN
	KW_FOR
	KW_ENSURE
	KW_ELSIF
	KW_DEFINED
	KW_AND
	KW_ALIAS
	KEYWORD__FILE__
	KEYWORD__LINE__
	KEYWORD__ENCODING__
	KEYWORD__DIR__
	KW_BEGIN
	KW_END
	keyword_end
)

var tokens = [...]string{
	ILLEGAL: "ILLEGAL",
	EOF:     "EOF",

	IDENT:          "IDENT",
	CONST:          "CONST",
	GLOBAL:         "GLOBAL",
	CLASS_VAR:      "CLASS_VAR",
	INT:            "INT",
	FLOAT:          "FLOAT",
	STRING:         "STRING",
	REGEX:          "REGEX",
	XSTR:           "XSTR",
	STRING_BEG:     "STRING_BEG",
	STRING_CONTENT: "STRING_CONTENT",
	STRING_END:     "STRING_END",
	XSTR_BEG:       "XSTR_BEG",
	XSTR_CONTENT:   "XSTR_CONTENT",
	XSTR_END:       "XSTR_END",
	REGEX_BEG:      "REGEX_BEG",
	REGEX_END:      "REGEX_END",

	ASSIGN:            "=",
	ADDASSIGN:         "+=",
	SUBASSIGN:         "-=",
	MULASSIGN:         "*=",
	DIVASSIGN:         "/=",
	MODASSIGN:         "%=",
	POWERASSIGN:       "**=",
	ORASSIGN:          "||=",
	ANDASSIGN:         "&&=",
	LSHIFTASSIGN:      "<<=",
	RSHIFTASSIGN:      ">>=",
	ANDASSIGN_BITWISE: "&=",
	ORASSIGN_BITWISE:  "|=",
	XORASSIGN:         "^=",

	PLUS:       "+",
	MINUS:      "-",
	BANG:       "!",
	ASTERISK:   "*",
	SLASH:      "/",
	MODULO:     "%",
	XOR:        "^",
	POWER:      "**",
	LONELY:     "&.",
	RANGE:      "..",
	RANGEEX:    "...",
	LAMBDA:     "->",
	AND:        "&",
	CAPTURE:    "&",
	LOGICALAND: "&&",
	LOGICALOR:  "||",

	LT:        "<",
	LTE:       "<=",
	GT:        ">",
	GTE:       ">=",
	EQ:        "==",
	NOTEQ:     "!=",
	SPACESHIP: "<=>",
	LSHIFT:    "<<",
	TILDE:     "~",
	MATCH:     "=~",
	NMATCH:    "!~",
	RSHIFT:    ">>",
	CASEEQ:    "===",

	NEWLINE:   "NEWLINE",
	COMMA:     ",",
	SEMICOLON: ";",
	HASH:      "#",

	DOT:         ".",
	COLON:       ":",
	LPAREN:      "(",
	RPAREN:      ")",
	LBRACE:      "{",
	RBRACE:      "}",
	LBRACKET:    "[",
	RBRACKET:    "]",
	EMBEXPR_BEG: "EMBEXPR_BEG",
	EMBEXPR_END: "EMBEXPR_END",
	PIPE:        "|",

	SCOPE:      "::",
	HASHROCKET: "=>",
	AT:         "@",

	QMARK:  "?",
	SYMBEG: ":",
	LABEL:  "LABEL",

	DEF:                 "def",
	SELF:                "self",
	END:                 "end",
	UNLESS:              "unless",
	IF:                  "if",
	THEN:                "then",
	ELSE:                "else",
	TRUE:                "true",
	FALSE:               "false",
	RETURN:              "return",
	NIL:                 "nil",
	MODULE:              "module",
	CLASS:               "class",
	DO:                  "do",
	YIELD:               "yield",
	BEGIN:               "begin",
	RESCUE:              "rescue",
	WHILE:               "while",
	UNTIL:               "until",
	CASE:                "case",
	WHEN:                "when",
	BREAK:               "break",
	NEXT:                "next",
	KEYWORD__FILE__:     "__FILE__",
	KEYWORD__LINE__:     "__LINE__",
	KEYWORD__ENCODING__: "__ENCODING__",
	KEYWORD__DIR__:      "__dir__",
	KW_BEGIN:            "BEGIN",
	KW_END:              "END",
	KW_ALIAS:            "alias",
	KW_AND:              "and",
	KW_DEFINED:          "defined?",
	KW_ELSIF:            "elsif",
	KW_ENSURE:           "ensure",
	KW_FOR:              "for",
	KW_IN:               "in",
	KW_NOT:              "not",
	KW_OR:               "or",
	KW_REDO:             "redo",
	KW_RETRY:            "retry",
	KW_SUPER:            "super",
	KW_UNDEF:            "undef",
}

// String returns the string corresponding to the token tok.
// For operators, delimiters, and keywords the string is the actual
// token character sequence (e.g., for the token ADD, the string is
// "+"). For all other tokens the string corresponds to the token
// constant name (e.g. for the token IDENT, the string is "IDENT").
func (tok Type) String() string {
	s := ""
	if 0 <= tok && tok < Type(len(tokens)) {
		s = tokens[tok]
	}
	if s == "" {
		s = "token(" + strconv.Itoa(int(tok)) + ")"
	}
	return s
}

var keywords map[string]Type

func init() {
	keywords = make(map[string]Type)
	for i := keyword_beg + 1; i < keyword_end; i++ {
		keywords[tokens[i]] = i
	}
}

// LookupIdent returns a keyword Type if ident is a keyword. If ident starts
// with an upper character it returns CONST. In any other case it returns IDENT
func LookupIdent(ident string) Type {
	if tok, ok := keywords[ident]; ok {
		return tok
	}
	// Ruby constants always start with ASCII [A-Z]; direct byte indexing
	// avoids the two-alloc bytes.Runes([]byte(ident)) path. Multi-byte
	// UTF-8 lead bytes are all > 0x7F, safely outside the A-Z range.
	if len(ident) > 0 && ident[0] >= 'A' && ident[0] <= 'Z' {
		return CONST
	}
	return IDENT
}

// A Type represents a type of a known token
type Type int

// NewToken returns a new Token associated with the given Type typ, the Literal
// literal and the Position pos
func NewToken(typ Type, literal string, pos int) Token {
	return Token{typ, literal, pos}
}

// A Token represents a known token with its literal representation
type Token struct {
	Type    Type
	Literal string
	Pos     int
}

// IsLiteral returns true for tokens corresponding to identifiers
// and basic type literals; it returns false otherwise.
func (t Token) IsLiteral() bool {
	return t.Type.IsLiteral()
}

// IsOperator returns true for tokens corresponding to operators and
// delimiters; it returns false otherwise.
func (t Token) IsOperator() bool {
	return t.Type.IsOperator()
}

// IsAssignOperator returns true for tokens corresponding to assignment
// operators and delimiters; it returns false otherwise.
func (t Token) IsAssignOperator() bool {
	return t.Type.IsAssignOperator()
}

// IsKeyword returns true for tokens corresponding to keywords;
// it returns false otherwise.
func (t Token) IsKeyword() bool {
	return t.Type.IsKeyword()
}

// Predicates

// IsLiteral returns true for tokens corresponding to identifiers
// and basic type literals; it returns false otherwise.
func (tok Type) IsLiteral() bool { return literal_beg < tok && tok < literal_end }

// IsOperator returns true for tokens corresponding to operators and
// delimiters; it returns false otherwise.
func (tok Type) IsOperator() bool { return operator_beg < tok && tok < operator_end }

// IsAssignOperator returns true for tokens corresponding to assignment
// operators and delimiters; it returns false otherwise.
func (tok Type) IsAssignOperator() bool {
	return operator_assign_beg < tok && tok < operator_assign_end
}

// IsKeyword returns true for tokens corresponding to keywords;
// it returns false otherwise.
func (tok Type) IsKeyword() bool { return keyword_beg < tok && tok < keyword_end }
