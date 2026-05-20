package lexer

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
	"unsafe"

	"github.com/lczyk/goruby/token"
)

// unsafeBytesToString returns a string view over b without copying. The
// returned string aliases b's backing storage, so b must not be mutated
// while the string is in use. Used by NewBytes to avoid a redundant copy.
func unsafeBytesToString(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return unsafe.String(unsafe.SliceData(b), len(b))
}

const (
	eof = -1
)

var (
	ruby20 = token.MustParseVersion("2.0")
	ruby21 = token.MustParseVersion("2.1")
	ruby23 = token.MustParseVersion("2.3")
	ruby26 = token.MustParseVersion("2.6")
	ruby27 = token.MustParseVersion("2.7")
	ruby40 = token.MustParseVersion("4.0")
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

	// Heredoc state captured at push time. If the interpolation contains a
	// nested heredoc (e.g. `<<OUTER ... #{<<INNER ... INNER} ... OUTER`),
	// the inner heredoc clobbers the lexer's heredoc fields, so the outer
	// heredoc body lexer would fail to find its closing delimiter on pop.
	// Restored on pop so the outer heredoc resumes with its own state.
	heredocDelim    string
	heredocIndent   bool
	heredocSquig    bool
	heredocStripped bool
	heredocQuote    rune
	heredocRest     segment
}

// pushInterp pushes an interpState, capturing the current heredoc-related
// lexer fields. If the interpolation body starts a nested heredoc (e.g.
// `<<OUTER ... #{<<INNER ... INNER} ... OUTER`), the inner heredoc clobbers
// the lexer's heredoc fields, so without snapshotting the outer heredoc
// body lexer would fail to find its closing delimiter on pop.
// lastTokIsValue reports whether the previously emitted token can stand on
// the LHS of an infix operator (so `&` after it is bitwise AND, not block-pass).
func lastTokIsValue(t token.Type) bool {
	switch t {
	case token.IDENT, token.CONST, token.INT, token.FLOAT, token.STRING,
		token.NIL, token.TRUE, token.FALSE, token.SELF,
		token.CLASS_VAR, token.GLOBAL,
		token.RPAREN, token.RBRACKET, token.RBRACE,
		token.END, token.STRING_END:
		return true
	}
	return false
}

func (l *Lexer) pushInterp(s interpState) {
	s.heredocDelim = l.heredocDelim
	s.heredocIndent = l.heredocIndent
	s.heredocSquig = l.heredocSquig
	s.heredocStripped = l.heredocStripped
	s.heredocQuote = l.heredocQuote
	s.heredocRest = l.heredocRest
	l.interpStack = append(l.interpStack, s)
}

// restoreHeredocState writes the saved heredoc fields back to the lexer.
func (l *Lexer) restoreHeredocState(s interpState) {
	l.heredocDelim = s.heredocDelim
	l.heredocIndent = s.heredocIndent
	l.heredocSquig = s.heredocSquig
	l.heredocStripped = s.heredocStripped
	l.heredocQuote = s.heredocQuote
	l.heredocRest = s.heredocRest
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
		segEnd: len(input),
		state:  startLexer,
		tokens: make([]token.Token, 0, 16),
	}
	for _, o := range opts {
		o(l)
	}
	l.hasMagicEncoding = detectMagicEncoding(input)
	if l.version.IsSet() && !l.version.AtLeast(ruby20) && !l.hasMagicEncoding &&
		hasNonAsciiOutsideComments(input) {
		l.invalidEncoding = true
	}
	return l
}

// NewBytes is like New but accepts a []byte without copying. The lexer
// holds a string view over the byte slice -- callers MUST NOT mutate the
// backing array while the Lexer is in use. Use this from hot paths (e.g.
// parser bootstrap) to avoid the byte->string copy that string(b) would
// otherwise allocate.
//
// Internally l.input is never mutated post-construction (the heredoc
// cursor model in HEREDOC_PLAN.md eliminated all splices), so the
// zero-copy view is safe for the lexer's own reads.
func NewBytes(input []byte, opts ...Option) *Lexer {
	return New(unsafeBytesToString(input), opts...)
}

// detectMagicEncoding reports whether the source has a `# coding:` /
// `# encoding:` magic comment on the first or second line. MRI 1.9
// defaults source encoding to US-ASCII without one, rejecting any
// non-ASCII byte outside comments.
func detectMagicEncoding(input string) bool {
	for i, line := 0, 0; line < 2 && i < len(input); line++ {
		end := strings.IndexByte(input[i:], '\n')
		if end < 0 {
			end = len(input) - i
		}
		raw := input[i : i+end]
		i += end + 1
		// Magic comment must be on a line that's only a comment.
		trimmed := strings.TrimLeft(raw, " \t")
		if !strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.Contains(trimmed, "coding:") || strings.Contains(trimmed, "coding=") {
			return true
		}
	}
	return false
}

// hasNonAsciiOutsideComments reports whether input contains any byte
// >=0x80 outside of `#`-line comments. Used to mirror MRI 1.9's
// rejection of non-ASCII source when no magic comment is present.
// Per line: any non-ASCII byte appearing before the first `#` on the
// line counts as code -- bytes after `#` are assumed to be inline
// comment text and ignored. False-negatives possible on `#` inside
// string literals, but the simpler scan is good enough for the test
// fixtures that drive this gate.
func hasNonAsciiOutsideComments(input string) bool {
	i := 0
	for i < len(input) {
		nl := strings.IndexByte(input[i:], '\n')
		end := len(input)
		if nl >= 0 {
			end = i + nl
		}
		// Anything from `#` onwards is a comment for this line.
		hash := strings.IndexByte(input[i:end], '#')
		stop := end
		if hash >= 0 {
			stop = i + hash
		}
		for k := i; k < stop; k++ {
			if input[k] >= 0x80 {
				return true
			}
		}
		if nl < 0 {
			return false
		}
		i = end + 1
	}
	return false
}

