package lexer

import (
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

// interpState saves the lexer state before entering an interpolation region.
type interpState struct {
	stateFn      StateFn // state to return to (e.g. lexStringContent)
	returnOnNext bool    // if true, pop on next checkInterpStack call (for #$var style)
	braceDepth   int     // outer braceDepth, restored on pop
}

// Option configures the lexer.
type Option func(*Lexer)

// WithVersion sets the target ruby version. The lexer will reject syntax
// introduced after this version. Zero value (default) means latest.
func WithVersion(v token.RubyVersion) Option {
	return func(l *Lexer) { l.version = v }
}

// New returns a Lexer instance ready to process the given input.
func New(input string, opts ...Option) *Lexer {
	l := &Lexer{
		input:  input,
		state:  startLexer,
		tokens: make(chan token.Token, 16),
	}
	for _, o := range opts {
		o(l)
	}
	return l
}

// Lexer is the engine to process input and emit Tokens
type Lexer struct {
	input         string           // the string being scanned.
	state         StateFn          // the next lexing function to enter
	pos           int              // current position in the input.
	start         int              // start position of this item.
	width         int              // width of last rune read from input.
	tokens        chan token.Token // channel of scanned tokens.
	lastToken     token.Token      // lastToken stores the last token emitted by the lexer
	hadWhitespace bool             // true if whitespace was skipped before current token
	version       token.RubyVersion

	// Heredoc state.
	heredocDelim    string
	heredocIndent   bool // <<-
	heredocSquig    bool // <<~
	heredocQuote    rune
	heredocPostBody string // bytes from after delim to end-of-line (incl. \n);
	// spliced back into input after the heredoc body's STRING_END so trailers
	// like <<EOS.chop and chained heredocs like <<A, <<B both lex naturally.

	// Interpolation state.
	braceDepth  int
	interpStack []interpState
}

// NextToken returns the next token from the input. When the lexer is exhausted
// (state is nil and the token buffer is drained), it returns token.EOF. Safe
// to call without checking HasNext first -- it will keep returning EOF.
func (l *Lexer) NextToken() token.Token {
	for {
		select {
		case item, ok := <-l.tokens:
			if ok {
				return item
			}
			return token.NewToken(token.EOF, "", l.pos)
		default:
			if l.state == nil {
				return token.NewToken(token.EOF, "", l.pos)
			}
			l.state = l.state(l)
			// When the state chain ends, close the channel only once buffered
			// tokens have been drained by the caller. This avoids trapping
			// tokens (e.g. ILLEGAL) that were emitted before errorf set state
			// to nil within a single NextToken call.
			if l.state == nil && len(l.tokens) == 0 {
				close(l.tokens)
			}
		}
	}
}

// HasNext returns true if there are tokens left (including buffered tokens
// emitted before the state machine stopped), false if the input is exhausted.
func (l *Lexer) HasNext() bool {
	return l.state != nil || len(l.tokens) > 0
}

// emit passes a token back to the client.
func (l *Lexer) emit(t token.Type) {
	tok := token.NewToken(t, l.input[l.start:l.pos], l.start)
	l.lastToken = tok
	l.tokens <- tok
	l.start = l.pos
}

// emitLiteral emits a token of the given type with an explicit literal,
// ignoring the input between l.start and l.pos. start is advanced to pos.
func (l *Lexer) emitLiteral(t token.Type, literal string) {
	tok := token.NewToken(t, literal, l.start)
	l.lastToken = tok
	l.tokens <- tok
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

// consumeEscape advances past the remainder of an escape sequence.
// The backslash has already been consumed; this reads the char after
// the backslash and consumes the rest of any multi-character sequence
// (e.g. \u{XXXX}, \xNN, \C-x, \M-x, \cX, \o{NNN}).
func (l *Lexer) consumeEscape() {
	r := l.next()
	switch r {
	case 'u':
		if l.peek() == '{' {
			l.next() // consume {
			for {
				c := l.next()
				if c == eof || c == '\n' {
					return
				}
				if c == '}' {
					break
				}
			}
		} else {
			// \uXXXX -- exactly 4 hex digits
			for i := 0; i < 4; i++ {
				if isHexDigit(l.peek()) {
					l.next()
				}
			}
		}
	case 'x':
		// \xNN -- 1 or 2 hex digits
		for i := 0; i < 2; i++ {
			if isHexDigit(l.peek()) {
				l.next()
			}
		}
	case 'C':
		if l.peek() == '-' {
			l.next() // consume -
			if l.peek() == '\\' {
				l.next()          // consume \
				l.consumeEscape() // target is an escape sequence (e.g. \C-\M-x, \C-\\)
			} else {
				l.next() // consume single target char
			}
		}
	case 'M':
		if l.peek() == '-' {
			l.next() // consume -
			if l.peek() == '\\' {
				l.next()          // consume \
				l.consumeEscape() // target is an escape sequence (e.g. \M-\C-x, \M-\\)
			} else {
				l.next() // consume single target char
			}
		}
	case 'c':
		l.next() // consume the control char
	case 'o':
		if l.peek() == '{' {
			l.next() // consume {
			for {
				c := l.next()
				if c == eof || c == '\n' {
					return
				}
				if c == '}' {
					break
				}
			}
		} else {
			// up to 3 octal digits
			for i := 0; i < 3; i++ {
				if isOctDigit(l.peek()) {
					l.next()
				}
			}
		}
	}
}

// peek returns but does not consume
// the next rune in the input.
func (l *Lexer) peek() rune {
	r := l.next()
	l.backup()
	return r
}

// peekPastWhitespace returns the first non-whitespace rune after the
// current position, without consuming input. Returns -1 at EOF.
func (l *Lexer) peekPastWhitespace() rune {
	pos := l.pos
	for pos < len(l.input) {
		r := rune(l.input[pos])
		if !isWhitespace(r) {
			return r
		}
		pos++
	}
	return -1
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
	// =begin block comment at column 0 (start of file or after newline).
	if r == '=' && l.start == 0 && l.lastToken.Type == token.ILLEGAL && l.lastToken.Literal == "" {
		if strings.HasPrefix(l.input[l.pos:], "begin") &&
			(l.pos+5 >= len(l.input) || l.input[l.pos+5] == '\n' || l.input[l.pos+5] == ' ' || l.input[l.pos+5] == '\t' || l.input[l.pos+5] == '\r') {
			l.backup()
			if skipBlockComment(l) {
				return startLexer
			}
			l.next()
		}
	}
	switch r {
	case '$':
		return lexGlobal
	case '\n':
		// Leading-dot: suppress NEWLINE when followed by . or &.
		// Only skip horizontal whitespace (spaces, tabs), not newlines.
		pos := l.pos
		for pos < len(l.input) {
			r := rune(l.input[pos])
			if r != ' ' && r != '\t' {
				break
			}
			pos++
		}
		// Suppress NEWLINE before .method or &.method, but NOT before
		// .. or ... (range literals).
		if pos < len(l.input) && l.input[pos] == '.' && pos+1 < len(l.input) && l.input[pos+1] != '.' {
			l.ignore()
			return startLexer
		}
		if pos < len(l.input) && l.input[pos] == '&' && pos+1 < len(l.input) && l.input[pos+1] == '.' {
			l.ignore()
			return startLexer
		}
		l.emit(token.NEWLINE)
		// =begin block comment at line start.
		if skipBlockComment(l) {
			return startLexer
		}
		// __END__ at line start: consume rest of input.
		if strings.HasPrefix(l.input[l.pos:], "__END__") {
			after := l.pos + 7
			if after >= len(l.input) || l.input[after] == '\n' || l.input[after] == '\r' {
				l.pos = len(l.input)
				l.emit(token.EOF)
				return nil
			}
		}
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
		if l.peek() == '.' {
			l.next()
			if l.peek() == '.' {
				l.next()
				l.emit(token.RANGEEX)
				return startLexer
			}
			l.emit(token.RANGE)
			return startLexer
		}
		if isDigit(l.peek()) {
			return lexFloat(l)
		}
		l.emit(token.DOT)
		return startLexer
	case '=':
		if l.peek() == '=' {
			l.next()
			if l.peek() == '=' {
				l.next()
				l.emit(token.CASEEQ)
			} else {
				l.emit(token.EQ)
			}
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
		if l.peek() == '>' {
			l.next()
			l.emit(token.LAMBDA)
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
		if isRegexBeginContext(l.lastToken.Type) {
			return lexRegexBegin
		}
		if l.peek() == '=' {
			l.next()
			l.emit(token.DIVASSIGN)
			return startLexer
		}
		l.emit(token.SLASH)
		return startLexer
	case '*':
		if l.peek() == '=' {
			l.next()
			l.emit(token.MULASSIGN)
			return startLexer
		}
		if l.peek() == '*' {
			l.next()
			if l.peek() == '=' {
				l.next()
				l.emit(token.POWERASSIGN)
				return startLexer
			}
			l.emit(token.POWER)
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
		// %q, %Q, %w, %W, %i, %I, %r, %x, %s always start percent literals.
		if p := l.peek(); isPercentTypeChar(p) {
			return lexPercentLiteral
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
			if l.peek() == '=' {
				l.next()
				l.emit(token.ANDASSIGN)
				return startLexer
			}
			l.emit(token.LOGICALAND)
			return startLexer
		}
		if l.peek() == '.' {
			l.next()
			l.emit(token.LONELY)
			return startLexer
		}
		if l.peek() == '=' {
			l.next()
			l.emit(token.ANDASSIGN_BITWISE)
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
			if l.peek() == '=' {
				l.next()
				l.emit(token.LSHIFTASSIGN)
				return startLexer
			}
			// Check for heredoc: <<, <<-, <<~
			// class << expr is singleton class syntax, not a heredoc.
			if l.lastToken.Type == token.CLASS {
				l.emit(token.LSHIFT)
				return startLexer
			}
			p := l.peek()
			if p == '-' || p == '~' {
				l.next()
				p2 := l.peek()
				if isLetter(p2) || p2 == '_' || p2 == '"' || p2 == '\'' || p2 == '`' {
					// <<~ implies indent-aware delim matching (and indent stripping
					// in the body), so pass indent=true for both - and ~.
					return lexHeredocStart(l, true, p == '~')
				}
				l.backup()
				l.emit(token.LSHIFT)
				return startLexer
			}
			if isLetter(p) || p == '_' || p == '"' || p == '\'' || p == '`' {
				return lexHeredocStart(l, false, false)
			}
			l.emit(token.LSHIFT)
			return startLexer
		}
		l.emit(token.LT)
		return startLexer
	case '>':
		if l.peek() == '>' {
			l.next()
			if l.peek() == '=' {
				l.next()
				l.emit(token.RSHIFTASSIGN)
			} else {
				l.emit(token.RSHIFT)
			}
			return startLexer
		}
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
		if inInterp(l) {
			l.braceDepth++
		}
		return startLexer
	case '}':
		if inInterp(l) {
			l.braceDepth--
			if l.braceDepth == 0 {
				l.emit(token.EMBEXPR_END)
				top := l.interpStack[len(l.interpStack)-1]
				l.interpStack = l.interpStack[:len(l.interpStack)-1]
				l.braceDepth = top.braceDepth
				return top.stateFn
			}
		}
		l.emit(token.RBRACE)
		return startLexer
	case '[':
		l.emit(token.LBRACKET)
		return startLexer
	case ']':
		l.emit(token.RBRACKET)
		return startLexer
	case '^':
		if l.peek() == '=' {
			l.next()
			l.emit(token.XORASSIGN)
			return startLexer
		}
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
		if l.peek() == '=' {
			l.next()
			l.emit(token.ORASSIGN_BITWISE)
			return startLexer
		}
		if p := l.peek(); p == '|' {
			l.next()
			if l.peek() == '=' {
				l.next()
				l.emit(token.ORASSIGN)
				return startLexer
			}
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

// inInterp returns true if the lexer is inside a #{...} interpolation region
// (as opposed to #$var / #@var style, which uses returnOnNext).
func inInterp(l *Lexer) bool {
	if len(l.interpStack) == 0 {
		return false
	}
	return !l.interpStack[len(l.interpStack)-1].returnOnNext
}

// checkInterpStack checks if we should return to a saved interpolation state.
func checkInterpStack(l *Lexer) StateFn {
	if len(l.interpStack) > 0 {
		top := &l.interpStack[len(l.interpStack)-1]
		if top.returnOnNext {
			l.interpStack = l.interpStack[:len(l.interpStack)-1]
			return top.stateFn
		}
	}
	return startLexer
}

// lexIdentifier scans an identifier after the first character has already
// been consumed by the caller (startLexer or an interp return path).
// The first character was validated by isLetter, so we start scanning
// from the second character; this is why the loop reads before checking.
func lexIdentifier(l *Lexer) StateFn {
	for {
		r := l.next()
		if isIdentChar(r) {
			continue
		}
		// ? and ! are valid method name suffixes in Ruby (e.g. nil?, run!).
		// r has already been consumed by l.next() and is within l.pos,
		// so we just break -- no need to consume the next character.
		if r == '?' || r == '!' {
			// already part of the identifier
		} else if r != eof {
			l.backup()
		}
		break
	}
	// Label detection: IDENT immediately followed by : (no space) is a label key.
	// Avoid when the identifier ends with ? or ! (method names like valid?: are
	// not valid label keys), and when the : is part of :: (scope resolution).
	if l.peek() == ':' && l.input[l.pos-1] != '?' && l.input[l.pos-1] != '!' {
		// Check for :: scope resolution -- peekSecond returns the rune after next.
		if l.peekSecond() == ':' {
			// This is :: -- emit normally, don't treat as label.
		} else {
			l.next()            // consume : so it is included in l.start..l.pos
			l.emit(token.LABEL) // literal is e.g. "foo:" -- colon stripped by parser
			return checkInterpStack
		}
	}
	literal := l.input[l.start:l.pos]
	l.emit(token.LookupIdent(literal))
	return checkInterpStack
}

func lexDigit(l *Lexer) StateFn {
	r := l.next()

	if l.input[l.start] == '0' {
		switch r {
		case 'x', 'X':
			return lexHexDigits(l)
		case 'o', 'O':
			return lexOctDigits(l)
		case 'b', 'B':
			return lexBinDigits(l)
		case 'd', 'D':
			return lexDecimalDigits(l)
		case '.':
			if isDigit(l.peek()) {
				return lexFloatFraction(l)
			}
		}
		// If followed by digit, continue reading as octal/decimal int.
		if isDigitOrUnderscore(r) {
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
	// Rational or complex suffix -- the suffix character was already
	// consumed as the r that broke the integer-part loop; emit in-place.
	if r == 'r' || r == 'i' {
		l.emit(token.INT)
		return startLexer
	}

	l.backup()
	l.emit(token.INT)
	return startLexer
}

func lexFloat(l *Lexer) StateFn {
	// Leading-dot float: .5 -- the dot was already consumed by startLexer.
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
		if r == '\\' {
			l.next() // only \\ and \' are escapes in single-quoted strings
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

func lexCharacterLiteral(l *Lexer) StateFn {
	l.ignore() // skip ?
	r := l.next()
	if isWhitespace(r) && r != '\t' && r != '\v' && r != '\f' && r != '\r' {
		return l.errorf("invalid character syntax; use ?\\s")
	}
	if r == '\\' {
		// Read the full escape sequence.
		r = l.next()
		switch r {
		case 'u':
			if l.peek() == '{' {
				l.next() // consume {
				for {
					c := l.next()
					if c == eof || c == '\n' {
						return l.errorf("unterminated Unicode escape")
					}
					if c == '}' {
						break
					}
				}
			}
			// else: \u without {} is invalid; already consumed u
		case 'x':
			// \xNN -- one or two hex digits
			for i := 0; i < 2; i++ {
				if isHexDigit(l.peek()) {
					l.next()
				}
			}
		case 'C':
			// \C-x or \C-\M-x
			if l.peek() == '-' {
				l.next() // consume -
				if l.peek() == '\\' {
					l.next()          // consume \\
					l.consumeEscape() // target is an escape sequence
				} else {
					l.next() // consume single target char
				}
				// else: \C-x -- already consumed the char after -
			}
		case 'M':
			// \M-x or \M-\C-x
			if l.peek() == '-' {
				l.next() // consume -
				if l.peek() == '\\' {
					l.next()          // consume \\
					l.consumeEscape() // target is an escape sequence
				} else {
					l.next() // consume single target char
				}
				// else: \M-x -- already consumed the char after -
			}
		case 'c':
			// \cx -- control char (lowercase c variant)
			l.next() // consume the character after \c
		case 'o':
			// \o{NNN} or \oNNN
			if l.peek() == '{' {
				l.next()
				for {
					c := l.next()
					if c == eof || c == '\n' {
						return l.errorf("unterminated octal escape")
					}
					if c == '}' {
						break
					}
				}
			} else {
				for i := 0; i < 3; i++ {
					if isOctDigit(l.peek()) {
						l.next()
					}
				}
			}
		}
		// Simple escapes (\n, \t, etc.) are already consumed (single char after \).
	}
	// After the char/escape, emit the character as a string.
	l.emit(token.STRING)
	return startLexer
}

func lexString(l *Lexer) StateFn {
	l.ignore() // consume opening "
	l.emit(token.STRING_BEG)
	return lexStringContent
}

// lexStringContent scans through a double-quoted string, emitting STRING_CONTENT
// for literal text and handling #{...} / #$var / #@var interpolation.
func lexStringContent(l *Lexer) StateFn {
	for {
		r := l.next()
		switch r {
		case '"':
			if l.pos-l.width > l.start {
				l.backup()
				l.emit(token.STRING_CONTENT)
				l.next() // re-consume the closing "
			}
			l.emit(token.STRING_END) // literal is the closing "
			return checkInterpStack
		case '#':
			p := l.peek()
			if p == '{' {
				if l.pos-l.width > l.start {
					l.backup()
					l.emit(token.STRING_CONTENT)
					l.next() // re-consume #
				}
				l.next() // consume {
				l.emit(token.EMBEXPR_BEG)
				l.interpStack = append(l.interpStack, interpState{stateFn: lexStringContent, braceDepth: l.braceDepth})
				l.braceDepth = 1
				return startLexer
			}
			if p == '@' || p == '$' {
				if l.pos-l.width > l.start {
					l.backup()
					l.emit(token.STRING_CONTENT)
					l.next() // re-consume #
				}
				l.ignore() // skip the # character
				l.interpStack = append(l.interpStack, interpState{stateFn: lexStringContent, returnOnNext: true, braceDepth: l.braceDepth})
				if p == '@' {
					l.next() // consume @
					if l.peek() == '@' {
						l.next()
						l.emit(token.CLASS_VAR)
					} else {
						l.emit(token.AT)
					}
					return lexIdentifier
				}
				// p == '$' -- consume $ so lexGlobal starts from the variable name
				l.next()
				return lexGlobal
			}
			// plain # character in string, continue
		case '\\':
			l.consumeEscape()
		case eof:
			return l.errorf("unterminated string")
		}
	}
}

func lexGlobal(l *Lexer) StateFn {
	r := l.next()

	if isWhitespace(r) {
		return l.errorf("Illegal character: '%c'", r)
	}

	// Single-character punctuation or digit globals: $., $?, $!, $~, $;, $0, etc.
	// Must check BEFORE isExpressionDelimiter since ; is both punct and delim.
	if isGlobalPunct(r) || isDigit(r) {
		l.emit(token.GLOBAL)
		return checkInterpStack
	}

	if isExpressionDelimiter(r) {
		return l.errorf("Illegal character: '%c'", r)
	}

	for isLetter(r) || isDigit(r) || r == '_' {
		r = l.next()
	}
	l.backup()
	l.emit(token.GLOBAL)
	return checkInterpStack
}

func isGlobalPunct(r rune) bool {
	switch r {
	case '.', '?', '!', '~', '@', ';', ':', '"', '<', '>', '\\', '/',
		'&', '*', '\'', '+', '-', '=', '$', '`', ',':
		return true
	}
	return false
}

// skipBlockComment checks if the current position starts a =begin block
// comment. If so, it consumes everything through the matching =end line
// and returns true. Returns false if not at a =begin.
func skipBlockComment(l *Lexer) bool {
	if !strings.HasPrefix(l.input[l.pos:], "=begin") {
		return false
	}
	after := l.pos + 6
	if after < len(l.input) && l.input[after] != '\n' && l.input[after] != ' ' && l.input[after] != '\t' && l.input[after] != '\r' {
		return false
	}
	// Advance past the =begin line.
	l.pos = after
	for l.pos < len(l.input) && l.input[l.pos] != '\n' {
		l.pos++
	}
	if l.pos < len(l.input) {
		l.pos++ // consume \n
	}
	// Scan for =end at line start.
	for l.pos < len(l.input) {
		if strings.HasPrefix(l.input[l.pos:], "=end") {
			endAfter := l.pos + 4
			if endAfter >= len(l.input) || l.input[endAfter] == '\n' || l.input[endAfter] == ' ' || l.input[endAfter] == '\t' || l.input[endAfter] == '\r' {
				l.pos = endAfter
				for l.pos < len(l.input) && l.input[l.pos] != '\n' {
					l.pos++
				}
				if l.pos < len(l.input) {
					l.pos++ // consume trailing \n
				}
				l.ignore()
				return true
			}
		}
		// Skip to next line.
		for l.pos < len(l.input) && l.input[l.pos] != '\n' {
			l.pos++
		}
		if l.pos < len(l.input) {
			l.pos++ // consume \n
		}
	}
	l.ignore()
	return true
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

	c := l.next() // type char or delimiter if no type char

	var typ rune    // q, Q, w, W, i, I, r, x, s, or 0 for bare %{...}
	var opener rune // the opening delimiter

	if isPercentTypeChar(c) {
		typ = c
		opener = l.next()
	} else {
		typ = 0 // bare %, defaults to %Q (interpolating string)
		opener = c
	}

	closer := closingDelim(opener)
	paired := opener != closer

	switch typ {
	case 'q':
		l.ignore() // skip type char and opener
		return lexPercentLiteralBody(l, opener, closer, paired, token.STRING)
	case 'w', 'i', 's':
		l.emitLiteral(token.STRING_BEG, string(typ))
		l.ignore() // skip type char and opener
		return lexPercentLiteralBodyEnd(l, opener, closer, paired, token.STRING_CONTENT, token.STRING_END)
	case 'Q', 'W', 'I', 0:
		lit := string(typ)
		if typ == 0 {
			lit = "Q" // bare % treated as %Q
		}
		l.emitLiteral(token.STRING_BEG, lit)
		l.ignore()
		return lexPercentContent(l, opener, closer, paired, token.STRING_CONTENT, token.STRING_END)
	case 'r':
		l.emitLiteral(token.REGEX_BEG, "r")
		l.ignore()
		return lexPercentContent(l, opener, closer, paired, token.STRING_CONTENT, token.REGEX_END)
	case 'x':
		l.emitLiteral(token.XSTR_BEG, "x")
		l.ignore()
		return lexPercentContent(l, opener, closer, paired, token.XSTR_CONTENT, token.XSTR_END)
	default:
		return l.errorf("unknown percent literal type: %c", typ)
	}
}

// lexPercentLiteralBody reads a non-interpolating percent literal body
// and emits a single token of the given type.
func lexPercentLiteralBody(l *Lexer, opener, closer rune, paired bool, tok token.Type) StateFn {
	depth := 0
	if paired {
		depth = 1
	}
	for {
		r := l.next()
		if r == eof {
			return l.errorf("unterminated percent literal")
		}
		if r == '\\' {
			l.next() // non-interpolating: only escape the next char
			continue
		}
		if paired {
			if r == opener {
				depth++
				continue
			}
			if r == closer {
				depth--
				if depth == 0 {
					l.backup()
					l.emit(tok)
					l.next()
					l.ignore()
					return startLexer
				}
				continue
			}
		} else {
			if r == closer {
				l.backup()
				l.emit(tok)
				l.next()
				l.ignore()
				return startLexer
			}
		}
	}
}

// lexPercentLiteralBodyEnd reads a non-interpolating percent literal body and
// emits contentTok for the body followed by endTok for the closing delimiter.
func lexPercentLiteralBodyEnd(l *Lexer, opener, closer rune, paired bool, contentTok, endTok token.Type) StateFn {
	depth := 0
	if paired {
		depth = 1
	}
	for {
		r := l.next()
		if r == eof {
			return l.errorf("unterminated percent literal")
		}
		if r == '\\' {
			l.next() // non-interpolating: only escape the next char
			continue
		}
		if paired {
			if r == opener {
				depth++
				continue
			}
			if r == closer {
				depth--
				if depth == 0 {
					if l.pos-l.width > l.start {
						l.backup()
						l.emit(contentTok)
						l.next()
					}
					l.ignore() // skip closer
					l.emit(endTok)
					return startLexer
				}
				continue
			}
		} else {
			if r == closer {
				if l.pos-l.width > l.start {
					l.backup()
					l.emit(contentTok)
					l.next()
				}
				l.ignore() // skip closer
				l.emit(endTok)
				return startLexer
			}
		}
	}
}

// lexPercentContent scans through an interpolating percent literal body,
// emitting content tokens and handling #{...} / #$var / #@var interpolation.
func lexPercentContent(l *Lexer, opener, closer rune, paired bool,
	contentTok, endTok token.Type) StateFn {

	depth := 0
	if paired {
		depth = 1
	}
	resumeFn := func(_ *Lexer) StateFn {
		return lexPercentContent(l, opener, closer, paired, contentTok, endTok)
	}

	for {
		r := l.next()
		if r == eof {
			return l.errorf("unterminated percent literal")
		}
		if r == '\\' {
			l.consumeEscape()
			continue
		}
		if paired {
			if r == opener {
				depth++
				continue
			}
			if r == closer {
				depth--
				if depth == 0 {
					if l.pos-l.width > l.start {
						l.backup()
						l.emit(contentTok)
						l.next()
					}
					l.ignore() // consume closer
					l.emit(endTok)
					return checkInterpStack
				}
				continue
			}
		} else {
			if r == closer {
				if l.pos-l.width > l.start {
					l.backup()
					l.emit(contentTok)
					l.next()
				}
				l.ignore() // consume closer
				l.emit(endTok)
				return checkInterpStack
			}
		}
		if r == '#' {
			p := l.peek()
			if p == '{' {
				if l.pos-l.width > l.start {
					l.backup()
					l.emit(contentTok)
					l.next()
				}
				l.next() // consume {
				l.emit(token.EMBEXPR_BEG)
				l.interpStack = append(l.interpStack, interpState{stateFn: resumeFn, braceDepth: l.braceDepth})
				l.braceDepth = 1
				return startLexer
			}
			if p == '@' || p == '$' {
				if l.pos-l.width > l.start {
					l.backup()
					l.emit(contentTok)
					l.next()
				}
				l.ignore() // skip #
				l.interpStack = append(l.interpStack, interpState{stateFn: resumeFn, returnOnNext: true, braceDepth: l.braceDepth})
				if p == '@' {
					l.next()
					if l.peek() == '@' {
						l.next()
						l.emit(token.CLASS_VAR)
					} else {
						l.emit(token.AT)
					}
					return lexIdentifier
				}
				l.next() // consume $
				return lexGlobal
			}
		}
	}
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
	l.ignore() // consume opening `
	l.emit(token.XSTR_BEG)
	return lexBacktickContent
}

// lexBacktickContent scans through a backtick command string, emitting
// XSTR_CONTENT for literal segments and handling #{...} / #$var / #@var.
func lexBacktickContent(l *Lexer) StateFn {
	for {
		r := l.next()
		switch r {
		case '`':
			if l.pos-l.width > l.start {
				l.backup()
				l.emit(token.XSTR_CONTENT)
				l.next() // re-consume the closing `
			}
			l.emit(token.XSTR_END)
			return checkInterpStack
		case '#':
			p := l.peek()
			if p == '{' {
				if l.pos-l.width > l.start {
					l.backup()
					l.emit(token.XSTR_CONTENT)
					l.next() // re-consume #
				}
				l.next() // consume {
				l.emit(token.EMBEXPR_BEG)
				l.interpStack = append(l.interpStack, interpState{stateFn: lexBacktickContent, braceDepth: l.braceDepth})
				l.braceDepth = 1
				return startLexer
			}
			if p == '@' || p == '$' {
				if l.pos-l.width > l.start {
					l.backup()
					l.emit(token.XSTR_CONTENT)
					l.next() // re-consume #
				}
				l.ignore() // skip #
				l.interpStack = append(l.interpStack, interpState{stateFn: lexBacktickContent, returnOnNext: true, braceDepth: l.braceDepth})
				if p == '@' {
					l.next()
					if l.peek() == '@' {
						l.next()
						l.emit(token.CLASS_VAR)
					} else {
						l.emit(token.AT)
					}
					return lexIdentifier
				}
				l.next() // consume $
				return lexGlobal
			}
		case '\\':
			l.consumeEscape()
		case eof:
			return l.errorf("unterminated command literal")
		}
	}
}

// lexHeredocStart reads the heredoc delimiter and transitions to body lexing.
func lexHeredocStart(l *Lexer, indent, squig bool) StateFn {
	l.heredocIndent = indent
	l.heredocSquig = squig
	l.heredocQuote = 0
	l.ignore() // consume the << or <<- or <<~

	// Check for quoted delimiter: <<"EOS", <<'EOS', <<`EOS`
	p := l.peek()
	if p == '"' || p == '\'' || p == '`' {
		l.heredocQuote = p
		l.next()
	}

	// Read delimiter word.
	l.heredocDelim = ""
	for {
		r := l.peek()
		if r == eof || r == '\n' {
			break
		}
		if l.heredocQuote != 0 && r == l.heredocQuote {
			l.next()
			break
		}
		if l.heredocQuote == 0 && !isLetter(r) && !isDigit(r) && r != '_' {
			break
		}
		l.next()
		l.heredocDelim += string(r)
	}

	// Capture rest-of-line (including the trailing \n) and splice it out, so
	// the body lexer sees the heredoc body immediately after a single \n.
	// The captured bytes are re-injected after STRING_END / STRING -- this
	// handles trailers (<<EOS.chop) and chained heredocs (<<A, <<B) uniformly.
	restStart := l.pos
	nlPos := restStart
	for nlPos < len(l.input) && l.input[nlPos] != '\n' {
		nlPos++
	}
	if nlPos < len(l.input) {
		l.heredocPostBody = l.input[restStart : nlPos+1]
		l.input = l.input[:restStart] + "\n" + l.input[nlPos+1:]
	} else {
		l.heredocPostBody = l.input[restStart:nlPos]
		l.input = l.input[:restStart]
	}
	// Consume the inserted \n (or hit eof on unterminated input).
	for {
		r := l.next()
		if r == eof || r == '\n' {
			break
		}
	}
	l.ignore()
	// Literal heredocs (<<'EOS') are a single STRING token.
	// Interpolating heredocs emit STRING_BEG + STRING_CONTENT segments.
	if l.heredocQuote == '\'' {
		return lexHeredocBody
	}
	if l.heredocSquig {
		// Pre-strip indentation so the interp lexer sees normalised content.
		stripSquigInterpBody(l)
	}
	if l.heredocQuote == '`' {
		l.emit(token.XSTR_BEG)
	} else {
		l.emit(token.STRING_BEG)
	}
	return lexHeredocContent
}

// stripSquigInterpBody rewrites l.input to remove the minimum common leading
// whitespace from the heredoc body lines, so an interpolating squiggy heredoc
// (<<~) can be lexed by lexHeredocContent without indent-aware emission. Body
// runs from l.pos to the line whose content equals heredocDelim.
func stripSquigInterpBody(l *Lexer) {
	delim := l.heredocDelim
	bodyStart := l.pos
	pos := bodyStart
	type lineInfo struct {
		start  int
		indent int
		blank  bool
	}
	var lines []lineInfo
	minIndent := -1
	delimLineStart := -1
	delimLineWS := 0
	for pos < len(l.input) {
		lineStart := pos
		i := pos
		for i < len(l.input) && (l.input[i] == ' ' || l.input[i] == '\t') {
			i++
		}
		indent := i - lineStart
		eol := i
		for eol < len(l.input) && l.input[eol] != '\n' {
			eol++
		}
		content := l.input[i:eol]
		if content == delim {
			delimLineStart = lineStart
			delimLineWS = indent
			break
		}
		blank := content == ""
		lines = append(lines, lineInfo{lineStart, indent, blank})
		if !blank {
			if minIndent < 0 || indent < minIndent {
				minIndent = indent
			}
		}
		if eol >= len(l.input) {
			return
		}
		pos = eol + 1
	}
	if delimLineStart < 0 {
		return
	}
	if minIndent < 0 {
		minIndent = 0
	}
	var b strings.Builder
	b.Grow(delimLineStart - bodyStart)
	for idx, ln := range lines {
		var lineEnd int
		if idx+1 < len(lines) {
			lineEnd = lines[idx+1].start
		} else {
			lineEnd = delimLineStart
		}
		strip := minIndent
		if ln.blank && strip > ln.indent {
			strip = ln.indent
		}
		b.WriteString(l.input[ln.start+strip : lineEnd])
	}
	stripped := b.String()
	delimLineContentStart := delimLineStart + delimLineWS
	l.input = l.input[:bodyStart] + stripped + l.input[delimLineContentStart:]
}

// matchHeredocDelimLine reports whether the line beginning at pos is the
// closing delimiter line. On match, returns the position past the delim text
// (caller still needs to consume the trailing \n / line tail).
func matchHeredocDelimLine(l *Lexer, pos int) (int, bool) {
	if l.heredocIndent {
		for pos < len(l.input) && (l.input[pos] == ' ' || l.input[pos] == '\t') {
			pos++
		}
	}
	end := pos + len(l.heredocDelim)
	if end > len(l.input) || l.input[pos:end] != l.heredocDelim {
		return 0, false
	}
	if end < len(l.input) && l.input[end] != '\n' {
		return 0, false
	}
	return end, true
}

// lexHeredocBody reads a literal (non-interpolating) heredoc body until
// the delimiter appears at line start.
func lexHeredocBody(l *Lexer) StateFn {
	delim := l.heredocDelim
	// Empty body: delim line is the first body line.
	if _, ok := matchHeredocDelimLine(l, l.pos); ok {
		if l.heredocSquig {
			tok := token.NewToken(token.STRING, "", l.start)
			l.lastToken = tok
			l.tokens <- tok
			l.start = l.pos
		} else {
			l.emit(token.STRING)
		}
		for {
			r := l.next()
			if r == eof || r == '\n' {
				break
			}
		}
		l.ignore()
		l.heredocDelim = ""
		if l.heredocPostBody != "" {
			l.input = l.input[:l.pos] + l.heredocPostBody + l.input[l.pos:]
			l.heredocPostBody = ""
		}
		return startLexer
	}
	for {
		r := l.next()
		if r == eof {
			return l.errorf("unterminated heredoc")
		}
		if r == '\n' {
			contentEnd := l.pos

			r2 := l.next()
			if r2 == eof {
				return l.errorf("unterminated heredoc")
			}
			if l.heredocIndent {
				for r2 == ' ' || r2 == '\t' {
					r2 = l.next()
					if r2 == eof {
						return l.errorf("unterminated heredoc")
					}
				}
			}

			matched := true
			for i := 0; i < len(delim); i++ {
				if r2 != rune(delim[i]) {
					matched = false
					break
				}
				if i < len(delim)-1 {
					r2 = l.next()
					if r2 == eof {
						return l.errorf("unterminated heredoc")
					}
				}
			}

			if matched {
				after := l.pos
				l.pos = contentEnd
				if l.heredocSquig {
					content := stripHeredocIndent(l.input[l.start:l.pos])
					tok := token.NewToken(token.STRING, content, l.start)
					l.lastToken = tok
					l.tokens <- tok
					l.start = l.pos
				} else {
					l.emit(token.STRING)
				}
				l.pos = after
				// Consume rest of delimiter line (handles <<EOS.chop etc.)
				for {
					r := l.next()
					if r == eof || r == '\n' {
						break
					}
				}
				l.ignore()
				l.heredocDelim = ""
				if l.heredocPostBody != "" {
					l.input = l.input[:l.pos] + l.heredocPostBody + l.input[l.pos:]
					l.heredocPostBody = ""
				}
				return startLexer
			}

			l.pos = contentEnd
		}
	}
}

// lexHeredocContent reads an interpolating heredoc body, emitting
// STRING_CONTENT (or XSTR_CONTENT for backtick heredocs) for literal
// segments and handling #{...} / #$var / #@var.
func lexHeredocContent(l *Lexer) StateFn {
	contentTok := token.STRING_CONTENT
	endTok := token.STRING_END
	if l.heredocQuote == '`' {
		contentTok = token.XSTR_CONTENT
		endTok = token.XSTR_END
	}
	delim := l.heredocDelim
	// Empty body: delim line is the first body line.
	if _, ok := matchHeredocDelimLine(l, l.pos); ok {
		l.emit(contentTok)
		for {
			r := l.next()
			if r == eof || r == '\n' {
				break
			}
		}
		l.ignore()
		l.emit(endTok)
		l.heredocDelim = ""
		if l.heredocPostBody != "" {
			l.input = l.input[:l.pos] + l.heredocPostBody + l.input[l.pos:]
			l.heredocPostBody = ""
		}
		return startLexer
	}
	for {
		r := l.next()
		if r == eof {
			return l.errorf("unterminated heredoc")
		}
		if r == '\n' {
			lineStart := l.pos // position just after \n
			r2 := l.next()
			if r2 == eof {
				return l.errorf("unterminated heredoc")
			}
			if l.heredocIndent {
				for r2 == ' ' || r2 == '\t' {
					r2 = l.next()
					if r2 == eof {
						return l.errorf("unterminated heredoc")
					}
				}
			}
			matched := true
			for i := 0; i < len(delim); i++ {
				if r2 != rune(delim[i]) {
					matched = false
					break
				}
				if i < len(delim)-1 {
					r2 = l.next()
					if r2 == eof {
						return l.errorf("unterminated heredoc")
					}
				}
			}
			if matched {
				if lineStart > l.start {
					after := l.pos
					l.pos = lineStart
					l.emit(contentTok)
					l.pos = after
				} else {
					l.pos = lineStart
					l.ignore()
				}
				// Consume rest of delimiter line.
				for {
					r := l.next()
					if r == eof || r == '\n' {
						break
					}
				}
				l.ignore()
				l.emit(endTok)
				l.heredocDelim = ""
				if l.heredocPostBody != "" {
					l.input = l.input[:l.pos] + l.heredocPostBody + l.input[l.pos:]
					l.heredocPostBody = ""
				}
				return startLexer
			}
			// Not a delimiter -- rewind so content includes the full line.
			l.pos = lineStart
			continue
		}
		if r == '#' {
			p := l.peek()
			if p == '{' {
				if l.pos-l.width > l.start {
					l.backup()
					l.emit(contentTok)
					l.next()
				}
				l.next() // consume {
				l.emit(token.EMBEXPR_BEG)
				l.interpStack = append(l.interpStack, interpState{stateFn: lexHeredocContent, braceDepth: l.braceDepth})
				l.braceDepth = 1
				return startLexer
			}
			if p == '@' || p == '$' {
				if l.pos-l.width > l.start {
					l.backup()
					l.emit(contentTok)
					l.next()
				}
				l.ignore() // skip #
				l.interpStack = append(l.interpStack, interpState{stateFn: lexHeredocContent, returnOnNext: true, braceDepth: l.braceDepth})
				if p == '@' {
					l.next()
					if l.peek() == '@' {
						l.next()
						l.emit(token.CLASS_VAR)
					} else {
						l.emit(token.AT)
					}
					return lexIdentifier
				}
				l.next() // consume $
				return lexGlobal
			}
		}
		if r == '\\' {
			l.consumeEscape()
		}
	}
}

// stripHeredocIndent removes the minimum common leading whitespace from
// non-blank lines in a squiggy heredoc (<<~).
func stripHeredocIndent(s string) string {
	lines := strings.Split(s, "\n")
	minIndent := -1
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		indent := 0
		for _, ch := range line {
			if ch == ' ' || ch == '\t' {
				indent++
			} else {
				break
			}
		}
		if minIndent == -1 || indent < minIndent {
			minIndent = indent
		}
	}
	if minIndent <= 0 {
		return s
	}
	for i, line := range lines {
		if len(line) >= minIndent {
			lines[i] = line[minIndent:]
		}
	}
	return strings.Join(lines, "\n")
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

func isPercentTypeChar(r rune) bool {
	switch r {
	case 'q', 'Q', 'w', 'W', 'i', 'I', 'r', 'x', 's':
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
	case token.ILLEGAL, token.EOF, token.NEWLINE,
		token.ASSIGN, token.MATCH, token.NMATCH,
		token.LPAREN, token.LBRACKET, token.LBRACE,
		token.COMMA, token.SEMICOLON, token.COLON, token.QMARK,
		token.BANG, token.TILDE,
		token.LOGICALAND, token.LOGICALOR, token.PIPE,
		token.IF, token.UNLESS, token.WHILE, token.UNTIL, token.RETURN, token.THEN,
		token.DO, token.CASE, token.WHEN, token.BREAK, token.NEXT,
		token.KW_AND, token.KW_OR, token.KW_NOT, token.KW_DEFINED, token.KW_SUPER,
		token.RANGE, token.RANGEEX,
		token.HASHROCKET:
		return true
	}
	return false
}

func lexRegexBegin(l *Lexer) StateFn {
	l.ignore() // consume opening /
	l.emit(token.REGEX_BEG)
	return lexRegexContent
}

// lexRegexContent scans through a regex literal, emitting STRING_CONTENT
// for literal segments and handling #{...} / #$var / #@var interpolation.
func lexRegexContent(l *Lexer) StateFn {
	for {
		r := l.next()
		switch r {
		case '/':
			if l.pos-l.width > l.start {
				l.backup()
				l.emit(token.STRING_CONTENT)
				l.next() // re-consume /
			}
			// Consume regex options (i, m, x, o, u, n, e, s)
			opts := ""
			for {
				p := l.peek()
				if p == 'i' || p == 'm' || p == 'x' || p == 'o' ||
					p == 'u' || p == 'n' || p == 'e' || p == 's' {
					l.next()
					opts += string(p)
				} else {
					break
				}
			}
			tok := token.NewToken(token.REGEX_END, opts, l.start)
			l.lastToken = tok
			l.tokens <- tok
			l.start = l.pos
			return checkInterpStack
		case '#':
			p := l.peek()
			if p == '{' {
				if l.pos-l.width > l.start {
					l.backup()
					l.emit(token.STRING_CONTENT)
					l.next()
				}
				l.next() // consume {
				l.emit(token.EMBEXPR_BEG)
				l.interpStack = append(l.interpStack, interpState{stateFn: lexRegexContent, braceDepth: l.braceDepth})
				l.braceDepth = 1
				return startLexer
			}
			if p == '@' || p == '$' {
				if l.pos-l.width > l.start {
					l.backup()
					l.emit(token.STRING_CONTENT)
					l.next()
				}
				l.ignore() // skip #
				l.interpStack = append(l.interpStack, interpState{stateFn: lexRegexContent, returnOnNext: true, braceDepth: l.braceDepth})
				if p == '@' {
					l.next()
					if l.peek() == '@' {
						l.next()
						l.emit(token.CLASS_VAR)
					} else {
						l.emit(token.AT)
					}
					return lexIdentifier
				}
				l.next() // consume $
				return lexGlobal
			}
		case '\\':
			l.consumeEscape()
		case eof:
			return l.errorf("unterminated regexp")
		}
	}
}

func isWhitespace(r rune) bool {
	return unicode.IsSpace(r) && r != '\n'
}

func isLetter(r rune) bool {
	if r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
		return true
	}
	return r > 127 && unicode.IsLetter(r)
}

func isDigit(r rune) bool {
	return '0' <= r && r <= '9'
}

func isDigitOrUnderscore(r rune) bool {
	return isDigit(r) || r == '_'
}

// isIdentChar reports whether r is a valid identifier continuation character.
// Ruby identifiers are [a-zA-Z_][a-zA-Z0-9_]*[?!]? for the ASCII subset.
// Non-ASCII continuation includes letters, marks (combining diacritics,
// vowel signs, virama), and decimal digits from other scripts.
func isIdentChar(r rune) bool {
	if r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') {
		return true
	}
	if r <= 127 {
		return false
	}
	return unicode.IsLetter(r) || unicode.IsMark(r) || unicode.Is(unicode.Nd, r)
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
