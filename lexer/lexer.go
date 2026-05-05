package lexer

import (
	"bytes"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/lczyk/goruby/token"
)

const (
	eof = -1
)

// LexStartFn represents the entrypoint the Lexer uses to start processing the
// input.
var LexStartFn = startLexer

// StateFn represents a function which is capable of lexing parts of the
// input. It returns another StateFn to proceed with.
//
// Typically a state function would get called from LexStartFn and should
// return LexStartFn to go back to the decision loop. It also could return
// another non start state function if the partial input to parse is abiguous.
type StateFn func(*Lexer) StateFn

const operatorCharacters = "+-!*/%&<>=,;#.:(){}[]|@?$"

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
	input        string           // the string being scanned.
	state        StateFn          // the next lexing function to enter
	pos          int              // current position in the input.
	start        int              // start position of this item.
	width        int              // width of last rune read from input.
	tokens       chan token.Token // channel of scanned tokens.
	lastToken    token.Token      // lastToken stores the last token emitted by the lexer
	hadWhitespace bool            // true if whitespace was skipped before current token
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
			panic(fmt.Errorf("No items left"))
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

// peekSecond returns the rune after the next rune, without consuming.
func (l *Lexer) peekSecond() rune {
	l.next()
	r := l.next()
	l.backup()
	l.backup()
	return r
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
		l.hadWhitespace = true
		l.ignore()
		return startLexer
	}
	hadWhitespace := l.hadWhitespace
	l.hadWhitespace = false
	// line continuation: backslash followed by newline
	if r == '\\' {
		if l.peek() == '\n' {
			l.next() // consume newline
			l.ignore()
			return startLexer
		}
		return l.errorf("Illegal character: '%c'", r)
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
		return lexString
	case ':':
		p := l.peek()
		if p == ':' {
			l.next()
			l.emit(token.SCOPE)
			return startLexer
		}
		if isWhitespace(p) {
			l.emit(token.COLON)
			return startLexer
		}
		l.emit(token.SYMBEG)
		return startLexer
	case '.':
		if isDigit(l.peek()) {
			return lexFloat(l)
		}
		l.emit(token.DOT)
		return startLexer
	case '=':
		if l.peek() == '=' {
			l.next()
			l.emit(token.EQ)
		} else if l.peek() == '>' {
			l.next()
			l.emit(token.HASHROCKET)
		} else if l.peek() == '~' {
			l.next()
			l.emit(token.MATCH)
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
		if l.peek() == '=' {
			l.next()
			l.emit(token.SUBASSIGN)
			return startLexer
		}
		l.emit(token.MINUS)
		return startLexer
	case '!':
		if l.peek() == '=' {
			l.next()
			l.emit(token.NOTEQ)
		} else if l.peek() == '~' {
			l.next()
			l.emit(token.NMATCH)
		} else {
			l.emit(token.BANG)
		}
		return startLexer
	case '~':
		l.emit(token.TILDE)
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
		if l.peek() == '=' {
			l.next()
			l.emit(token.DIVASSIGN)
			return startLexer
		}
		if isRegexBeginContext(l.lastToken.Type) {
			return lexRegex
		}
		l.emit(token.SLASH)
		return startLexer
	case '*':
		if l.peek() == '=' {
			l.next()
			l.emit(token.MULASSIGN)
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
		// % literal: %w[...], %q{...}, %r/.../, %x|...|, %{...}
		// Same context as regex, or after whitespace following IDENT/CONST
		// (method call syntax: `foo %w[a b]`)
		if isRegexBeginContext(l.lastToken.Type) ||
			(hadWhitespace && isMethodCallTarget(l.lastToken.Type)) {
			return lexPercentLiteral
		}
		l.emit(token.MODULO)
		return startLexer
	case '&':
		if p := l.peek(); p == '&' {
			l.next()
			l.emit(token.LOGICALAND)
			return startLexer
		}
		if p := l.peek(); isLetter(p) {
			l.emit(token.CAPTURE)
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
	case '^':
		l.emit(token.XOR)
		return startLexer
	case '`':
		return lexBacktick
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
		if l.lastToken.Type == token.DO || l.lastToken.Type == token.LBRACE {
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
	case '@':
		if l.peek() == '@' {
			l.next()
			l.emit(token.CLASS_VAR)
			return startLexer
		}
		l.emit(token.AT)
		return startLexer

	default:
		if isDigit(r) {
			return lexDigit
		} else if isLetter(r) {
			return lexIdentifier
		} else {
			return l.errorf("Illegal character: '%c'", r)
		}
	}
}

func lexIdentifier(l *Lexer) StateFn {
	legalIdentifierCharacters := []byte{'?', '!'}
	r := l.next()
	for {
		if unicode.IsSpace(r) || strings.ContainsRune(operatorCharacters, r) || r == eof {
			if bytes.ContainsRune(legalIdentifierCharacters, r) {
				l.next()
				break
			}
			break
		}
		r = l.next()
	}
	l.backup()
	literal := l.input[l.start:l.pos]
	l.emit(token.LookupIdent(literal))
	return startLexer
}

func lexDigit(l *Lexer) StateFn {
	r := l.next()

	// Leading zero: check for hex, octal, binary, decimal prefixes.
	if r == '0' {
		p := l.peek()
		switch p {
		case 'x', 'X':
			l.next() // consume x
			return lexHexDigits(l)
		case 'o', 'O':
			l.next() // consume o
			return lexOctDigits(l)
		case 'b', 'B':
			l.next() // consume b
			return lexBinDigits(l)
		case 'd', 'D':
			l.next() // consume d
			return lexDecimalDigits(l)
		case '.': // 0.5
			l.next() // consume .
			return lexFloatFraction(l)
		}
		// If followed by digit, continue reading as octal/decimal int.
		if isDigitOrUnderscore(p) {
			r = l.next()
		}
	}

	// Integer part.
	for isDigitOrUnderscore(r) {
		r = l.next()
	}

	// Check what follows the integer part.
	if r == '.' && isDigit(l.peek()) {
		return lexFloatFraction(l)
	}
	if r == 'e' || r == 'E' {
		p := l.peek()
		if p == '+' || p == '-' {
			if isDigit(l.peekSecond()) {
				l.next() // consume e/E
				l.next() // consume +/-
				return lexFloatExponent(l)
			}
		} else if isDigit(p) {
			l.next() // consume e/E
			return lexFloatExponent(l)
		}
	}
	// Rational or complex suffix.
	if r == 'r' || r == 'i' {
		l.next() // consume suffix
		l.emit(token.INT)
		return startLexer
	}

	l.backup()
	l.emit(token.INT)
	return startLexer
}

func lexFloat(l *Lexer) StateFn {
	// Leading-dot float: .5 — the dot was already consumed by startLexer.
	return lexFloatFraction(l)
}

func lexFloatFraction(l *Lexer) StateFn {
	// Fractional part: already consumed the dot, read digits.
	r := l.next()
	for isDigitOrUnderscore(r) {
		r = l.next()
	}
	// Optional exponent.
	if r == 'e' || r == 'E' {
		p := l.peek()
		if p == '+' || p == '-' {
			if isDigit(l.peekSecond()) || p == '-' {
				l.next()
				l.next()
				return lexFloatExponent(l)
			}
		} else if isDigit(p) {
			l.next()
			return lexFloatExponent(l)
		}
	}
	// Optional rational/complex suffix.
	if r == 'r' || r == 'i' {
		l.next()
	}
	l.backup()
	l.emit(token.FLOAT)
	return startLexer
}

func lexFloatExponent(l *Lexer) StateFn {
	r := l.next()
	for isDigitOrUnderscore(r) {
		r = l.next()
	}
	// Optional rational/complex suffix after exponent.
	if r == 'r' || r == 'i' {
		l.next()
	}
	l.backup()
	l.emit(token.FLOAT)
	return startLexer
}

func lexHexDigits(l *Lexer) StateFn {
	r := l.next()
	for isHexDigit(r) {
		r = l.next()
	}
	l.backup()
	l.emit(token.INT)
	return startLexer
}

func lexOctDigits(l *Lexer) StateFn {
	r := l.next()
	for isOctDigit(r) {
		r = l.next()
	}
	l.backup()
	l.emit(token.INT)
	return startLexer
}

func lexBinDigits(l *Lexer) StateFn {
	r := l.next()
	for isBinDigit(r) {
		r = l.next()
	}
	l.backup()
	l.emit(token.INT)
	return startLexer
}

func lexDecimalDigits(l *Lexer) StateFn {
	r := l.next()
	for isDigitOrUnderscore(r) {
		r = l.next()
	}
	l.backup()
	l.emit(token.INT)
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
		r = l.next()
	}
	if p := l.peek(); !isWhitespace(p) && !isExpressionDelimiter(p) {
		return l.errorf("unexpected '?'")
	}
	l.emit(token.STRING)
	return startLexer
}

func lexString(l *Lexer) StateFn {
	l.ignore()
	r := l.next()

	for r != '"' {
		if r == '\\' {
			l.next() // skip escaped character (e.g. \", \\, \n, \t)
		} else if r == '#' && l.peek() == '{' {
			l.next() // consume {
			l.next() // consume first char of interpolation
			skipInterpolation(l)
		} else if r == eof {
			return l.errorf("unterminated string")
		}
		r = l.next()
	}
	l.backup()
	l.emit(token.STRING)
	l.next()
	l.ignore()
	return startLexer
}

// skipInterpolation skips characters until the matching '}' for #{...}.
func skipInterpolation(l *Lexer) {
	depth := 1
	for depth > 0 {
		r := l.next()
		switch r {
		case eof:
			return
		case '{':
			depth++
		case '}':
			depth--
		case '"':
			// nested string inside interpolation
			for {
				c := l.next()
				if c == eof || c == '\n' {
					return
				}
				if c == '\\' {
					l.next() // skip escaped char
					continue
				}
				if c == '"' {
					break
				}
			}
		case '\'':
			// nested single-quoted string
			for {
				c := l.next()
				if c == eof || c == '\n' {
					return
				}
				if c == '\'' {
					break
				}
			}
		case '/':
			// nested regex inside interpolation
			for {
				c := l.next()
				if c == eof || c == '\n' {
					return
				}
				if c == '\\' {
					l.next()
					continue
				}
				if c == '/' {
					break
				}
			}
		case '\\':
			l.next() // skip escaped char
		case '#':
			if l.peek() == '{' {
				l.next()
				depth++
			}
		}
	}
}

func lexGlobal(l *Lexer) StateFn {
	r := l.next()

	if isWhitespace(r) {
		return l.errorf("Illegal character: '%c'", r)
	}

	if isExpressionDelimiter(r) {
		return l.errorf("Illegal character: '%c'", r)
	}

	// Single-character punctuation or digit globals: $., $?, $!, $~, $0, etc.
	if isGlobalPunct(r) || isDigit(r) {
		l.emit(token.GLOBAL)
		return startLexer
	}

	for !isWhitespace(r) && !isExpressionDelimiter(r) && !isGlobalDelim(r) {
		r = l.next()
	}
	l.backup()
	l.emit(token.GLOBAL)
	return startLexer
}

func isGlobalPunct(r rune) bool {
	switch r {
	case '.', '?', '!', '~', '@', ';', ':', '"', '<', '>', '\\', '/',
		'&', '*', '\'', '+', '-', '=', '$', '`', ',':
		return true
	}
	return false
}

func isGlobalDelim(r rune) bool {
	return isGlobalPunct(r) || r == '.' || r == ','
}

func commentLexer(l *Lexer) StateFn {
	l.emit(token.HASH)
	r := l.next()

	for r != '\n' && r != eof {
		r = l.next()
	}
	l.backup()
	l.emit(token.STRING)
	return startLexer
}

func lexPercentLiteral(l *Lexer) StateFn {
	l.ignore() // skip %

	opener := l.next()

	// %q, %Q, %w, %W, %i, %I, %r, %x, %s — opener is the type char
	if isLetter(opener) {
		opener = l.next() // next char is the actual delimiter
	}

	closing := closingDelim(opener)
	paired := opener != closing // {} () [] <> track depth; // !! track single match

	if paired {
		depth := 1
		for depth > 0 {
			c := l.next()
			if c == eof {
				return l.errorf("unterminated percent literal")
			}
			if c == '\\' {
				l.next()
				continue
			}
			if c == opener {
				depth++
			} else if c == closing {
				depth--
			}
		}
	} else {
		for {
			c := l.next()
			if c == eof {
				return l.errorf("unterminated percent literal")
			}
			if c == '\\' {
				l.next()
				continue
			}
			if c == closing {
				break
			}
		}
	}
	l.backup()
	l.emit(token.STRING)
	l.next()
	l.ignore() // consume closing delimiter
	return startLexer
}

func closingDelim(r rune) rune {
	switch r {
	case '{':
		return '}'
	case '(':
		return ')'
	case '[':
		return ']'
	case '<':
		return '>'
	default:
		return r
	}
}

func lexBacktick(l *Lexer) StateFn {
	l.ignore()
	r := l.next()

	for r != '`' {
		if r == eof {
			return l.errorf("unterminated command literal")
		}
		r = l.next()
	}
	l.backup()
	l.emit(token.XSTR)
	l.next()
	l.ignore()
	return startLexer
}

func isExpressionEnd(tok token.Type) bool {
	switch tok {
	case token.IDENT, token.CONST, token.GLOBAL, token.CLASS_VAR,
		token.INT, token.STRING, token.REGEX, token.XSTR,
		token.RPAREN, token.RBRACKET, token.RBRACE,
		token.TRUE, token.FALSE, token.NIL, token.SELF,
		token.END:
		return true
	}
	return false
}

func isMethodCallTarget(tok token.Type) bool {
	switch tok {
	case token.IDENT, token.CONST, token.GLOBAL,
		token.RPAREN, token.RBRACKET, token.RBRACE,
		token.END:
		return true
	}
	return false
}

func isRegexBeginContext(tok token.Type) bool {
	switch tok {
	case token.EOF, token.NEWLINE,
		token.ASSIGN, token.MATCH, token.NMATCH,
		token.LPAREN, token.LBRACKET, token.LBRACE,
		token.COMMA, token.SEMICOLON, token.COLON, token.QMARK,
		token.BANG, token.TILDE,
		token.IF, token.UNLESS, token.WHILE, token.UNTIL, token.RETURN, token.THEN,
		token.DO, token.CASE, token.WHEN, token.BREAK, token.NEXT,
		token.HASHROCKET:
		return true
	}
	return false
}

func lexRegex(l *Lexer) StateFn {
	l.ignore()
	r := l.next()

	for r != '/' {
		if r == eof {
			return l.errorf("unterminated regexp")
		}
		if r == '\\' {
			l.next() // skip escaped character
		}
		r = l.next()
	}
	l.backup()
	l.emit(token.REGEX)
	l.next()
	l.ignore()
	// Consume regex options (i, m, x, o)
	for {
		r := l.next()
		if r != 'i' && r != 'm' && r != 'x' && r != 'o' {
			l.backup()
			break
		}
	}
	return startLexer
}

func isWhitespace(r rune) bool {
	return unicode.IsSpace(r) && r != '\n'
}

func isLetter(r rune) bool {
	return unicode.IsLetter(r) || r == '_'
}

func isDigit(r rune) bool {
	return '0' <= r && r <= '9'
}

func isDigitOrUnderscore(r rune) bool {
	return isDigit(r) || r == '_'
}

func isHexDigit(r rune) bool {
	return isDigit(r) || ('a' <= r && r <= 'f') || ('A' <= r && r <= 'F') || r == '_'
}

func isOctDigit(r rune) bool {
	return ('0' <= r && r <= '7') || r == '_'
}

func isBinDigit(r rune) bool {
	return r == '0' || r == '1' || r == '_'
}

func isExpressionDelimiter(r rune) bool {
	return r == '\n' || r == ';' || r == eof
}