// segment denotes a contiguous range of l.input to be consumed by the lexer.
// Used by the cursor mechanism (see segEnd / pending below) so heredoc paths
// can splice in rest-of-line content WITHOUT mutating l.input.
type segment struct {
	start, end int
}

// Lexer is the engine to process input and emit Tokens
type Lexer struct {
	input              string           // the string being scanned (immutable post-New).
	state              StateFn          // the next lexing function to enter
	pos                int              // current position in the input.
	segEnd             int              // end of current view of input; when pos reaches segEnd, pop pending
	pending            []segment        // upcoming ranges of input to consume after current view exhausts
	start              int              // start position of this item.
	width              int              // width of last rune read from input.
	tokens             []token.Token // queue of scanned tokens, drained by NextToken.
	tokenHead          int           // index of next unread token in tokens.
	litPool            []string         // per-token literal pool indexed by Token.LitOff (LitLen unused with []string indexing)
	lastToken          token.Token      // lastToken stores the last token emitted by the lexer
	hadWhitespace      bool             // true if whitespace was skipped before current token
	ternaryDepth       int              // pending ternary ? without matching :
	version            token.RubyVersion
	hasMagicEncoding   bool // source has a `# coding:` / `# encoding:` magic comment
	invalidEncoding    bool // version is <2.0, no magic comment, source has non-ASCII outside comments
	tokenHadWhitespace bool // whitespace before the token currently being lexed

	// Heredoc state.
	heredocDelim    string
	heredocIndent   bool // <<-
	heredocSquig    bool // <<~
	heredocStripped bool // <<~ source had a positive common indent that was stripped
	heredocQuote    rune
	heredocRest segment // range in l.input holding rest-of-line; queued onto
	// l.pending at body-end so the lexer reads it after STRING_END.

	// Squig body buffer swap (phase 5). When a squig heredoc is set up,
	// rather than splicing the stripped body into l.input we build a small
	// freestanding buffer of "stripped_body + delim + \n" and TEMPORARILY
	// swap it in as l.input for the duration of body lex. At STRING_END,
	// finishHeredoc restores the original l.input and resumes at
	// heredocSquigRestorePos (= position past delim line in the original).
	heredocSwapped         bool   // true while l.input points at the stripped buffer
	heredocSavedInput      string // original l.input, restored on body end
	heredocSavedSegEnd     int    // original l.segEnd, restored on body end
	heredocSquigRestorePos int    // position in original l.input to resume at

	// Interpolation state.
	braceDepth  int
	interpStack []interpState
}

// NextToken returns the next token from the input. When the lexer is exhausted
// (state is nil and the token buffer is drained), it returns token.EOF. Safe
// to call without checking HasNext first -- it will keep returning EOF.
func (l *Lexer) NextToken() token.Token {
	for {
		if l.tokenHead < len(l.tokens) {
			tok := l.tokens[l.tokenHead]
			l.tokenHead++
			// Drained -- reset both indices so the backing array gets reused
			// instead of growing unboundedly across long inputs.
			if l.tokenHead == len(l.tokens) {
				l.tokens = l.tokens[:0]
				l.tokenHead = 0
			}
			return tok
		}
		if l.state == nil {
			return token.NewToken(token.EOF, "", l.pos)
		}
		l.state = l.state(l)
	}
}

// HasNext returns true if there are tokens left (including buffered tokens
// emitted before the state machine stopped), false if the input is exhausted.
func (l *Lexer) HasNext() bool {
	return l.state != nil || l.tokenHead < len(l.tokens)
}

// Input returns the source text being scanned. Immutable post-New. Used by
// the parser to resolve token spans via Token.LitOf(src) during parse.
func (l *Lexer) Input() string { return l.input }

// Pool returns the lexer's literal pool: per-token-text storage for the
// emitLiteral path (escape-decoded STRING_CONTENT, percent-prefix
// STRING_BEG, heredoc tags, regex flags, error messages, etc). Source-
// slice tokens carry LitOff = -1 and don't appear here -- consumers
// resolve their text via tok.LitOf(src). ast.Program holds the pool
// reference post-parse so AST queries that need decoded literals
// continue to work after src is dropped.
func (l *Lexer) Pool() []string { return l.litPool }

// Lit returns the literal text for tok. For pool-backed tokens it returns
// pool[LitOff] (zero-alloc). For source-slice tokens it falls back to
// tok.LitOf(l.input) -- callers that may read mid-heredoc-swap (where
// l.input is a transient buffer) should pass their own cached source
// string to tok.LitOf directly instead.
func (l *Lexer) Lit(tok token.Token) string {
	if tok.LitOff >= 0 && int(tok.LitOff) < len(l.litPool) {
		return l.litPool[tok.LitOff]
	}
	return tok.LitOf(l.input)
}

// newToken builds a source-slice token. Pos/End refer to the source span
// l.start..l.pos. LitOff = -1 marks the token as not having an entry in
// l.litPool -- consumers reconstruct the text via tok.LitOf(src).
//
// During heredoc body lex (l.heredocSwapped) the lexer's input has been
// swapped to a transient buffer; tokens emitted with Pos pointing into
// that buffer are unreadable once the swap is restored. To keep their
// text reachable, the bytes are captured into the pool right away (the
// token becomes a pool-backed one via newTokenLit).
func (l *Lexer) newToken(t token.Type) token.Token {
	if l.heredocSwapped {
		return l.newTokenLit(t, l.input[l.start:l.pos])
	}
	return token.Token{
		Type:   t,
		Pos:    l.start,
		End:    int32(l.pos - l.start),
		LitOff: -1,
	}
}

// newTokenLit builds a token whose literal text doesn't match its source
// span (escape-decoded STRING_CONTENT, percent-prefix STRING_BEG, heredoc
// tag, regex flags, error message). The literal is appended to l.litPool
// and Token.LitOff indexes into it. End remains the source span length.
func (l *Lexer) newTokenLit(t token.Type, literal string) token.Token {
	off := int32(len(l.litPool))
	l.litPool = append(l.litPool, literal)
	return token.Token{
		Type:   t,
		Pos:    l.start,
		End:    int32(l.pos - l.start),
		LitOff: off,
	}
}

