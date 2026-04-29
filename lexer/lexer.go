package lexer

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/lczyk/goruby/token"
)

const eof = -1

// LexStartFn represents the entrypoint the Lexer uses to start processing the
// input.
var LexStartFn = startLexer

// StateFn represents a function which is capable of lexing parts of the
// input. It returns another StateFn to proceed with.
//
// Typically a state function would get called from LexStartFn and should
// return LexStartFn to go back to the decision loop. It also could return
// another non start state function if the partial input to parse is ambiguous.
type StateFn func(*Lexer) StateFn

// New returns a Lexer instance ready to process the given input.
func New(input string) *Lexer {
	l := &Lexer{
		input:  input,
		state:  startLexer,
		tokens: make(chan token.Token, 2), // Two token sufficient.
	}
	return l
}

// Lexer is the engine to process input and emit Tokens
type Lexer struct {
	input     string           // the string being scanned.
	state     StateFn          // the next lexing function to enter
	pos       int              // current position in the input.
	start     int              // start position of this item.
	width     int              // width of last rune read from input.
	tokens    chan token.Token // channel of scanned tokens.
	lastToken token.Token      // lastToken stores the last token emitted by the lexer
}

// NextToken will return the next token processed from the lexer.
//
// Callers should make sure to call Lexer.HasNext before calling this method
// as it will panic if it is called after token.EOF is returned.
func (l *Lexer) NextToken() token.Token {
	for {
		select {
		case item, ok := <-l.tokens:
			if ok {
				return item
			}
			panic(fmt.Errorf("no items left"))
		default:
			l.state = l.state(l)
			if l.state == nil {
				close(l.tokens)
			}
		}
	}
}

// HasNext returns true if there are tokens left, false if EOF has reached
func (l *Lexer) HasNext() bool {
	return l.state != nil
}

// emit passes a token back to the client.
func (l *Lexer) emit(t token.Type) {
	token := token.NewToken(t, l.input[l.start:l.pos], l.start)
	l.lastToken = token
	l.tokens <- token
	l.start = l.pos
}

// next returns the next rune in the input.
func (l *Lexer) next() rune {
	if l.pos >= len(l.input) {
		l.width = 0
		return eof
	}
	var r rune
	r, l.width = utf8.DecodeRuneInString(l.input[l.pos:])
	l.pos += l.width
	return r
}

// ignore skips over the pending input before this point.
func (l *Lexer) ignore() {
	l.start = l.pos
}

// backup steps back one rune.
// Can be called only once per call of next.
func (l *Lexer) backup() {
	l.pos -= l.width
}

// peek returns but does not consume
// the next rune in the input.
func (l *Lexer) peek() rune {
	r := l.next()
	l.backup()
	return r
}

// checks if the next substring matches the given string
func (l *Lexer) peek_string_match(s string) bool {
	if l.pos+len(s) > len(l.input) {
		return false
	}
	for i := 0; i < len(s); i++ {
		if l.input[l.pos+i] != s[i] {
			return false
		}
	}
	return true
}

// error returns an error token and terminates the scan by passing
// back a nil pointer that will be the next state, terminating l.run.
func (l *Lexer) errorf(format string, args ...interface{}) StateFn {
	l.tokens <- token.NewToken(token.ILLEGAL, fmt.Sprintf(format, args...), l.start)
	return nil
}

