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
	KW_USING
	KW_REFINE
	KEYWORD__CALLEE__
	KEYWORD__METHOD__
	keyword_end
)

// TypeMax is the largest valid Type value (exclusive upper bound: TypeMax+1
// is enough capacity for a Type-indexed table).
const TypeMax = keyword_end - 1

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
	KW_USING:            "using",
	KW_REFINE:           "refine",
	KEYWORD__CALLEE__:   "__callee__",
	KEYWORD__METHOD__:   "__method__",
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

// typeFixedLits[t] is the canonical literal text for Type t when t is a
// keyword, operator, or punctuation token whose source text is fully
// determined by its type. Empty for variable-content types (IDENT, INT,
// STRING, NEWLINE-as-name etc).
var typeFixedLits [TypeMax + 1]string

func init() {
	keywords = make(map[string]Type)
	for i := keyword_beg + 1; i < keyword_end; i++ {
		keywords[tokens[i]] = i
	}
	variable := map[Type]bool{
		ILLEGAL: true, EOF: true,
		IDENT: true, CONST: true, GLOBAL: true, CLASS_VAR: true,
		INT: true, FLOAT: true, STRING: true, REGEX: true, XSTR: true,
		STRING_BEG: true, STRING_CONTENT: true, STRING_END: true,
		XSTR_BEG: true, XSTR_CONTENT: true, XSTR_END: true,
		REGEX_BEG: true, REGEX_END: true,
		NEWLINE: true, HASH: true,
		EMBEXPR_BEG: true, EMBEXPR_END: true,
		LABEL: true, SYMBEG: true,
	}
	for i := Type(0); i <= TypeMax; i++ {
		if variable[i] {
			continue
		}
		if int(i) < len(tokens) {
			typeFixedLits[i] = tokens[i]
		}
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

// A Type represents a type of a known token.
// Sized int32: ~265 distinct token types in this package fit comfortably,
// and the narrower field lets Token pack to 32B instead of 40B.
type Type int32

// NewToken returns a new Token associated with the given Type typ, the
// Literal literal and the Position pos. The End field is filled from
// len(literal) so callers gain a complete source span without having to
// thread the end offset through every emit site.
func NewToken(typ Type, literal string, pos int) Token {
	return Token{Type: typ, Literal: literal, Pos: pos, End: int32(len(literal))}
}

// A Token represents a known token with its literal representation.
// Field order is tuned for compact layout: the 16B string header sits first
// (8B align), then 8B Pos, 4B End, 4B Type, and four trailing bools fit in
// the final 4B. Total: 36B -> 40B padded.
//
// The End field stores the token's source length (so the exclusive end
// offset is Pos + int(End)). Redundant with len(Literal) for now, exposed
// so consumers can migrate off Literal without re-deriving spans. Once
// readers are off Literal, Literal is removed and Token packs into 24B.
type Token struct {
	Literal         string
	Pos             int
	End             int32
	Type            Type
	HadWhitespace   bool // true if whitespace was skipped before this token
	SingleQuoted    bool // true for STRING tokens emitted from a single-quoted source literal
	IsCharLit       bool // true for STRING tokens emitted from a `?X` character literal
	HeredocStripped bool // true on STRING_BEG for `<<~` heredocs whose source had a positive common indent
}

// EndPos returns the exclusive end byte offset of the token in source.
func (tok Token) EndPos() int { return tok.Pos + int(tok.End) }

// LitOf returns the token's source text given the original lexer input.
// Returns "" for synthetic / position-less tokens (Pos < 0 or zero-length).
// Migration target for callers currently reading tok.Literal directly --
// usable while the parser still holds source. After-parse readers must use
// AST-stored values instead.
func (tok Token) LitOf(source string) string {
	if tok.End == 0 {
		return ""
	}
	end := tok.Pos + int(tok.End)
	if tok.Pos < 0 || end > len(source) {
		return ""
	}
	return source[tok.Pos:end]
}

// Literal returns the compile-time-constant literal text for fixed-literal
// types (keywords, operators, punctuation). Returns "" for variable-content
// types (IDENT, INT, STRING, ...) -- callers must use LitOf(source) or an
// AST-stored value instead.
func (t Type) Literal() string {
	if 0 <= t && t < Type(len(typeFixedLits)) {
		return typeFixedLits[t]
	}
	return ""
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