// emit passes a token back to the client.
func (l *Lexer) emit(t token.Type) {
	tok := l.newToken(t)
	tok.HadWhitespace = l.tokenHadWhitespace
	if t == token.STRING_END || t == token.XSTR_END {
		tok.HeredocStripped = l.heredocStripped
		l.heredocStripped = false
	}
	l.tokenHadWhitespace = false
	l.lastToken = tok
	l.tokens = append(l.tokens, tok)
	l.start = l.pos
}

// emitLiteral emits a token of the given type with an explicit literal,
// ignoring the input between l.start and l.pos. start is advanced to pos.
func (l *Lexer) emitLiteral(t token.Type, literal string) {
	tok := l.newTokenLit(t, literal)
	tok.HadWhitespace = l.tokenHadWhitespace
	if t == token.STRING_END || t == token.XSTR_END {
		tok.HeredocStripped = l.heredocStripped
		l.heredocStripped = false
	}
	l.tokenHadWhitespace = false
	l.lastToken = tok
	l.tokens = append(l.tokens, tok)
	l.start = l.pos
}

// emitLiteralSQ is emitLiteral but marks the token as SingleQuoted so the
// printer renders it with single quotes.
func (l *Lexer) emitLiteralSQ(t token.Type, literal string) {
	tok := l.newTokenLit(t, literal)
	tok.HadWhitespace = l.tokenHadWhitespace
	tok.SingleQuoted = true
	l.tokenHadWhitespace = false
	l.lastToken = tok
	l.tokens = append(l.tokens, tok)
	l.start = l.pos
}

// advanceSegment pops the next pending segment, jumping l.pos / l.segEnd to
// its range. Resets l.start so tokens never span a segment boundary. Returns
// false (and leaves pos at segEnd) when no pending segments remain.
func (l *Lexer) advanceSegment() bool {
	if len(l.pending) == 0 {
		return false
	}
	seg := l.pending[0]
	l.pending = l.pending[1:]
	l.pos = seg.start
	l.segEnd = seg.end
	l.start = l.pos
	return true
}

// byteAt returns the byte at logical offset from l.pos, walking pending
// segments when the offset crosses l.segEnd. Used by lookahead paths that
// must respect cursor boundaries (otherwise they'd see "physical" bytes
// past segEnd that don't belong to the current logical stream).
func (l *Lexer) byteAt(off int) (byte, bool) {
	p := l.pos + off
	if p < l.segEnd && p < len(l.input) {
		return l.input[p], true
	}
	if p < l.segEnd {
		return 0, false
	}
	over := p - l.segEnd
	for _, seg := range l.pending {
		size := seg.end - seg.start
		if over < size {
			idx := seg.start + over
			if idx >= len(l.input) {
				return 0, false
			}
			return l.input[idx], true
		}
		over -= size
	}
	return 0, false
}

// finishHeredoc handles the transition at heredoc body end. Cursor mode
// (heredocRest set, phase 2+): queues the after-body continuation as the
// next pending segment and switches the view to the rest-of-line segment.
// Legacy splice mode (heredocPostBody non-empty): re-injects postBody into
// l.input. Either way, leaves the lexer positioned to consume rest-of-line
// content next, followed by what was after the heredoc.
func (l *Lexer) finishHeredoc() {
	l.heredocDelim = ""
	// Phase 5: if a squig body was lexed against the stripped buffer, restore
	// the original l.input first and reposition past the delim line in the
	// original source. Then the cursor / splice transitions below run on the
	// real input as if no swap had happened.
	if l.heredocSwapped {
		l.input = l.heredocSavedInput
		l.segEnd = l.heredocSavedSegEnd
		l.pos = l.heredocSquigRestorePos
		l.start = l.pos
		l.heredocSwapped = false
		l.heredocSavedInput = ""
		l.heredocSavedSegEnd = 0
		l.heredocSquigRestorePos = 0
	}
	if l.heredocRest != (segment{}) {
		rest := l.heredocRest
		l.heredocRest = segment{}
		// Push current continuation {l.pos, l.segEnd} to the front of pending
		// so it's consumed after the rest-of-line segment.
		l.pending = append([]segment{{l.pos, l.segEnd}}, l.pending...)
		l.pos = rest.start
		l.segEnd = rest.end
		l.start = l.pos
	}
}

// next returns the next rune in the input.
func (l *Lexer) next() rune {
	for l.pos >= l.segEnd {
		if !l.advanceSegment() {
			l.width = 0
			return eof
		}
	}
	if b := l.input[l.pos]; b < utf8.RuneSelf {
		l.width = 1
		l.pos++
		return rune(b)
	}
	var r rune
	r, l.width = utf8.DecodeRuneInString(l.input[l.pos:l.segEnd])
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
	if l.pos >= l.width {
		l.pos -= l.width
	} else {
		l.pos = 0
	}
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
	if l.pos >= l.segEnd {
		// Look ahead into the next non-empty pending segment without popping.
		for i := 0; i < len(l.pending); i++ {
			seg := l.pending[i]
			if seg.start >= seg.end {
				continue
			}
			if b := l.input[seg.start]; b < utf8.RuneSelf {
				return rune(b)
			}
			r, _ := utf8.DecodeRuneInString(l.input[seg.start:seg.end])
			return r
		}
		return eof
	}
	if b := l.input[l.pos]; b < utf8.RuneSelf {
		return rune(b)
	}
	r, _ := utf8.DecodeRuneInString(l.input[l.pos:l.segEnd])
	return r
}

// peekSecond returns the rune after the next rune, without consuming.
func (l *Lexer) peekSecond() rune {
	w := l.width
	l.next()
	w1 := l.width
	r := l.next()
	l.backup()
	l.width = w1
	l.backup()
	l.width = w
	return r
}