func startLexer(l *Lexer) StateFn {
	r := l.next()
	if isWhitespace(r) {
		// we sometimes need to emit disambiguating whitespace
		// rules:
		// before every ?
		switch l.peek() {
		case '?':
			l.next() // consume the whitespace
			l.emit(token.QMARK)
		case '[':
			l.next() // consume the whitespace
			l.emit(token.SLBRACKET)
		case 'o':
			// hack to handle space-disambiguated 'or'
			if l.peek_string_match("or ") {
				l.ignore() // ignore the space
				l.next()   // consume the o
				l.next()   // consume the r
				l.emit(token.LOGICALOR)
				l.ignore() // ignore the space
			} else {
				l.ignore()
			}
		case 'a':
			// hack to handle space-disambiguated 'and'
			if l.peek_string_match("and ") {
				l.ignore() // ignore the space
				l.next()   // consume the a
				l.next()   // consume the n
				l.next()   // consume the d
				l.emit(token.LOGICALAND)
				l.ignore() // ignore the space
			} else {
				l.ignore()
			}

		default:
			l.ignore()
		}
		return startLexer
	}
	switch r {
	case '$':
		return lexGlobal
	case '\n':
		l.emit(token.NEWLINE)
		return startLexer
	case '\'':
		return lexSingleQuoteString
	case '"':
		return lexString('"')
	case ':':
		p := l.peek()
		if isWhitespace(p) {
			l.emit(token.COLON)
			return startLexer
		}
		return lexSymbol
	case '.':
		p := l.next()
		if p == '.' {
			p = l.next()
			if p == '.' {
				l.emit(token.DDDOT)
				return startLexer
			}
			l.backup()
			l.emit(token.DDOT)
			return startLexer
		}
		l.backup()
		l.emit(token.DOT)
		return startLexer
	case '=':
		if l.peek() == '=' {
			l.next()
			l.emit(token.EQ)
		} else if l.peek() == '>' {
			l.next()
			l.emit(token.HASHROCKET)
		} else {
			l.emit(token.ASSIGN)
		}
		return startLexer
	case '+':
		if l.peek() == '=' {
			l.next()
			l.emit(token.ADDASSIGN)
			return startLexer
		}
		l.emit(token.PLUS)
		return startLexer
	case '-':
		p := l.peek()
		if p == '=' {
			l.next()
			l.emit(token.SUBASSIGN)
			return startLexer
		} else if p == '>' {
			l.next()
			l.emit(token.LAMBDAROCKET)
			return startLexer
		}
		l.emit(token.MINUS)
		return startLexer
	case '!':
		if l.peek() == '=' {
			l.next()
			l.emit(token.NOTEQ)
		} else {
			l.emit(token.BANG)
		}
		return startLexer
	case '?':
		p := l.peek()
		if isWhitespace(p) {
			l.emit(token.QMARK)
			return startLexer
		}
		if isExpressionDelimiter(p) {
			fmt.Printf("warning: invalid character syntax; use ?%c\n", r)
			l.ignore()
			return l.errorf("unexpected '?'")
		}
		return lexCharacterLiteral
	case '/':
		p := l.peek()
		if p == '=' {
			l.next()
			l.emit(token.DIVASSIGN)
			return startLexer
		} else if isWhitespace(p) || isExpressionDelimiter(p) {
			l.emit(token.SLASH)
			return startLexer
		} else {
			return lexString('/')
		}
	case '*':
		if l.peek() == '=' {
			l.next()
			l.emit(token.MULASSIGN)
			return startLexer
		} else if l.peek() == '*' {
			l.next()
			l.emit(token.POW)
			return startLexer
		}
		l.emit(token.ASTERISK)
		return startLexer
	case '%':
		if l.peek() == '=' {
			l.next()
			l.emit(token.MODASSIGN)
			return startLexer
		}
		l.emit(token.MODULO)
		return startLexer
	case '&':
		if p := l.peek(); p == '&' {
			l.next()
			l.emit(token.LOGICALAND)
			return startLexer
		}
		l.emit(token.AND)
		return startLexer
	case '<':
		if l.peek() == '=' {
			l.next()
			if l.peek() == '>' {
				l.next()
				l.emit(token.SPACESHIP)
				return startLexer
			}
			l.emit(token.LTE)
			return startLexer
		}
		if l.peek() == '<' {
			l.next()
			l.emit(token.LSHIFT)
			return startLexer
		}
		l.emit(token.LT)
		return startLexer
	case '>':
		if l.peek() == '=' {
			l.next()
			l.emit(token.GTE)
			return startLexer
		}
		l.emit(token.GT)
		return startLexer
	case '(':
		l.emit(token.LPAREN)
		return startLexer
	case ')':
		l.emit(token.RPAREN)
		return startLexer
	case '{':
		l.emit(token.LBRACE)
		return startLexer
	case '}':
		l.emit(token.RBRACE)
		return startLexer
	case '[':
		l.emit(token.LBRACKET)
		return startLexer
	case ']':
		l.emit(token.RBRACKET)
		return startLexer
	case ',':
		l.emit(token.COMMA)
		return startLexer
	case ';':
		l.emit(token.SEMICOLON)
		return startLexer
	case eof:
		l.emit(token.EOF)
		return startLexer
	case '#':
		return commentLexer
	case '|':
		if l.lastToken.Type == token.LBRACE {
			l.emit(token.PIPE)
			return startLexer
		}
		if p := l.peek(); p == '|' {
			l.next()
			l.emit(token.LOGICALOR)
			return startLexer
		}
		l.emit(token.PIPE)
		return startLexer

	default:
		if isDigit(r) {
			return lexNumber
		} else if isLetter(r) {
			return lexIdentifierOrKeyword
		} else {
			return l.errorf("Illegal character: '%c'", r)
		}
	}
}