// error returns an error token and terminates the scan by passing
// back a nil pointer that will be the next state, terminating l.run.
func (l *Lexer) errorf(format string, args ...interface{}) StateFn {
	l.tokens = append(l.tokens, l.newTokenLit(token.ILLEGAL, fmt.Sprintf(format, args...)))
	return nil
}

func startLexer(l *Lexer) StateFn {
	if l.invalidEncoding {
		return l.errorf("invalid multibyte char: Ruby 1.9 requires a magic encoding comment for non-ASCII source")
	}
	r := l.next()
	if isWhitespace(r) {
		l.hadWhitespace = true
		l.ignore()
		return startLexer
	}
	hadWhitespace := l.hadWhitespace
	l.hadWhitespace = false
	l.tokenHadWhitespace = hadWhitespace
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
	if r == '=' && l.start == 0 && l.lastToken.Type == token.ILLEGAL && l.Lit(l.lastToken) == "" {
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
		// Lookahead respects cursor-segment boundaries via byteAt so that the
		// rest-of-line `\n` emitted at heredoc end doesn't peek into body
		// bytes that physically follow restEnd in l.input.
		off := 0
		for {
			c, ok := l.byteAt(off)
			if !ok || (c != ' ' && c != '\t') {
				break
			}
			off++
		}
		c0, ok0 := l.byteAt(off)
		c1, ok1 := l.byteAt(off + 1)
		// Suppress NEWLINE before .method or &.method, but NOT before
		// .. or ... (range literals).
		if ok0 && c0 == '.' && ok1 && c1 != '.' {
			l.ignore()
			return startLexer
		}
		if ok0 && c0 == '&' && ok1 && c1 == '.' {
			l.ignore()
			return startLexer
		}
		// Trailing `or`/`and` keywords suppress NEWLINE: `x or\ny` -> `x or y`.
		if l.lastToken.Type == token.KW_AND || l.lastToken.Type == token.KW_OR {
			l.ignore()
			return startLexer
		}
		// Ruby 4.0+: leading logical operators as line continuation.
		// Only when version is explicitly set -- this changes existing behaviour.
		if l.version.IsSet() && l.version.AtLeast(ruby40) && ok0 {
			c2, _ := l.byteAt(off + 2)
			c3, _ := l.byteAt(off + 3)
			if (c0 == '|' && c1 == '|') ||
				(c0 == '&' && c1 == '&') ||
				(c0 == 'o' && c1 == 'r' && (c2 == ' ' || c2 == '\t')) ||
				(c0 == 'a' && c1 == 'n' && c2 == 'd' && (c3 == ' ' || c3 == '\t')) {
				l.ignore()
				return startLexer
			}
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
			if l.ternaryDepth > 0 {
				l.ternaryDepth--
			}
			l.emit(token.COLON)
			return startLexer
		}
		if l.ternaryDepth > 0 && isTernaryContext(l.lastToken.Type) {
			l.ternaryDepth--
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
		if isWhitespace(p) || isTernaryContext(l.lastToken.Type) {
			l.ternaryDepth++
			l.emit(token.QMARK)
			return startLexer
		}
		if isExpressionDelimiter(p) {
			l.ignore()
			return l.errorf("invalid character syntax; use ?%q", p)
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
		if hadWhitespace && isMethodCallTarget(l.lastToken.Type) {
			p := l.peek()
			if p != ' ' && p != '\t' && p != '\n' && p != '\r' && p != eof {
				return lexRegexBegin
			}
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
		// Percent literal vs binary modulo: literal only legal where a fresh
		// expression can start. After an operand (string, ident value, `)` ...)
		// `%` is binary mod; the exception is a spaced %literal as a method arg
		// (`foo %w[a b]`), allowed only when the prior token is a call target.
		regexCtx := isRegexBeginContext(l.lastToken.Type)
		methodArgCtx := hadWhitespace && isMethodCallTarget(l.lastToken.Type)
		if regexCtx || methodArgCtx {
			// %q, %Q, %w, %W, %i, %I, %r, %x, %s -- typed literal w/ delim.
			if p := l.peek(); isPercentTypeChar(p) {
				delim := l.peekSecond()
				if !isLetter(delim) && !isDigit(delim) && !isWhitespace(delim) && delim != eof {
					return lexPercentLiteral
				}
			}
		}
		if regexCtx {
			// bare % literal in regex-begin context (e.g. `= %{str}`, after
			// `(`, `,`, etc. -- delim shape decided downstream).
			return lexPercentLiteral
		}
		if methodArgCtx && isPercentDelimiter(l.peek()) {
			// bare % literal as method arg (`foo %{str}`).
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
			if !l.version.AtLeast(ruby23) {
				// Lonely operator `&.` arrived in 2.3; older MRI rejects.
				l.next()
				return l.errorf("safe-navigation operator `&.` requires Ruby 2.3+")
			}
			l.next()
			l.emit(token.LONELY)
			return startLexer
		}
		if l.peek() == '=' {
			l.next()
			l.emit(token.ANDASSIGN_BITWISE)
			return startLexer
		}
		// CAPTURE (block-pass &foo / &:sym) vs AND (bitwise infix).
		// MRI's rule (paraphrased): `&` is block-pass when followed by
		// a valid operand-start (letter, `:` for symbol, `@`/`$` for
		// ivars/globals) with no whitespace after, AND either there's
		// no value-like token before it (start of expression) OR there
		// IS whitespace before it (so `a &b` is `a(&b)`, but `a&b` and
		// `a & b` are infix).
		if p := l.peek(); isLetter(p) || p == ':' || p == '@' || p == '$' {
			prevValue := lastTokIsValue(l.lastToken.Type)
			if !prevValue || l.tokenHadWhitespace {
				l.emit(token.CAPTURE)
				return startLexer
			}
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
			// After value-producing tokens that can't be method names,
			// << is always left-shift. After IDENT/CONST without
			// whitespace (e.g. @ar<<X), it's also left-shift.
			if l.lastToken.Type == token.CLASS || isHeredocBlockedContext(l.lastToken.Type) {
				l.emit(token.LSHIFT)
				return startLexer
			}
			if !hadWhitespace && isMethodCallTarget(l.lastToken.Type) {
				l.emit(token.LSHIFT)
				return startLexer
			}
			p := l.peek()
			if p == '-' || p == '~' {
				if p == '~' && !l.version.AtLeast(ruby23) {
					l.emit(token.LSHIFT)
					return startLexer
				}
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
				l.restoreHeredocState(top)
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
		if l.lastToken.Type == token.DEF || l.lastToken.Type == token.DOT || l.lastToken.Type == token.SYMBEG {
			l.emitLiteral(token.IDENT, "`")
			return startLexer
		}
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
			saved := *top
			l.interpStack = l.interpStack[:len(l.interpStack)-1]
			l.restoreHeredocState(saved)
			return saved.stateFn
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
		// Exception: `!=` is the not-equal operator; `foo!=x` is
		// `foo != x`, not setter `foo!=` on receiver. Don't absorb `!`
		// when followed by `=` (unless it's `==` which would belong to
		// `!==`, not a valid token -- but `!=` then `=` would be `!==`
		// which Ruby treats as `!=` then `=`, so still want to leave it).
		if r == '?' || r == '!' {
			if r == '!' && l.peek() == '=' {
				l.backup()
				break
			}
			// already part of the identifier
		} else if r != eof {
			l.backup()
		}
		break
	}
	// Label detection: IDENT immediately followed by : (no space) is a label key.
	// Avoid when the identifier ends with ? or ! (method names like valid?: are
	// not valid label keys), when the : is part of :: (scope resolution),
	// or when the identifier follows @ or @@ (instance/class variables).
	if l.peek() == ':' &&
		l.lastToken.Type != token.AT && l.lastToken.Type != token.CLASS_VAR &&
		l.lastToken.Type != token.DOT && l.lastToken.Type != token.LONELY {
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
			// Dangling e[+-] with no digit: MRI rejects as syntax error.
			l.next() // consume +/-
			return l.errorf("trailing '%c' in number", r)
		} else if isDigit(p) {
			l.next() // consume e/E
			return lexFloatExponent(l)
		} else if p == eof {
			// Dangling e at EOF: MRI rejects as syntax error.
			return l.errorf("trailing '%c' in number", r)
		}
	}
	// Rational or complex suffix (Ruby 2.1+). `ri` = rational+imaginary.
	if r == 'r' || r == 'i' {
		if l.version.AtLeast(ruby21) {
			if r == 'r' && l.peek() == 'i' {
				l.next() // consume the trailing `i`
			}
			l.emit(token.INT)
			return startLexer
		}
		return l.errorf("rational/complex literal suffix requires Ruby 2.1+")
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
			if isDigit(l.peekSecond()) {
				l.next()
				l.next()
				return lexFloatExponent(l)
			}
			// Dangling e[+-] with no digit: MRI rejects as syntax error.
			l.next()
			return l.errorf("trailing '%c' in number", r)
		} else if isDigit(p) {
			l.next()
			return lexFloatExponent(l)
		}
		if p == eof {
			// Dangling e at EOF: MRI rejects as syntax error.
			return l.errorf("trailing '%c' in number", r)
		}
	}
	// Optional rational/complex suffix (Ruby 2.1+). `ri` = rational+imaginary.
	if r == 'r' || r == 'i' {
		if l.version.AtLeast(ruby21) {
			if r == 'r' && l.peek() == 'i' {
				l.next() // consume the trailing `i`
			}
			l.next()
			l.backup()
			l.emit(token.FLOAT)
			return startLexer
		}
		return l.errorf("rational/complex literal suffix requires Ruby 2.1+")
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
	// Optional rational/complex suffix after exponent (Ruby 2.1+).
	// `ri` = rational+imaginary.
	if r == 'r' || r == 'i' {
		if l.version.AtLeast(ruby21) {
			if r == 'r' && l.peek() == 'i' {
				l.next()
			}
			l.next()
			l.backup()
			l.emit(token.FLOAT)
			return startLexer
		}
		return l.errorf("rational/complex literal suffix requires Ruby 2.1+")
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
	tok := l.newToken(token.STRING)
	tok.HadWhitespace = l.tokenHadWhitespace
	tok.SingleQuoted = true
	l.tokenHadWhitespace = false
	l.lastToken = tok
	l.tokens = append(l.tokens, tok)
	l.start = l.pos
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
			} else {
				for i := 0; i < 4; i++ {
					if isHexDigit(l.peek()) {
						l.next()
					}
				}
			}
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
		case '0', '1', '2', '3', '4', '5', '6', '7':
			for i := 0; i < 2; i++ {
				if p := l.peek(); p >= '0' && p <= '7' {
					l.next()
				}
			}
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
	// After the char/escape, emit the character as a string. Mark with
	// IsCharLit so the printer can re-emit as `?X` (preserves MRI's
	// StringFlags shape -- char literals always inherit source encoding).
	tok := l.newToken(token.STRING)
	tok.HadWhitespace = l.tokenHadWhitespace
	tok.IsCharLit = true
	l.tokenHadWhitespace = false
	l.lastToken = tok
	l.tokens = append(l.tokens, tok)
	l.start = l.pos
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
				l.pushInterp(interpState{stateFn: lexStringContent, braceDepth: l.braceDepth})
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
				l.pushInterp(interpState{stateFn: lexStringContent, returnOnNext: true, braceDepth: l.braceDepth})
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
		// $-x globals: $-w, $-v, $-d, $-0, etc. consume one more char.
		if r == '-' {
			p := l.peek()
			if isLetter(p) || isDigit(p) {
				l.next()
			}
		}
		// Numbered capture globals: $1, $12, $123 -- consume all digits.
		if isDigit(r) {
			for isDigit(l.peek()) {
				l.next()
			}
		}
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

	if (typ == 'i' || typ == 'I') && !l.version.AtLeast(ruby20) {
		return l.errorf("%%i{} literals require Ruby 2.0+")
	}

	switch typ {
	case 'q':
		l.ignore() // skip type char and opener
		return lexPercentLiteralBodySQ(l, opener, closer, paired, token.STRING)
	case 'w', 'i', 's':
		l.emitLiteralSQ(token.STRING_BEG, string(typ))
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

// lexPercentLiteralBodySQ is lexPercentLiteralBody but marks the emitted
// STRING token as SingleQuoted so the printer renders %q[...] as '...'.
func lexPercentLiteralBodySQ(l *Lexer, opener, closer rune, paired bool, tok token.Type) StateFn {
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
			l.next()
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
					sq := l.newToken(tok)
					sq.HadWhitespace = l.tokenHadWhitespace
					sq.SingleQuoted = true
					l.tokenHadWhitespace = false
					l.lastToken = sq
					l.tokens = append(l.tokens, sq)
					l.start = l.pos
					l.next()
					l.ignore()
					return startLexer
				}
				continue
			}
		} else {
			if r == closer {
				l.backup()
				sq := l.newToken(tok)
				sq.HadWhitespace = l.tokenHadWhitespace
				sq.SingleQuoted = true
				l.tokenHadWhitespace = false
				l.lastToken = sq
				l.tokens = append(l.tokens, sq)
				l.start = l.pos
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
	// Build content with delim escapes resolved -- MRI strips `\<delim>` to
	// just `<delim>` and `\\` to `\` inside %w/%i/%s bodies. Splitting and
	// downstream consumers see the stripped form.
	var buf []byte
	for {
		r := l.next()
		if r == eof {
			return l.errorf("unterminated percent literal")
		}
		if r == '\\' {
			nxt := l.next()
			if nxt == eof {
				return l.errorf("unterminated percent literal")
			}
			switch nxt {
			case '\\', opener, closer:
				buf = append(buf, byte(nxt))
			default:
				buf = append(buf, '\\')
				buf = utf8AppendRune(buf, nxt)
			}
			continue
		}
		if paired && r == opener {
			depth++
			buf = utf8AppendRune(buf, r)
			continue
		}
		if r == closer {
			if paired {
				depth--
				if depth > 0 {
					buf = utf8AppendRune(buf, r)
					continue
				}
			}
			if len(buf) > 0 {
				l.emitLiteralSQ(contentTok, string(buf))
			}
			l.ignore()
			l.emit(endTok)
			return startLexer
		}
		buf = utf8AppendRune(buf, r)
	}
}

func utf8AppendRune(b []byte, r rune) []byte {
	if r < 0x80 {
		return append(b, byte(r))
	}
	var tmp [4]byte
	n := utf8.EncodeRune(tmp[:], r)
	return append(b, tmp[:n]...)
}

// lexPercentContent scans through an interpolating percent literal body,
// emitting content tokens and handling #{...} / #$var / #@var interpolation.
func lexPercentContent(l *Lexer, opener, closer rune, paired bool,
	contentTok, endTok token.Type) StateFn {

	pDepth := new(int)
	if paired {
		*pDepth = 1
	}
	return lexPercentContentInner(l, opener, closer, paired, contentTok, endTok, pDepth)
}

func lexPercentContentInner(l *Lexer, opener, closer rune, paired bool,
	contentTok, endTok token.Type, pDepth *int) StateFn {

	resumeFn := func(_ *Lexer) StateFn {
		return lexPercentContentInner(l, opener, closer, paired, contentTok, endTok, pDepth)
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
				*pDepth++
				continue
			}
			if r == closer {
				*pDepth--
				if *pDepth == 0 {
					if l.pos-l.width > l.start {
						l.backup()
						l.emit(contentTok)
						l.next()
					}
					l.ignore() // consume closer
					if endTok == token.REGEX_END {
						consumeRegexFlags(l)
					}
					l.emit(endTok)
					return checkInterpStack
				}
				continue
			}
		} else {
			if r == closer {
				if l.pos-l.width > l.start {
					l.backup()
					if endTok == token.REGEX_END {
						// MRI's %r with non-meta delim normalises `\<delim>`
						// in content to bare `<delim>` (escape of delimiter).
						raw := string(l.input[l.start:l.pos])
						norm := strings.ReplaceAll(raw, "\\"+string(closer), string(closer))
						l.emitLiteral(contentTok, norm)
					} else {
						l.emit(contentTok)
					}
					l.next()
				}
				l.ignore() // consume closer
				if endTok == token.REGEX_END {
					consumeRegexFlags(l)
				}
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
				l.pushInterp(interpState{stateFn: resumeFn, braceDepth: l.braceDepth})
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
				l.pushInterp(interpState{stateFn: resumeFn, returnOnNext: true, braceDepth: l.braceDepth})
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

// consumeRegexFlags consumes trailing regex option flags (i,m,x,o,u,n,e,s)
// and advances l.pos past them so the next emit() includes them in the token literal.
func consumeRegexFlags(l *Lexer) {
	for {
		p := l.peek()
		if p == 'i' || p == 'm' || p == 'x' || p == 'o' ||
			p == 'u' || p == 'n' || p == 'e' || p == 's' {
			l.next()
		} else {
			break
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
				l.pushInterp(interpState{stateFn: lexBacktickContent, braceDepth: l.braceDepth})
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
				l.pushInterp(interpState{stateFn: lexBacktickContent, returnOnNext: true, braceDepth: l.braceDepth})
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

func buildHeredocTag(indent, squig bool, quote rune, delim string) string {
	var b strings.Builder
	b.WriteString("<<")
	if squig {
		b.WriteByte('~')
	} else if indent {
		b.WriteByte('-')
	}
	if quote != 0 {
		b.WriteRune(quote)
	}
	b.WriteString(delim)
	if quote != 0 {
		b.WriteRune(quote)
	}
	return b.String()
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

	// Read delimiter word. Capture as a single slice of the input after the
	// loop instead of growing l.heredocDelim by O(n^2) per-rune concatenation.
	delimStart := l.pos
	delimEnd := delimStart
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
		delimEnd = l.pos
	}
	l.heredocDelim = l.input[delimStart:delimEnd]

	// Capture rest-of-line (including the trailing \n) and splice it out, so
	// the body lexer sees the heredoc body immediately after a single \n.
	// The captured bytes are re-injected after STRING_END / STRING -- this
	// handles trailers (<<EOS.chop) and chained heredocs (<<A, <<B) uniformly.
	//
	// Inside interpolation (#{}), the rest-of-line may contain the closing }
	// and further interpolation. We must stop the capture at the unmatched }
	// so the interpolation state can process it.
	restStart := l.pos
	nlPos := restStart
	braces := 0
	inInterp := len(l.interpStack) > 0
	// Scan within the current cursor segment so nested heredocs on rest-of-
	// line of an outer heredoc see the segment-trailing \n. inInterp still
	// uses the splice path and needs full-input scope to find the unmatched
	// } that bounds the rest-of-line capture.
	scanEnd := l.segEnd
	if inInterp {
		scanEnd = len(l.input)
	}
	for nlPos < scanEnd && l.input[nlPos] != '\n' {
		if inInterp {
			switch l.input[nlPos] {
			case '{':
				braces++
			case '}':
				if braces == 0 {
					goto foundEnd
				}
				braces--
			}
		}
		nlPos++
	}
foundEnd:
	if nlPos < l.segEnd && l.input[nlPos] == '\n' {
		// Queue rest-of-line as a pending segment to be lexed after
		// STRING_END. Jump l.pos directly to body start; no input mutation.
		l.heredocRest = segment{restStart, nlPos + 1}
		if nlPos+1 == l.segEnd && len(l.pending) > 0 {
			// Nested heredoc: the \n is at the end of the current segment
			// (e.g. trailing \n of an outer heredoc's rest-of-line). Body
			// source is the next pending segment (the outer heredoc's
			// after-body continuation).
			body := l.pending[0]
			l.pending = l.pending[1:]
			l.pos = body.start
			l.segEnd = body.end
		} else {
			l.pos = nlPos + 1
		}
		l.start = l.pos
	} else if inInterp && nlPos < len(l.input) && l.input[nlPos] == '}' {
		// Inside interpolation: rest-of-line stops at the unmatched }. Scan
		// past } to find the real \n that ends the source line containing
		// `<<EOS`. heredocRest captures `[restStart, realNl+1]` (includes
		// the }, the rest of outer-string content, and the \n). Body source
		// is at realNl+1 in original input -- unless that position sits at
		// the current segment's boundary, in which case body lives in the
		// next pending segment (nested-heredoc-inside-outer-rest-of-line).
		realNl := nlPos
		for realNl < len(l.input) && l.input[realNl] != '\n' {
			realNl++
		}
		if realNl >= len(l.input) {
			return l.errorf("unterminated heredoc")
		}
		l.heredocRest = segment{restStart, realNl + 1}
		if realNl+1 >= l.segEnd && len(l.pending) > 0 {
			body := l.pending[0]
			l.pending = l.pending[1:]
			l.pos = body.start
			l.segEnd = body.end
		} else {
			l.pos = realNl + 1
		}
		l.start = l.pos
	} else {
		// No \n and no } -- unterminated heredoc (eof). Set up an empty
		// rest segment + body cursor at end-of-input so STRING_BEG is still
		// emitted (matches previous behaviour) and body lex errors out
		// immediately with "unterminated heredoc".
		l.heredocRest = segment{restStart, restStart}
		l.pos = nlPos
		l.start = l.pos
		l.segEnd = nlPos
	}
	// All heredocs (including single-quoted) emit STRING_BEG + STRING_CONTENT + STRING_END.
	if l.heredocQuote == '\'' {
		if l.heredocSquig {
			setupSquigBodyBuffer(l)
		}
		tag := buildHeredocTag(l.heredocIndent, l.heredocSquig, '\'', l.heredocDelim)
		l.emitLiteral(token.STRING_BEG, tag)
		return lexHeredocBody
	}
	if l.heredocSquig {
		// Pre-strip indentation so the interp lexer sees normalised content.
		setupSquigBodyBuffer(l)
	}
	tag := buildHeredocTag(l.heredocIndent, l.heredocSquig, l.heredocQuote, l.heredocDelim)
	if l.heredocQuote == '`' {
		l.emitLiteral(token.XSTR_BEG, tag)
	} else {
		l.emitLiteral(token.STRING_BEG, tag)
	}
	return lexHeredocContent
}

// setupSquigBodyBuffer builds a stripped-body buffer for a squig heredoc
// (<<~) and temporarily swaps l.input to it for the duration of body lex.
// Body runs from l.pos to the line whose content equals heredocDelim.
//
// Phase 5 of the heredoc cursor migration: the original implementation
// spliced the stripped body into l.input, allocating a fresh full-input
// string per heredoc (~570MB total on the real-files bench). The buffer-
// swap variant allocates only the stripped body plus delim+\n (a small
// constant overhead per heredoc) and routes body lex through that buffer.
// finishHeredoc restores l.input to the saved original when body ends.
func setupSquigBodyBuffer(l *Lexer) {
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
	if minIndent > 0 {
		l.heredocStripped = true
	}
	var b strings.Builder
	// Stripped body + delim text + (optional \n). matchHeredocDelimLine
	// needs the delim text present at the tail of the buffer to terminate
	// the body lex naturally.
	delimLineContentStart := delimLineStart + delimLineWS
	delimEnd := delimLineContentStart + len(delim)
	hasNewline := delimEnd < len(l.input) && l.input[delimEnd] == '\n'
	bodyApprox := delimLineStart - bodyStart
	b.Grow(bodyApprox + len(delim) + 1)
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
	b.WriteString(delim)
	if hasNewline {
		b.WriteByte('\n')
	}
	buf := b.String()

	// Compute the resume position in the ORIGINAL input -- one byte past the
	// delim line (the \n if present, or end-of-input).
	restorePos := delimEnd
	if hasNewline {
		restorePos++
	}

	// Save original lexer state, swap l.input to the stripped buffer for
	// body lex. finishHeredoc restores when STRING_END is emitted.
	l.heredocSwapped = true
	l.heredocSavedInput = l.input
	l.heredocSavedSegEnd = l.segEnd
	l.heredocSquigRestorePos = restorePos
	l.input = buf
	l.pos = 0
	l.start = 0
	l.segEnd = len(buf)
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
// the delimiter appears at line start. Emits STRING_CONTENT + STRING_END.
func lexHeredocBody(l *Lexer) StateFn {
	delim := l.heredocDelim
	// Empty body: delim line is the first body line.
	if _, ok := matchHeredocDelimLine(l, l.pos); ok {
		l.emitLiteral(token.STRING_CONTENT, "")
		l.emitLiteral(token.STRING_END, "")
		for {
			r := l.next()
			if r == eof || r == '\n' {
				break
			}
		}
		l.ignore()
		l.finishHeredoc()
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
			// Require the delim to occupy the rest of the line: peek the
			// next char and ensure it's `\n` or eof. Without this check, a
			// body line like `};#1` matches the `};` delim by prefix and
			// terminates the heredoc early, swallowing later content.
			if matched {
				peek := l.peek()
				if peek != '\n' && peek != eof {
					matched = false
				}
			}

			if matched {
				after := l.pos
				l.pos = contentEnd
				content := l.input[l.start:l.pos]
				l.emitLiteral(token.STRING_CONTENT, content)
				l.emitLiteral(token.STRING_END, "")
				l.pos = after
				// Consume rest of delimiter line (handles <<EOS.chop etc.)
				for {
					r := l.next()
					if r == eof || r == '\n' {
						break
					}
				}
				l.ignore()
				l.finishHeredoc()
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
		l.finishHeredoc()
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
			if len(delim) == 0 {
				matched = r2 == '\n' || r2 == eof
			} else {
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
				// Consume rest of delimiter line (for non-empty delimiters,
				// skip any trailing chars after the delimiter word).
				if len(delim) > 0 {
					for {
						r := l.next()
						if r == eof || r == '\n' {
							break
						}
					}
				}
				l.ignore()
				l.emit(endTok)
				l.finishHeredoc()
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
				l.pushInterp(interpState{stateFn: lexHeredocContent, braceDepth: l.braceDepth})
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
				l.pushInterp(interpState{stateFn: lexHeredocContent, returnOnNext: true, braceDepth: l.braceDepth})
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
		token.ADDASSIGN, token.SUBASSIGN, token.MULASSIGN,
		token.DIVASSIGN, token.MODASSIGN, token.POWERASSIGN,
		token.ORASSIGN, token.ANDASSIGN,
		token.LSHIFTASSIGN, token.RSHIFTASSIGN,
		token.ANDASSIGN_BITWISE, token.ORASSIGN_BITWISE, token.XORASSIGN,
		token.LPAREN, token.LBRACKET, token.LBRACE,
		token.COMMA, token.SEMICOLON, token.COLON, token.QMARK,
		token.BANG, token.TILDE,
		token.PLUS, token.MINUS, token.ASTERISK, token.POWER, token.MODULO,
		token.SLASH,
		token.LT, token.GT, token.LTE, token.GTE, token.EQ, token.NOTEQ,
		token.SPACESHIP, token.CASEEQ,
		token.LSHIFT, token.RSHIFT,
		token.AND, token.XOR,
		token.LOGICALAND, token.LOGICALOR, token.PIPE,
		token.IF, token.UNLESS, token.WHILE, token.UNTIL, token.RETURN, token.THEN,
		token.DO, token.CASE, token.WHEN, token.BREAK, token.NEXT,
		token.YIELD, token.BEGIN, token.RESCUE, token.KW_ENSURE, token.ELSE, token.KW_ELSIF,
		token.KW_AND, token.KW_OR, token.KW_NOT, token.KW_DEFINED, token.KW_SUPER,
		token.KW_IN, token.KW_FOR,
		token.RANGE, token.RANGEEX,
		token.HASHROCKET, token.EMBEXPR_BEG,
		token.LABEL:
		return true
	}
	return false
}

func isTernaryContext(tok token.Type) bool {
	switch tok {
	case token.IDENT, token.CONST, token.GLOBAL, token.CLASS_VAR,
		token.INT, token.FLOAT, token.STRING, token.REGEX,
		token.XSTR,
		token.RPAREN, token.RBRACKET, token.RBRACE,
		token.SELF, token.NIL, token.TRUE, token.FALSE,
		token.STRING_END, token.REGEX_END, token.END:
		return true
	}
	return false
}

func isPercentDelimiter(r rune) bool {
	return r != eof && r != '\n' && !isLetter(r) && !isDigit(r) && !isWhitespace(r)
}

func isHeredocBlockedContext(tok token.Type) bool {
	switch tok {
	case token.GLOBAL, token.CLASS_VAR,
		token.INT, token.FLOAT, token.STRING, token.REGEX,
		token.XSTR,
		token.RPAREN, token.RBRACKET, token.RBRACE,
		token.SELF, token.NIL, token.TRUE, token.FALSE,
		token.STRING_END, token.REGEX_END:
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
			tok := l.newTokenLit(token.REGEX_END, opts)
			l.lastToken = tok
			l.tokens = append(l.tokens, tok)
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
				l.pushInterp(interpState{stateFn: lexRegexContent, braceDepth: l.braceDepth})
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
				l.pushInterp(interpState{stateFn: lexRegexContent, returnOnNext: true, braceDepth: l.braceDepth})
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