// NOTE: Overlap of `?` and `!`
const OPERATOR_CHARS = "+-*/%&<>=,;#.:(){}[]|@$?!"
const IDENT_CHARS = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_?!"
const LEGAL_IDENT_CHARS = "?!"

func lexIdentifierOrKeywordCore(l *Lexer) rune {
	r := l.next()
	for {
		if strings.ContainsRune(IDENT_CHARS, r) {
			r = l.next()
		} else {
			break
		}
	}
	l.backup()
	return r
}

func lexIdentifierOrKeyword(l *Lexer) StateFn {
	r := lexIdentifierOrKeywordCore(l)
	literal := l.input[l.start:l.pos]
	t := token.LookupIdent(literal)
	if t == token.COMMENT {
		// TODO: i think this branch can be removed?
		for r != '\n' && r != eof {
			r = l.next()
		}
		l.backup()
		l.emit(token.COMMENT)
	} else {
		l.emit(t)
	}
	return startLexer
}

func lexNumber(l *Lexer) StateFn {
	// walk until we find a non digit
	r := l.next()
	for isDigitOrUnderscore(r) {
		r = l.next()
	}
	if r == '.' {
		// 123. ..
		r = l.next()
		if isDigit(r) {
			// 123.4 ..
			// walk until we find a non digit
			for isDigitOrUnderscore(r) {
				r = l.next()
			}
			l.backup()
			l.emit(token.FLOAT)
		} else {
			// 123. ..
			// maybe a method call. back up twice
			l.backup()
			l.backup()
			l.emit(token.INT)
		}
	} else {
		// we have an int
		l.backup()
		l.emit(token.INT)
	}
	return startLexer
}

func lexSingleQuoteString(l *Lexer) StateFn {
	l.ignore()
	r := l.next()

	for r != '\'' {
		r = l.next()
	}
	l.backup()
	l.emit(token.STRING)
	l.next()
	l.ignore()
	return startLexer
}

func lexCharacterLiteral(l *Lexer) StateFn {
	l.ignore()
	r := l.next()
	if isWhitespace(r) && r != '\t' && r != '\v' && r != '\f' && r != '\r' {
		return l.errorf("invalid character syntax; use ?\\s")
	}
	if r == '\\' {
		l.next()
	}
	if p := l.peek(); !isWhitespace(p) && !isExpressionDelimiter(p) {
		return l.errorf("unexpected '?'")
	}
	l.emit(token.STRING)
	return startLexer
}

func lexSymbol(l *Lexer) StateFn {
	_ = lexIdentifierOrKeywordCore(l)
	l.emit(token.SYMBOL)
	return startLexer
}

func lexString(end rune) StateFn {
	return func(l *Lexer) StateFn {
		l.ignore()
		r := l.next()

		for r != end {
			if r == '\\' {
				l.next()
			}
			r = l.next()
		}
		l.backup()
		l.emit(token.STRING)
		l.next()
		l.ignore()
		return startLexer
	}
}

func lexGlobal(l *Lexer) StateFn {
	_ = lexIdentifierOrKeywordCore(l)
	l.emit(token.IDENT)
	return startLexer
}

func commentLexer(l *Lexer) StateFn {
	r := l.next() // consume the '#'

	for r != '\n' && r != eof {
		r = l.next()
	}
	l.backup()
	l.emit(token.COMMENT)
	return startLexer
}

func isWhitespace(r rune) bool {
	return unicode.IsSpace(r) && r != '\n'
}

func isLetter(r rune) bool {
	return 'a' <= r && r <= 'z' || 'A' <= r && r <= 'Z' || r == '_'
}

func isDigit(r rune) bool {
	return '0' <= r && r <= '9'
}

func isDigitOrUnderscore(r rune) bool {
	return isDigit(r) || r == '_'
}

func isExpressionDelimiter(r rune) bool {
	return r == '\n' || r == ';' || r == eof
}
