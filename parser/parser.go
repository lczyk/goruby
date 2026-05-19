package parser

import (
	"context"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/lczyk/trace"
	"github.com/lczyk/goruby/ast"
	"github.com/lczyk/goruby/lexer"
	"github.com/lczyk/goruby/token"
	"github.com/pkg/errors"
)

var (
	ruby20 = token.MustParseVersion("2.0")
	ruby21 = token.MustParseVersion("2.1")
	ruby25 = token.MustParseVersion("2.5")
	ruby26 = token.MustParseVersion("2.6")
	ruby27 = token.MustParseVersion("2.7")
	ruby30 = token.MustParseVersion("3.0")
	ruby31 = token.MustParseVersion("3.1")
)

// Possible precendece values
const (
	_ int = iota
	precLowest
	precBlockDo     // do
	precBlockBraces // { |x| }
	precIfUnless    // modifier-if, modifier-unless
	precKwAndOr     // and, or (keywords) -- below =, ?:, .. -- binds looser than `||` / `&&`
	precComma       // , in expression lists
	precAssignment  // x = 5
	precTenary      // ?, :
	precRange       // .., ... (above ?: but below ||)
	precLogicalOr   // ||
	precLogicalAnd  // &&
	precEquals      // ==, !=, <=>
	precLessGreater // >, <, >=, <=
	precOr          // |
	precAnd         // &
	precShift       // <<
	precSum         // + or -
	precProduct     // *, /, %
	precPrefix      // -X or !X
	precPower       // ** (right-associative, binds tighter than unary)
	precCallArg     // func x
	precCall        // foo.myFunction(X)
	precIndex       // array[index]
	precScope       // A::B
	precCapture     // &block
	precSymbol      // :Symbol
	precHighest
)

var precedences = map[token.Type]int{
	token.RESCUE:            precIfUnless,
	token.IF:                precIfUnless,
	token.UNLESS:            precIfUnless,
	token.WHILE:             precIfUnless,
	token.UNTIL:             precIfUnless,
	token.EQ:                precEquals,
	token.NOTEQ:             precEquals,
	token.MATCH:             precEquals,
	token.NMATCH:            precEquals,
	token.SPACESHIP:         precEquals,
	token.LSHIFT:            precShift,
	token.RSHIFT:            precShift,
	token.CASEEQ:            precEquals,
	token.QMARK:             precTenary,
	token.COLON:             precTenary,
	token.LT:                precLessGreater,
	token.GT:                precLessGreater,
	token.LTE:               precLessGreater,
	token.GTE:               precLessGreater,
	token.PLUS:              precSum,
	token.MINUS:             precSum,
	token.SLASH:             precProduct,
	token.ASTERISK:          precProduct,
	token.MODULO:            precProduct,
	token.ASSIGN:            precAssignment,
	token.ADDASSIGN:         precAssignment,
	token.SUBASSIGN:         precAssignment,
	token.MULASSIGN:         precAssignment,
	token.DIVASSIGN:         precAssignment,
	token.MODASSIGN:         precAssignment,
	token.LPAREN:            precCall,
	token.DOT:               precCall,
	token.IDENT:             precCallArg,
	token.CONST:             precCallArg,
	token.GLOBAL:            precCallArg,
	token.INT:               precCallArg,
	token.FLOAT:             precCallArg,
	token.STRING:            precCallArg,
	token.STRING_BEG:        precCallArg,
	token.XSTR_BEG:          precCallArg,
	token.REGEX_BEG:         precCallArg,
	token.REGEX:             precCallArg,
	token.XSTR:              precCallArg,
	token.SELF:              precCallArg,
	token.LAMBDA:            precCallArg,
	token.LBRACKET:          precIndex,
	token.LBRACE:            precBlockBraces,
	token.DO:                precBlockDo,
	token.SCOPE:             precScope,
	token.SYMBEG:            precSymbol,
	token.HASHROCKET:        precAssignment,
	token.COMMA:             precComma,
	token.KW_IN:             precAssignment,
	token.THEN:              precHighest,
	token.NEWLINE:           precHighest,
	token.PIPE:              precOr,
	token.XOR:               precOr,
	token.AND:               precAnd,
	token.LOGICALOR:         precLogicalOr,
	token.LOGICALAND:        precLogicalAnd,
	token.LABEL:             precCallArg,
	token.CLASS_VAR:         precCallArg,
	token.AT:                precCallArg,
	token.KW_DEFINED:        precCallArg,
	token.BANG:              precCallArg,
	token.NIL:                precCallArg,
	token.TRUE:               precCallArg,
	token.FALSE:              precCallArg,
	token.KEYWORD__FILE__:    precCallArg,
	token.KEYWORD__LINE__:    precCallArg,
	token.KEYWORD__METHOD__:  precCallArg,
	token.KEYWORD__DIR__:     precCallArg,
	token.KEYWORD__ENCODING__: precCallArg,
	token.KEYWORD__CALLEE__:  precCallArg,
	token.CAPTURE:           precCapture,
	token.POWER:             precPower,
	token.RANGE:             precRange,
	token.RANGEEX:           precRange,
	token.LONELY:            precCall,
	token.KW_AND:            precKwAndOr,
	token.KW_OR:             precKwAndOr,
	token.POWERASSIGN:       precAssignment,
	token.ORASSIGN:          precAssignment,
	token.ANDASSIGN:         precAssignment,
	token.LSHIFTASSIGN:      precAssignment,
	token.RSHIFTASSIGN:      precAssignment,
	token.ANDASSIGN_BITWISE: precAssignment,
	token.ORASSIGN_BITWISE:  precAssignment,
	token.XORASSIGN:         precAssignment,
}

var tokensNotPossibleInCallArgs = []token.Type{
	token.ASSIGN,
	token.ADDASSIGN,
	token.SUBASSIGN,
	token.MULASSIGN,
	token.DIVASSIGN,
	token.MODASSIGN,
	token.POWERASSIGN,
	token.ORASSIGN,
	token.ANDASSIGN,
	token.LSHIFTASSIGN,
	token.RSHIFTASSIGN,
	token.ANDASSIGN_BITWISE,
	token.ORASSIGN_BITWISE,
	token.XORASSIGN,
	token.LT,
	token.LTE,
	token.GT,
	token.GTE,
	token.SPACESHIP,
	token.LSHIFT,
	token.RSHIFT,
	token.CASEEQ,
	token.EQ,
	token.NOTEQ,
	token.MATCH,
	token.NMATCH,
	token.IF,
	token.UNLESS,
	token.WHILE,
	token.UNTIL,
	token.RESCUE,
	token.COLON,
	token.QMARK,
	token.RBRACKET,
	token.COMMA,
	token.THEN,
	token.HASHROCKET,
	token.KW_AND,
	token.KW_OR,
	token.LOGICALAND,
	token.LOGICALOR,
	token.SLASH,
	token.MODULO,
	token.PIPE,
	token.AND,
	token.XOR,
	token.RANGE,
	token.RANGEEX,
}

type (
	prefixParseFn func(*parser) ast.Expression
	infixParseFn  func(*parser, ast.Expression) ast.Expression
)

// Parse-fn dispatch tables. Populated once in package init via method
// expressions so each ParseFile pays zero registration / closure-alloc cost.
var (
	prefixParseFns [token.TypeMax + 1]prefixParseFn
	infixParseFns  [token.TypeMax + 1]infixParseFn
)

var defaultExpressionTerminators = []token.Type{
	token.SEMICOLON,
	token.NEWLINE,
}

// A parser parses the token emitted by the provided lexer.Lexer and returns an
// AST describing the parsed program.
type parser struct {
	file    *token.File
	l       *lexer.Lexer
	errors  []error
	version token.RubyVersion

	// Tracing/debugging
	mode Mode // parsing mode
	ctx  context.Context

	pos        token.Pos
	curToken   token.Token
	peekToken  token.Token
	peek2Token token.Token

	inPattern          bool // true when parsing a pattern matching clause
	suppressDoBlock    bool // true inside while/until/for conditions
	suppressHashrocket bool // true in call-arg lists to prevent => as rightward assignment
	suppressKwAndOr    bool // true in paren-less call args -- `foo x and y` is `foo(x) and y`
	embExprDepth       int  // depth of `#{...}` interpolation nesting; heredocs constructed here need the chain-quirk workaround
	comments           []*ast.Comment
}

func (p *parser) init(filename string, src []byte, mode Mode) {
	p.file = token.NewFile(filename, len(src))

	p.l = lexer.New(string(src), lexer.WithVersion(p.version))
	p.errors = []error{}

	p.mode = mode
	if p.ctx == nil {
		p.ctx = context.Background()
		if mode&Trace != 0 {
			p.ctx = trace.WithTracer(p.ctx, trace.NewTracer())
		}
	}

	// Bootstrap three-token lookahead window.
	p.peekToken = p.nextNonCommentToken()
	p.peek2Token = p.nextNonCommentToken()
	p.nextToken()
}

func init() {
	prefixParseFns[token.ILLEGAL] = (*parser).parseIllegal
	prefixParseFns[token.IDENT] = (*parser).parseIdentifier
	prefixParseFns[token.CONST] = (*parser).parseIdentifier
	prefixParseFns[token.AT] = (*parser).parseInstanceVariable
	prefixParseFns[token.INT] = (*parser).parseIntegerLiteral
	prefixParseFns[token.FLOAT] = (*parser).parseFloatLiteral
	prefixParseFns[token.STRING] = (*parser).parseStringLiteral
	prefixParseFns[token.STRING_BEG] = (*parser).parseInterpolatedString
	prefixParseFns[token.XSTR_BEG] = (*parser).parseInterpolatedString
	prefixParseFns[token.REGEX_BEG] = (*parser).parseInterpolatedRegex
	prefixParseFns[token.REGEX] = (*parser).parseStringLiteral
	prefixParseFns[token.XSTR] = (*parser).parseStringLiteral
	prefixParseFns[token.BANG] = (*parser).parsePrefixExpression
	prefixParseFns[token.PLUS] = (*parser).parsePrefixExpression
	prefixParseFns[token.MINUS] = (*parser).parsePrefixExpression
	prefixParseFns[token.ASTERISK] = (*parser).parseSplatExpression
	prefixParseFns[token.POWER] = (*parser).parseSplatExpression
	prefixParseFns[token.TILDE] = (*parser).parsePrefixExpression
	prefixParseFns[token.LOGICALAND] = (*parser).parsePrefixExpression
	prefixParseFns[token.LOGICALOR] = (*parser).parsePrefixExpression
	prefixParseFns[token.TRUE] = (*parser).parseBoolean
	prefixParseFns[token.FALSE] = (*parser).parseBoolean
	prefixParseFns[token.LPAREN] = (*parser).parseGroupedExpression
	prefixParseFns[token.IF] = (*parser).parseIfExpression
	prefixParseFns[token.UNLESS] = (*parser).parseIfExpression
	prefixParseFns[token.WHILE] = (*parser).parseLoopExpression
	prefixParseFns[token.UNTIL] = (*parser).parseLoopExpression
	prefixParseFns[token.KW_FOR] = (*parser).parseLoopExpression
	prefixParseFns[token.CASE] = (*parser).parseCaseExpression
	prefixParseFns[token.WHEN] = (*parser).parseErrorSkip // when outside case is an error
	prefixParseFns[token.ELSE] = (*parser).parseErrorSkip // else outside if/case is an error
	prefixParseFns[token.KW_ELSIF] = (*parser).parseErrorSkip
	prefixParseFns[token.KW_IN] = (*parser).parseErrorSkip
	prefixParseFns[token.BREAK] = (*parser).parseJumpExpression
	prefixParseFns[token.NEXT] = (*parser).parseJumpExpression
	prefixParseFns[token.KW_REDO] = (*parser).parseJumpExpression
	prefixParseFns[token.KW_RETRY] = (*parser).parseJumpExpression
	prefixParseFns[token.KW_ENSURE] = (*parser).parseErrorSkip
	prefixParseFns[token.DEF] = (*parser).parseFunctionLiteral
	prefixParseFns[token.SCOPE] = (*parser).parseTopLevelScope
	prefixParseFns[token.LABEL] = (*parser).parseLabelExpression
	prefixParseFns[token.SYMBEG] = (*parser).parseSymbolLiteral
	prefixParseFns[token.LBRACKET] = (*parser).parseArrayLiteral
	prefixParseFns[token.NIL] = (*parser).parseNilLiteral
	prefixParseFns[token.SELF] = (*parser).parseSelf
	prefixParseFns[token.MODULE] = (*parser).parseModule
	prefixParseFns[token.CLASS] = (*parser).parseClass
	prefixParseFns[token.LBRACE] = (*parser).parseHash
	prefixParseFns[token.DO] = (*parser).parseBlock
	prefixParseFns[token.YIELD] = (*parser).parseYield
	prefixParseFns[token.GLOBAL] = (*parser).parseGlobal
	prefixParseFns[token.KEYWORD__FILE__] = (*parser).parseKeyword__FILE__
	prefixParseFns[token.KEYWORD__LINE__] = (*parser).parseKeyword__LINE__
	prefixParseFns[token.KEYWORD__ENCODING__] = (*parser).parseEncodingKeyword
	prefixParseFns[token.KEYWORD__DIR__] = (*parser).parseKeyword__DIR__
	prefixParseFns[token.KW_BEGIN] = (*parser).parseBeginBlock
	prefixParseFns[token.KW_END] = (*parser).parseEndBlock
	prefixParseFns[token.KW_USING] = (*parser).parseUsing
	prefixParseFns[token.KW_REFINE] = (*parser).parseRefine
	prefixParseFns[token.KEYWORD__CALLEE__] = (*parser).parseKeyword__CALLEE__
	prefixParseFns[token.KEYWORD__METHOD__] = (*parser).parseKeyword__METHOD__
	prefixParseFns[token.RPAREN] = (*parser).parseErrorSkip
	prefixParseFns[token.RBRACKET] = (*parser).parseErrorSkip
	prefixParseFns[token.RBRACE] = (*parser).parseErrorSkip
	prefixParseFns[token.NEWLINE] = (*parser).parseErrorSkip
	prefixParseFns[token.EMBEXPR_END] = (*parser).parseErrorSkip
	prefixParseFns[token.HASHROCKET] = (*parser).parseErrorSkip
	prefixParseFns[token.RESCUE] = (*parser).parseExceptionHandlingBlock
	prefixParseFns[token.BEGIN] = (*parser).parseExceptionHandlingBlock
	prefixParseFns[token.CLASS_VAR] = (*parser).parseClassVariable
	prefixParseFns[token.AND] = (*parser).parseBlockCapture // &:to_s, &block
	prefixParseFns[token.CAPTURE] = (*parser).parseBlockCapture
	prefixParseFns[token.KW_SUPER] = (*parser).parseSuper
	prefixParseFns[token.KW_UNDEF] = (*parser).parseUndef
	prefixParseFns[token.KW_NOT] = (*parser).parsePrefixExpression
	prefixParseFns[token.KW_DEFINED] = (*parser).parseDefinedExpression
	prefixParseFns[token.RETURN] = (*parser).parseReturnExpression
	prefixParseFns[token.KW_ALIAS] = (*parser).parseAlias
	prefixParseFns[token.LAMBDA] = (*parser).parseLambda
	prefixParseFns[token.RANGE] = (*parser).parseBeginlessRange
	prefixParseFns[token.RANGEEX] = (*parser).parseRangeOrForwarding

	infixParseFns[token.PLUS] = (*parser).parseInfixExpression
	infixParseFns[token.MINUS] = (*parser).parseInfixExpression
	infixParseFns[token.SLASH] = (*parser).parseInfixExpression
	infixParseFns[token.ASTERISK] = (*parser).parseInfixExpression
	infixParseFns[token.MODULO] = (*parser).parseInfixExpression
	infixParseFns[token.AND] = (*parser).parseInfixExpression
	infixParseFns[token.PIPE] = (*parser).parseInfixExpression
	infixParseFns[token.XOR] = (*parser).parseInfixExpression
	infixParseFns[token.EQ] = (*parser).parseInfixExpression
	infixParseFns[token.NOTEQ] = (*parser).parseInfixExpression
	infixParseFns[token.LT] = (*parser).parseInfixExpression
	infixParseFns[token.GT] = (*parser).parseInfixExpression
	infixParseFns[token.LTE] = (*parser).parseInfixExpression
	infixParseFns[token.GTE] = (*parser).parseInfixExpression
	infixParseFns[token.LOGICALOR] = (*parser).parseInfixExpression
	infixParseFns[token.LOGICALAND] = (*parser).parseInfixExpression
	infixParseFns[token.MATCH] = (*parser).parseInfixExpression
	infixParseFns[token.NMATCH] = (*parser).parseInfixExpression
	infixParseFns[token.SPACESHIP] = (*parser).parseInfixExpression
	infixParseFns[token.LSHIFT] = (*parser).parseInfixExpression
	infixParseFns[token.RSHIFT] = (*parser).parseInfixExpression
	infixParseFns[token.CASEEQ] = (*parser).parseInfixExpression
	infixParseFns[token.HASHROCKET] = (*parser).parseRightwardAssignment
	infixParseFns[token.ASSIGN] = (*parser).parseAssignment
	infixParseFns[token.ADDASSIGN] = (*parser).parseAssignmentOperator
	infixParseFns[token.SUBASSIGN] = (*parser).parseAssignmentOperator
	infixParseFns[token.MULASSIGN] = (*parser).parseAssignmentOperator
	infixParseFns[token.DIVASSIGN] = (*parser).parseAssignmentOperator
	infixParseFns[token.MODASSIGN] = (*parser).parseAssignmentOperator
	infixParseFns[token.LSHIFTASSIGN] = (*parser).parseAssignmentOperator
	infixParseFns[token.RSHIFTASSIGN] = (*parser).parseAssignmentOperator
	infixParseFns[token.ANDASSIGN_BITWISE] = (*parser).parseAssignmentOperator
	infixParseFns[token.ORASSIGN_BITWISE] = (*parser).parseAssignmentOperator
	infixParseFns[token.XORASSIGN] = (*parser).parseAssignmentOperator
	infixParseFns[token.IF] = (*parser).parseModifierConditionalExpression
	infixParseFns[token.UNLESS] = (*parser).parseModifierConditionalExpression
	infixParseFns[token.WHILE] = (*parser).parseModifierLoopExpression
	infixParseFns[token.UNTIL] = (*parser).parseModifierLoopExpression
	infixParseFns[token.KW_IN] = (*parser).parseInfixExpression
	infixParseFns[token.QMARK] = (*parser).parseTenaryIfExpression
	infixParseFns[token.LPAREN] = (*parser).parseCallExpressionWithParens
	infixParseFns[token.IDENT] = (*parser).parseCallArgument
	infixParseFns[token.CONST] = (*parser).parseCallArgument
	infixParseFns[token.GLOBAL] = (*parser).parseCallArgument
	infixParseFns[token.INT] = (*parser).parseCallArgument
	infixParseFns[token.FLOAT] = (*parser).parseCallArgument
	infixParseFns[token.STRING] = (*parser).parseStringConcat
	infixParseFns[token.STRING_BEG] = (*parser).parseStringConcat
	infixParseFns[token.XSTR_BEG] = (*parser).parseCallArgument
	infixParseFns[token.REGEX_BEG] = (*parser).parseCallArgument
	infixParseFns[token.REGEX] = (*parser).parseCallArgument
	infixParseFns[token.XSTR] = (*parser).parseCallArgument
	infixParseFns[token.LABEL] = (*parser).parseCallArgument
	infixParseFns[token.SYMBEG] = (*parser).parseCallArgument
	infixParseFns[token.CLASS_VAR] = (*parser).parseCallArgument
	infixParseFns[token.CAPTURE] = (*parser).parseCallArgument
	infixParseFns[token.SELF] = (*parser).parseCallArgument
	infixParseFns[token.LAMBDA] = (*parser).parseCallArgument
	infixParseFns[token.AT] = (*parser).parseCallArgument
	infixParseFns[token.KW_DEFINED] = (*parser).parseCallArgument
	infixParseFns[token.BANG] = (*parser).parseCallArgument
	infixParseFns[token.NIL] = (*parser).parseCallArgument
	infixParseFns[token.TRUE] = (*parser).parseCallArgument
	infixParseFns[token.FALSE] = (*parser).parseCallArgument
	infixParseFns[token.KEYWORD__FILE__] = (*parser).parseCallArgument
	infixParseFns[token.KEYWORD__LINE__] = (*parser).parseCallArgument
	infixParseFns[token.KEYWORD__METHOD__] = (*parser).parseCallArgument
	infixParseFns[token.KEYWORD__DIR__] = (*parser).parseCallArgument
	infixParseFns[token.KEYWORD__ENCODING__] = (*parser).parseCallArgument
	infixParseFns[token.KEYWORD__CALLEE__] = (*parser).parseCallArgument
	infixParseFns[token.LBRACE] = (*parser).parseCallBlock
	infixParseFns[token.DO] = (*parser).parseCallBlock
	infixParseFns[token.DOT] = (*parser).parseMethodCall
	infixParseFns[token.COMMA] = (*parser).parseExpressions
	infixParseFns[token.LBRACKET] = (*parser).parseIndexExpression
	infixParseFns[token.POWER] = (*parser).parseInfixExpression
	infixParseFns[token.RANGE] = (*parser).parseInfixExpression
	infixParseFns[token.RANGEEX] = (*parser).parseInfixExpression
	infixParseFns[token.RESCUE] = (*parser).parseRescueModifier
	infixParseFns[token.LONELY] = (*parser).parseMethodCall
	infixParseFns[token.KW_AND] = (*parser).parseInfixExpression
	infixParseFns[token.KW_OR] = (*parser).parseInfixExpression
	infixParseFns[token.POWERASSIGN] = (*parser).parseAssignmentOperator
	infixParseFns[token.ORASSIGN] = (*parser).parseAssignmentOperator
	infixParseFns[token.ANDASSIGN] = (*parser).parseAssignmentOperator
	infixParseFns[token.SCOPE] = (*parser).parseScopedIdentifierExpression
}

func (p *parser) nextNonCommentToken() token.Token {
	if !p.l.HasNext() {
		return token.NewToken(token.EOF, "", -1)
	}
	tok := p.l.NextToken()
	for tok.Type == token.HASH {
		hashTok := tok
		var value string
		if p.l.HasNext() {
			strTok := p.l.NextToken()
			value = strTok.Literal
		}
		if p.mode&ParseComments != 0 {
			p.comments = append(p.comments, &ast.Comment{Token: hashTok, Value: value})
		}
		if p.l.HasNext() {
			tok = p.l.NextToken()
		} else {
			return token.NewToken(token.EOF, "", -1)
		}
	}
	return tok
}

func (p *parser) nextToken() {
	if p.pos.IsValid() && trace.IsTracing(p.ctx) {
		s := p.curToken.Type.String()
		switch {
		case p.curToken.IsLiteral():
			trace.MessageStrCtx(p.ctx, s+" "+p.curToken.Literal)
		case p.curToken.IsOperator(), p.curToken.IsKeyword():
			trace.MessageStrCtx(p.ctx, "\""+s+"\"")
		default:
			trace.MessageStrCtx(p.ctx, s)
		}
	}
	p.curToken = p.peekToken
	p.peekToken = p.peek2Token
	p.pos = p.file.Pos(p.curToken.Pos)
	if p.curToken.Type == token.NEWLINE {
		p.file.AddLine(p.curToken.Pos + 1)
	}
	p.peek2Token = p.nextNonCommentToken()
}

// Errors returns all errors which happened during the parsing of the input.
func (p *parser) Errors() []error {
	return p.errors
}

func (p *parser) peekError(t ...token.Type) {
	epos := p.file.Position(p.pos)
	err := &unexpectedTokenError{
		Pos:            epos,
		expectedTokens: t,
		actualToken:    p.peekToken.Type,
		actualLiteral:  p.peekToken.Literal,
	}
	p.errors = append(p.errors, errors.WithStack(err))
}

func (p *parser) expectError(t ...token.Type) {
	epos := p.file.Position(p.pos)
	err := &unexpectedTokenError{
		Pos:            epos,
		expectedTokens: t,
		actualToken:    p.curToken.Type,
		actualLiteral:  p.curToken.Literal,
	}
	p.errors = append(p.errors, errors.WithStack(err))
}

func (p *parser) noPrefixParseFnError(t token.Type) {
	p.errors = append(p.errors, &parseError{
		Pos:  p.file.Position(p.pos),
		Kind: SyntaxError,
		Msg:  fmt.Sprintf("no prefix parse function for type %s found", t),
	})
}

func (p *parser) versionError(minVer token.RubyVersion, feature string) {
	msg := fmt.Sprintf("%s requires ruby %s or later", feature, minVer)
	epos := p.file.Position(p.pos)
	if epos.Filename != "" || epos.IsValid() {
		msg = epos.String() + ": " + msg
	}
	p.errors = append(p.errors, errors.New(msg))
}

func (p *parser) spacedOperator(op, next token.Token) bool {
	return op.Pos+len(op.Literal) < next.Pos
}

// ParseProgram returns the parsed program AST and all errors which occured
// during the parse process. If the error is not nil the AST may be incomplete
// and callers should always check if they can handle the error with providing
// more input by checking with e.g. IsEOFError.
func (p *parser) ParseProgram() (*ast.Program, error) {
	defer trace.TraceCtx(p.ctx)()
	program := &ast.Program{}
	program.Statements = []ast.Statement{}
	for !p.currentTokenIs(token.EOF) {
		if p.currentTokenIs(token.NEWLINE) {
			// Early exit
			p.nextToken()
			continue
		}
		stmt := p.parseStatement()
		if stmt != nil {
			program.Statements = append(program.Statements, stmt)
		}
		p.nextToken()
	}
	if p.mode&ParseComments != 0 && len(p.comments) > 0 {
		program.Statements = mergeComments(program.Statements, p.comments)
	}
	if len(p.errors) != 0 {
		return program, NewErrors("Parsing errors", p.errors...)
	}
	return program, nil
}

func stmtPos(s ast.Statement) int {
	defer func() { recover() }()
	return s.Pos()
}

func mergeComments(stmts []ast.Statement, comments []*ast.Comment) []ast.Statement {
	merged := make([]ast.Statement, 0, len(stmts)+len(comments))
	ci := 0
	for _, stmt := range stmts {
		sp := stmtPos(stmt)
		for ci < len(comments) && comments[ci].Pos() < sp {
			merged = append(merged, comments[ci])
			ci++
		}
		merged = append(merged, stmt)
	}
	for ci < len(comments) {
		merged = append(merged, comments[ci])
		ci++
	}
	return merged
}

func (p *parser) parseStatement() ast.Statement {
	defer trace.TraceCtx(p.ctx)()
	switch p.curToken.Type {
	case token.ILLEGAL:
		p.errors = append(p.errors, &parseError{
			Pos:  p.file.Position(p.pos),
			Kind: LexError,
			Msg:  p.curToken.Literal,
		})
		return nil
	case token.EOF:
		p.expectError(token.NEWLINE)
		return nil
	case token.NEWLINE, token.SEMICOLON:
		return nil
	case token.END, token.RBRACE, token.RBRACKET:
		// Compound expressions leave these terminators at curToken.
		// Silently skip rather than producing an error.
		return nil
	case token.RETURN:
		// Bare return with modifier: route through expression path
		// so the modifier infix handler (if/unless/while/until) can
		// attach to the JumpExpression.
		if p.peekTokenOneOf(token.IF, token.UNLESS, token.WHILE, token.UNTIL) {
			return p.parseExpressionStatement()
		}
		return p.parseReturnStatement()
	default:
		return p.parseExpressionStatement()
	}
}

func (p *parser) parseReturnExpression() ast.Expression {
	jmp := &ast.JumpExpression{Token: p.curToken}
	if !p.peekTokenOneOf(token.NEWLINE, token.SEMICOLON, token.END, token.EOF, token.RBRACE, token.RPAREN, token.RBRACKET, token.EMBEXPR_END, token.IF, token.UNLESS, token.WHILE, token.UNTIL) {
		p.nextToken()
		jmp.Value = p.parseExpression(precIfUnless)
	}
	return jmp
}

func (p *parser) parseReturnStatement() ast.Statement {
	defer trace.TraceCtx(p.ctx)()
	stmt := &ast.ReturnStatement{Token: p.curToken}
	if p.peekTokenOneOf(token.NEWLINE, token.SEMICOLON, token.END, token.EOF, token.RBRACE) {
		return stmt
	}
	p.nextToken()

	if p.currentTokenOneOf(token.NEWLINE, token.SEMICOLON, token.END, token.EOF) {
		return stmt
	}
	if p.currentTokenOneOf(token.IF, token.UNLESS, token.WHILE, token.UNTIL) {
		return stmt
	}
	valToken := p.curToken
	stmt.ReturnValue = p.parseExpression(precIfUnless)
	if list, ok := stmt.ReturnValue.(ast.ExpressionList); ok {
		stmt.ReturnValue = &ast.ArrayLiteral{Elements: list}
	}

	// Modifier if/unless/while/until after return value: wrap
	// the ReturnStatement in a Conditional/LoopExpression.
	if p.peekTokenOneOf(token.IF, token.UNLESS) {
		p.nextToken()
		jmp := &ast.JumpExpression{Token: stmt.Token}
		jmp.Value = stmt.ReturnValue
		return &ast.ExpressionStatement{Expression: p.parseModifierConditionalExpression(jmp)}
	}
	if p.peekTokenOneOf(token.WHILE, token.UNTIL) {
		p.nextToken()
		jmp := &ast.JumpExpression{Token: stmt.Token}
		jmp.Value = stmt.ReturnValue
		return &ast.ExpressionStatement{Expression: p.parseModifierLoopExpression(jmp)}
	}
	if p.peekTokenOneOf(token.NEWLINE, token.SEMICOLON, token.ELSE, token.KW_ELSIF, token.END, token.RBRACE, token.EOF,
		token.RESCUE) {
		return stmt
	}

	if !p.peekTokenIs(token.COMMA) {
		p.peekError(token.COMMA)
		return nil
	}

	arr := &ast.ArrayLiteral{Token: valToken, Elements: []ast.Expression{stmt.ReturnValue}}
	for p.peekTokenIs(token.COMMA) {
		p.consume(token.COMMA)
		arr.Elements = append(arr.Elements, p.parseExpression(precLowest))
	}
	arr.Rbracket = p.curToken
	stmt.ReturnValue = arr

	if !p.acceptOneOf(token.NEWLINE, token.SEMICOLON) {
		return nil
	}
	return stmt
}

var bareCallArgTokens = []token.Type{
	token.TRUE, token.FALSE, token.NIL,
	token.INT, token.FLOAT,
	token.SELF, token.CONST,
	token.STRING, token.STRING_BEG,
	token.SYMBEG,
	token.AT, token.CLASS_VAR,
	token.GLOBAL,
	token.REGEX_BEG, token.REGEX,
	token.DEF, // `private def x` -- def returns sym, becomes arg
	// __FILE__ / __LINE__ / __method__ / __dir__ / __ENCODING__ / __callee__
	// are atomic ident-like expressions and can start a paren-less arg.
	token.KEYWORD__FILE__, token.KEYWORD__LINE__, token.KEYWORD__METHOD__,
	token.KEYWORD__DIR__, token.KEYWORD__ENCODING__, token.KEYWORD__CALLEE__,
}

func (p *parser) parseExpressionStatement() *ast.ExpressionStatement {
	defer trace.TraceCtx(p.ctx)()
	stmt := &ast.ExpressionStatement{Token: p.curToken}
	stmt.Expression = p.parseExpression(precLowest)
	// Bare function call with literal args: `foo true`, `bar 42`, `raise Error`, etc.
	if ident, ok := stmt.Expression.(*ast.Identifier); ok && !ident.IsConstant() && p.peekTokenOneOf(bareCallArgTokens...) {
		exp := &ast.ContextCallExpression{Token: ident.Token, Function: ident}
		p.nextToken()
		exp.Arguments = p.parseCallArguments(token.SEMICOLON, token.NEWLINE, token.LBRACE, token.DO)
		if p.currentTokenOneOf(token.LBRACE, token.DO) {
			exp.Block = p.parseBlockExpr()
		}
		// Peek-across-newline block attach would be wrong: in MRI, a block
		// (`{...}` or `do...end`) on a fresh line after a paren-less call
		// is a separate statement (hash / unrelated block-of-code), not a
		// block argument.
		stmt.Expression = exp
		// Re-enter the expression loop for modifier if/unless/while/until.
		if p.peekTokenOneOf(token.IF, token.UNLESS, token.WHILE, token.UNTIL, token.RESCUE) {
			p.nextToken()
			infix := infixParseFns[p.curToken.Type]
			if infix != nil {
				stmt.Expression = infix(p, stmt.Expression)
			}
		}
	}
	// Block attachment: expr do...end or expr { ... }
	// Only attach if the block opener is contiguous with the call (no
	// intervening NEWLINE/SEMICOLON). In MRI, `foo\n{...}` is two
	// statements, not a call with a block argument.
	if p.peekTokenOneOf(token.DO, token.LBRACE) && !p.currentTokenOneOf(token.NEWLINE, token.SEMICOLON) {
		if _, isIdent := stmt.Expression.(*ast.Identifier); !isIdent {
			p.nextToken()
			stmt.Expression = p.parseCallBlock(stmt.Expression)
		}
	}
	for p.peekTokenOneOf(token.SEMICOLON, token.NEWLINE) {
		p.nextToken()
		if p.peekTokenIs(token.DOT) {
			p.nextToken()
			stmt.Expression = p.parseMethodCall(stmt.Expression)
		}
	}
	return stmt
}

func (p *parser) parseExpression(precedence int) ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	prefix := prefixParseFns[p.curToken.Type]
	if prefix == nil {
		p.noPrefixParseFnError(p.curToken.Type)
		return nil
	}
	leftExp := prefix(p)
	for precedence < p.peekPrecedence() {
		if leftExp == nil {
			return nil // fail early and stop parsing
		}
		if p.currentTokenOneOf(token.NEWLINE, token.SEMICOLON) {
			return leftExp
		}
		if p.suppressDoBlock && p.peekTokenIs(token.DO) {
			return leftExp
		}
		if p.suppressHashrocket && p.peekTokenIs(token.HASHROCKET) {
			return leftExp
		}
		if p.suppressKwAndOr && p.peekTokenOneOf(token.KW_AND, token.KW_OR) {
			return leftExp
		}
		// `ident ::Foo` (with leading space on `::`) is a paren-less call
		// where `::Foo` starts a top-level scoped-constant arg, not
		// `ident::Foo` (scope inside ident). Only applies when ident is
		// lowercase (constants can legitimately scope: `Foo::Bar`).
		if p.peekTokenIs(token.SCOPE) && p.peekToken.HadWhitespace {
			if id, ok := leftExp.(*ast.Identifier); ok && !id.IsConstant() {
				call := &ast.ContextCallExpression{Token: id.Token, Function: id}
				p.nextToken() // advance to ::
				prevAO := p.suppressKwAndOr
				prevSDB := p.suppressDoBlock
				p.suppressKwAndOr = true
				p.suppressDoBlock = true
				call.Arguments = p.parseCallArguments(
					token.SEMICOLON, token.NEWLINE, token.LBRACE, token.DO,
				)
				p.suppressKwAndOr = prevAO
				p.suppressDoBlock = prevSDB
				if p.currentTokenOneOf(token.LBRACE, token.DO) {
					call.Block = p.parseBlockExpr()
				}
				leftExp = call
				continue
			}
		}
		// `ident [array]` (with leading space on `[`) is a command call with
		// an array literal as its first arg, not an index expression.
		if p.peekTokenIs(token.LBRACKET) && p.peekToken.HadWhitespace {
			if id, ok := leftExp.(*ast.Identifier); ok && !id.IsConstant() {
				call := &ast.ContextCallExpression{Token: id.Token, Function: id}
				p.nextToken() // advance to [
				call.Arguments = p.parseCallArguments(
					token.SEMICOLON, token.NEWLINE, token.LBRACE, token.DO,
				)
				if p.currentTokenOneOf(token.LBRACE, token.DO) {
					call.Block = p.parseBlockExpr()
				}
				leftExp = call
				continue
			}
		}
		// `ident (expr)` with whitespace before `(` is a command call whose
		// first arg is a parenthesised expression, not a normal paren call.
		// MRI keeps the explicit ParenthesesNode on the arg in this case.
		if p.peekTokenIs(token.LPAREN) && p.peekToken.HadWhitespace {
			if id, ok := leftExp.(*ast.Identifier); ok && !id.IsConstant() {
				call := &ast.ContextCallExpression{Token: id.Token, Function: id}
				p.nextToken() // advance to (
				prevAO := p.suppressKwAndOr
				p.suppressKwAndOr = true
				call.Arguments = p.parseCallArguments(
					token.SEMICOLON, token.NEWLINE, token.LBRACE, token.DO,
				)
				p.suppressKwAndOr = prevAO
				if p.currentTokenOneOf(token.LBRACE, token.DO) {
					call.Block = p.parseBlockExpr()
				}
				leftExp = call
				continue
			}
		}
		// `ident -x` / `ident +x` (whitespace before -/+, none after) is a
		// command call with a unary-negated arg, not infix subtraction /
		// addition. `ident - x` / `ident-x` stay infix.
		isOperandStart := func(t token.Type) bool {
			switch t {
			case token.IDENT, token.CONST, token.INT, token.FLOAT,
				token.AT, token.CLASS_VAR, token.GLOBAL, token.LPAREN, token.LBRACKET:
				return true
			}
			return false
		}
		if p.peekTokenOneOf(token.MINUS, token.PLUS) && p.peekToken.HadWhitespace &&
			!p.peek2Token.HadWhitespace && isOperandStart(p.peek2Token.Type) {
			if id, ok := leftExp.(*ast.Identifier); ok && !id.IsConstant() {
				call := &ast.ContextCallExpression{Token: id.Token, Function: id}
				p.nextToken() // advance to - / +
				prevAO := p.suppressKwAndOr
				p.suppressKwAndOr = true
				call.Arguments = p.parseCallArguments(
					token.SEMICOLON, token.NEWLINE, token.LBRACE, token.DO,
				)
				p.suppressKwAndOr = prevAO
				if p.currentTokenOneOf(token.LBRACE, token.DO) {
					call.Block = p.parseBlockExpr()
				}
				leftExp = call
				continue
			}
		}
		infix := infixParseFns[p.peekToken.Type]
		if infix == nil {
			return leftExp
		}
		p.nextToken()
		leftExp = infix(p, leftExp)
	}
	// Leading-dot continuation: expr\n.method
	for p.peekTokenIs(token.NEWLINE) && p.peek2TokenIs(token.DOT) {
		p.nextToken() // consume NEWLINE
		p.nextToken() // consume DOT (now curToken)
		leftExp = p.parseMethodCall(leftExp)
	}
	// After an identifier, { is always a block (not a hash).
	if _, ok := leftExp.(*ast.Identifier); ok && p.peekTokenIs(token.LBRACE) {
		p.nextToken()
		leftExp = p.parseCallBlock(leftExp)
		for precedence < p.peekPrecedence() {
			if p.currentTokenOneOf(token.NEWLINE, token.SEMICOLON) {
				return leftExp
			}
			if p.suppressDoBlock && p.peekTokenIs(token.DO) {
				return leftExp
			}
			infix := infixParseFns[p.peekToken.Type]
			if infix == nil {
				return leftExp
			}
			p.nextToken()
			leftExp = infix(p, leftExp)
		}
	}
	return leftExp
}

func (p *parser) parseExceptionHandlingBlock() ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	block := &ast.ExceptionHandlingBlock{BeginToken: p.curToken}
	if !p.acceptOneOf(token.NEWLINE, token.SEMICOLON) {
		return nil
	}
	// Try body ends at END, RESCUE, or ENSURE.
	block.TryBody = p.parseBlockStatement(token.END, token.RESCUE, token.KW_ENSURE)
	block.Rescues = []*ast.RescueBlock{}
	for p.peekTokenIs(token.RESCUE) {
		p.accept(token.RESCUE)
		rescue := p.parseRescueBlock()
		block.Rescues = append(block.Rescues, rescue)
	}
	if p.peekTokenIs(token.ELSE) {
		p.accept(token.ELSE)
		p.acceptOneOf(token.NEWLINE, token.SEMICOLON)
		block.ElseBody = p.parseBlockStatement(token.END, token.KW_ENSURE)
	}
	// Optional else clause (runs when no exception was raised).
	if p.peekTokenIs(token.ELSE) {
		p.accept(token.ELSE)
		p.acceptOneOf(token.NEWLINE, token.SEMICOLON)
		block.ElseBody = p.parseBlockStatement(token.END, token.KW_ENSURE)
	}
	// Optional ensure clause.
	if p.peekTokenIs(token.KW_ENSURE) {
		p.accept(token.KW_ENSURE)
		p.acceptOneOf(token.NEWLINE, token.SEMICOLON)
		block.EnsureBody = p.parseBlockStatement(token.END)
	}
	if !p.accept(token.END) {
		return nil
	}
	block.EndToken = p.curToken
	return block
}

func (p *parser) parseRescueBlock() *ast.RescueBlock {
	defer trace.TraceCtx(p.ctx)()
	block := &ast.RescueBlock{Token: p.curToken}
	classes := []*ast.Identifier{}
	for !p.peekTokenOneOf(token.HASHROCKET, token.NEWLINE, token.SEMICOLON, token.EOF, token.END, token.THEN) {
		isSplat := false
		if p.peekTokenIs(token.ASTERISK) {
			isSplat = true
			p.accept(token.ASTERISK)
		}
		p.nextToken()
		expr := p.parseExpression(precAssignment)
		name := ""
		if expr != nil {
			name = expr.String()
		}
		if isSplat {
			name = "*" + name
		}
		class := &ast.Identifier{Token: p.curToken, Value: name}
		classes = append(classes, class)
		if p.peekTokenIs(token.COMMA) {
			p.accept(token.COMMA)
		} else {
			break
		}
	}
	block.ExceptionClasses = classes

	if p.peekTokenIs(token.HASHROCKET) {
		p.accept(token.HASHROCKET)
		if !p.accept(token.IDENT) {
			return nil
		}
		block.Exception = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
	}
	if !p.acceptOneOf(token.NEWLINE, token.SEMICOLON) {
		return nil
	}
	block.Body = p.parseBlockStatement(token.END, token.RESCUE, token.KW_ENSURE, token.ELSE)
	return block
}

func (p *parser) parseExpressions(left ast.Expression) ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	// Trailing comma: a, = 1 -- peekToken after , is = or closing delimiter
	if p.peekTokenOneOf(token.ASSIGN, token.RPAREN, token.RBRACKET, token.RBRACE) {
		return ast.ExpressionList{left}
	}
	p.nextToken()
	for p.currentTokenOneOf(token.NEWLINE, token.SEMICOLON) {
		p.nextToken()
	}
	if p.currentTokenOneOf(token.RPAREN, token.RBRACKET, token.RBRACE) {
		return ast.ExpressionList{left}
	}
	elements := []ast.Expression{left}
	if next := p.parseExpression(precAssignment); next != nil {
		elements = append(elements, next)
	} else {
		p.errors = append(p.errors, &parseError{
			Pos:  p.file.Position(p.pos),
			Kind: SyntaxError,
			Msg:  "expected expression after ','",
		})
	}
	for p.peekTokenIs(token.COMMA) {
		p.consume(token.COMMA)
		for p.currentTokenOneOf(token.NEWLINE, token.SEMICOLON) {
			p.nextToken()
		}
		// Trailing comma: after consume, curToken is past the comma.
		if p.currentTokenOneOf(token.RPAREN, token.RBRACKET, token.RBRACE) {
			break
		}
		// Trailing comma followed by `=` is multi-assign with implicit-rest
		// LHS (`a, b, = X`). Append the sentinel so the printer preserves
		// the trailing comma, then hand off to parseAssignment.
		if p.currentTokenIs(token.ASSIGN) {
			elements = append(elements, &ast.ImplicitRest{Token: p.curToken})
			lhs := ast.ExpressionList(elements)
			return p.parseAssignment(lhs)
		}
		if next := p.parseExpression(precAssignment); next != nil {
			elements = append(elements, next)
		} else {
			p.errors = append(p.errors, &parseError{
				Pos:  p.file.Position(p.pos),
				Kind: SyntaxError,
				Msg:  "expected expression after ','",
			})
		}
	}
	return ast.ExpressionList(elements)
}

func (p *parser) parseBlockCapture() ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	capture := &ast.BlockCapture{Token: p.curToken}
	if p.peekTokenIs(token.LAMBDA) {
		p.nextToken()
		capture.Expr = p.parseLambda()
		return capture
	}
	if p.peekTokenIs(token.SYMBEG) {
		p.nextToken()
		sym := p.parseSymbolLiteral()
		if sym == nil {
			return nil
		}
		capture.Expr = sym
		return capture
	}
	// Anonymous block forwarding: &) or &, (ruby 3.1+)
	if p.peekTokenOneOf(token.RPAREN, token.COMMA, token.NEWLINE, token.SEMICOLON) {
		return capture
	}
	p.nextToken()
	capture.Expr = p.parseExpression(precPrefix)
	if capture.Expr == nil {
		return capture
	}
	if ident, ok := capture.Expr.(*ast.Identifier); ok {
		capture.Name = ident
	}
	return capture
}

func (p *parser) parseAssignmentOperator(left ast.Expression) ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	assignIndex := strings.LastIndexByte(p.curToken.Literal, '=')
	if assignIndex < 0 {
		return nil
	}
	newInf := &ast.InfixExpression{
		Left:     left,
		Operator: p.curToken.Literal[:assignIndex],
	}
	assign := &ast.Assignment{
		Token: p.curToken,
		Left:  left,
	}
	p.nextToken()
	for p.currentTokenIs(token.NEWLINE) {
		p.nextToken()
	}
	newInf.Right = p.parseExpression(precAssignment)
	if newInf.Right == nil {
		return nil
	}
	assign.Right = newInf
	return assign
}

func (p *parser) parseAssignment(left ast.Expression) ast.Expression {
	defer trace.TraceCtx(p.ctx)()

	switch leftNode := left.(type) {
	case *ast.Identifier:
	case *ast.Global:
	case *ast.ClassVariable:
	case *ast.IndexExpression:
	case *ast.InstanceVariable:
	case *ast.ScopedIdentifier:
	case *ast.PrefixExpression:
		_ = leftNode
	case *ast.SplatExpression:
		_ = leftNode
	case ast.ExpressionList:
	case *ast.ParenExpression:
		_ = leftNode
	case *ast.Keyword__FILE__:
		p.errors = append(p.errors, &parseError{
			Pos:  p.file.Position(p.pos),
			Kind: SyntaxError,
			Msg:  "Can't assign to __FILE__",
		})
		return nil
	case *ast.ContextCallExpression:
		// obj.method = value => obj.method=(value)
		leftNode.Function.Value += "="
		leftNode.Function.Token.Literal += "="
		p.nextToken()
		right := p.parseExpression(precLowest)
		if right == nil {
			return nil
		}
		leftNode.Arguments = []ast.Expression{right}
		return leftNode
	default:
		p.expectError(token.EOF)
		return nil
	}

	assign := &ast.Assignment{
		Token: p.curToken,
		Left:  left,
	}
	p.nextToken()
	for p.currentTokenIs(token.NEWLINE) {
		p.nextToken()
	}
	rhsPrec := precLowest
	if p.suppressHashrocket {
		rhsPrec = precComma
	}
	expr := p.parseExpression(rhsPrec)
	if expr == nil {
		return nil
	}
	// `a = b and c` -> `(a = b) and c` (and/or have lower precedence than =)
	if inf, ok := expr.(*ast.InfixExpression); ok && (inf.Operator == "and" || inf.Operator == "or") {
		assign.Right = inf.Left
		inf.Left = assign
		return inf
	}
	// Modifier-loop on assignment: `a = b while c` -> `(a = b) while c`.
	// Restructure so the assignment is the loop body, not the loop the
	// assignment value.
	if loop, ok := expr.(*ast.LoopExpression); ok && loop.EndToken.Type != token.END {
		if len(loop.Block.Statements) == 1 {
			if es, ok := loop.Block.Statements[0].(*ast.ExpressionStatement); ok {
				assign.Right = es.Expression
				loop.Block = &ast.BlockStatement{
					Statements: []ast.Statement{
						&ast.ExpressionStatement{Expression: assign},
					},
				}
				return loop
			}
		}
	}
	right, ok := expr.(*ast.ConditionalExpression)
	if !ok || right.Token.Type == token.QMARK || right.EndToken.Type == token.END {
		assign.Right = expr
		return assign
	}
	// Modifier if/unless: restructure `x = val if cond` into `if cond then x = val end`
	expStmt, ok := right.Consequence.Statements[0].(*ast.ExpressionStatement)
	if !ok {
		assign.Right = expr
		return assign
	}
	assign.Right = expStmt.Expression
	right.Consequence = &ast.BlockStatement{
		Statements: []ast.Statement{
			&ast.ExpressionStatement{Expression: assign},
		},
	}
	return right
}

func (p *parser) acceptIvarName() bool {
	if p.peekTokenOneOf(token.IDENT, token.CONST) || p.peekToken.Type.IsKeyword() {
		p.nextToken()
		return true
	}
	p.peekError(token.IDENT, token.CONST)
	return false
}

func (p *parser) parseInstanceVariable() ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	instanceVariable := &ast.InstanceVariable{Token: p.curToken}
	if !p.acceptIvarName() {
		return nil
	}
	instanceVariable.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
	return instanceVariable
}

func (p *parser) parseClassVariable() ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	cv := &ast.ClassVariable{Token: p.curToken}
	if !p.acceptIvarName() {
		return nil
	}
	cv.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
	return cv
}

func (p *parser) parseLabelExpression() ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	// The LABEL token's literal is e.g. "foo:"
	name := strings.TrimSuffix(p.curToken.Literal, ":")
	key := &ast.SymbolLiteral{
		Token: p.curToken,
		Value: &ast.StringLiteral{Value: name},
	}
	if p.peekTokenOneOf(token.COMMA, token.RPAREN, token.RBRACE, token.RBRACKET, token.NEWLINE, token.SEMICOLON, token.PIPE) {
		// Hash-value-omission (Ruby 3.1+): `foo:` is shorthand for `foo: foo`.
		// Store with Right=nil so the printer preserves the omitted form
		// instead of expanding it (which would re-parse as ImplicitNode in
		// MRI, diverging from a source written as `foo: foo`).
		return &ast.InfixExpression{
			Token:    key.Token,
			Left:     key,
			Operator: ":",
			Right:    nil,
		}
	}
	p.nextToken()
	val := p.parseExpression(precAssignment)
	return &ast.InfixExpression{
		Token:    key.Token,
		Left:     key,
		Operator: ":",
		Right:    val,
	}
}

// parseErrorSkip is an error-recovery handler for tokens that appear as
// curToken in unexpected contexts (e.g. ) outside parens, ] outside indexing).
// It emits an error and returns a Nil placeholder so parsing can continue.
func (p *parser) parseRescueModifier(left ast.Expression) ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	// expr rescue fallback -- low-precedence infix
	p.nextToken() // consume rescue
	right := p.parseExpression(precLowest)
	return &ast.InfixExpression{
		Token:    p.curToken,
		Left:     left,
		Operator: "rescue",
		Right:    right,
	}
}

func (p *parser) parseTopLevelScope() ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	// ::Foo, ::Foo::Bar -- scope resolution from top-level
	tok := p.curToken // SCOPE token
	p.nextToken()     // advance past ::
	if !p.currentTokenIs(token.CONST) {
		p.expectError(token.CONST)
		return nil
	}
	inner := p.parseIdentifier().(*ast.Identifier)
	return &ast.ScopedIdentifier{Token: tok, Inner: inner}
}

func (p *parser) parseDefinedExpression() ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	expr := &ast.DefinedExpression{Token: p.curToken}
	// defined? can be: defined?(expr) (no space -- call-paren syntax) or
	// defined? expr (with space, optionally followed by a grouped paren
	// expr which MRI keeps as a ParenthesesNode).
	if p.peekTokenIs(token.LPAREN) && !p.peekToken.HadWhitespace {
		p.accept(token.LPAREN)
		p.nextToken()
		for p.currentTokenOneOf(token.NEWLINE, token.SEMICOLON) {
			p.nextToken()
		}
		expr.Expr = p.parseExpression(precLowest)
		for p.peekTokenOneOf(token.NEWLINE, token.SEMICOLON) {
			p.nextToken()
		}
		if p.peekTokenIs(token.RPAREN) {
			p.accept(token.RPAREN)
		}
	} else {
		p.nextToken()
		// Parse at precComma so a trailing `,` (in a paren-less call
		// arg list like `assert(defined? x, "msg")`) terminates the
		// operand instead of being absorbed.
		expr.Expr = p.parseExpression(precComma)
	}
	return expr
}

func (p *parser) parseErrorSkip() ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	p.expectError(token.IDENT) // generic expected error
	return &ast.Nil{Token: p.curToken}
}

func (p *parser) parseJumpExpression() ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	jmp := &ast.JumpExpression{Token: p.curToken}
	// break/next can take an optional value: break expr, next expr
	// redo/retry take no value
	if p.currentTokenIs(token.BREAK) || p.currentTokenIs(token.NEXT) {
		if !p.peekTokenOneOf(token.NEWLINE, token.SEMICOLON, token.EOF, token.IF, token.UNLESS, token.WHILE, token.UNTIL, token.RESCUE,
			token.RPAREN, token.RBRACKET, token.RBRACE, token.END, token.DOT, token.EMBEXPR_END) {
			p.nextToken()
			jmp.Value = p.parseExpression(precIfUnless)
		}
	}
	return jmp
}

// isLabelPair reports whether e is an `a:` / `a: val` pair (encoded by
// parseLabelExpression as an InfixExpression with operator ":" and a
// SymbolLiteral on the left).
func isLabelPair(e ast.Expression) bool {
	ix, ok := e.(*ast.InfixExpression)
	if !ok || ix.Operator != ":" {
		return false
	}
	_, ok = ix.Left.(*ast.SymbolLiteral)
	return ok
}

func appendLabelPair(m *ast.OrderedExprMap, e ast.Expression) {
	ix := e.(*ast.InfixExpression)
	if ix.Right != nil {
		m.Set(ix.Left, ix.Right)
		return
	}
	// Value omission (`a:`): mirror parseKeyValue's encoding -- store the
	// identifier as the value and mark omitted, so the hash printer emits
	// `a:` (label form) instead of falling back to the value-nil path.
	sym, ok := ix.Left.(*ast.SymbolLiteral)
	if !ok {
		m.Set(ix.Left, nil)
		return
	}
	name := strings.TrimSuffix(sym.Token.Literal, ":")
	m.Set(sym, &ast.Identifier{Token: sym.Token, Value: name})
	m.SetOmitted(sym)
}

func allLabelPairs(elements []ast.Expression) bool {
	for _, e := range elements {
		if isLabelPair(e) {
			continue
		}
		// `**rest` / `**nil` is a valid hash-pattern element alongside label pairs.
		if pe, ok := e.(*ast.PrefixExpression); ok && pe.Operator == "**" {
			continue
		}
		return false
	}
	return true
}

func (p *parser) parsePattern() ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	pat := p.parsePatternOr()
	if pat == nil {
		return nil
	}
	// Implicit hash with string label key: in "key": val
	if _, isStr := pat.(*ast.StringLiteral); isStr && p.peekTokenOneOf(token.COLON, token.SYMBEG) {
		p.acceptOneOf(token.COLON, token.SYMBEG)
		pairs := ast.NewOrderedExprMap()
		key := &ast.SymbolLiteral{Token: p.curToken, Value: pat.(*ast.StringLiteral)}
		var val ast.Expression
		if !p.peekTokenOneOf(token.COMMA, token.RBRACE, token.NEWLINE, token.SEMICOLON, token.THEN, token.EOF) {
			p.nextToken()
			val = p.parsePatternBinding()
		}
		pairs.Set(key, val)
		for p.peekTokenIs(token.COMMA) {
			p.accept(token.COMMA)
			p.nextToken()
			p.parsePatternHashPair(pairs)
		}
		pat = &ast.HashLiteral{Token: p.curToken, Map: pairs}
	}
	// Implicit array pattern: in a, b, c == in [a, b, c]
	if p.peekTokenIs(token.COMMA) {
		elements := []ast.Expression{pat}
		trailingComma := false
		for p.peekTokenIs(token.COMMA) {
			p.accept(token.COMMA)
			if p.peekTokenOneOf(token.NEWLINE, token.SEMICOLON, token.THEN, token.IF, token.UNLESS, token.EOF) {
				trailingComma = true
				break
			}
			p.nextToken()
			elements = append(elements, p.parsePatternOr())
		}
		if trailingComma {
			// `in 0,` matches `[0, ...anything]` -- MRI tags this with
			// ImplicitRestNode on the pattern. Preserve via the sentinel
			// so the printer emits the trailing comma back.
			elements = append(elements, &ast.ImplicitRest{Token: p.curToken})
		}
		// All-label-pair elements collapse to an implicit hash pattern: MRI
		// parses `in a: 0, b: 1` as a hash pattern, not an array containing
		// hash pairs. Wrapping in `[...]` would re-parse with implicit-hash-in-
		// array which MRI rejects.
		if allLabelPairs(elements) {
			pairs := ast.NewOrderedExprMap()
			var splats []ast.Expression
			for _, e := range elements {
				if isLabelPair(e) {
					appendLabelPair(pairs, e)
				} else {
					// `**X` rest pattern -- record as a splat entry on the
					// HashLiteral so the printer emits it with the other pairs.
					splats = append(splats, e)
				}
			}
			pat = &ast.HashLiteral{Token: p.curToken, Map: pairs, Splats: splats}
		} else {
			pat = &ast.ArrayLiteral{Token: p.curToken, Elements: elements}
		}
	}
	// Guard clause: pattern if / unless condition
	if p.peekTokenOneOf(token.IF, token.UNLESS) {
		p.acceptOneOf(token.IF, token.UNLESS)
		guardTok := p.curToken
		p.nextToken()
		p.inPattern = false
		guard := p.parseExpression(precLowest)
		p.inPattern = true
		pat = &ast.InfixExpression{
			Token:    guardTok,
			Left:     pat,
			Operator: guardTok.Literal,
			Right:    guard,
		}
	}
	return pat
}

func (p *parser) parsePatternOr() ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	left := p.parsePatternBinding()
	for p.peekTokenIs(token.PIPE) {
		p.accept(token.PIPE)
		p.nextToken()
		right := p.parsePatternBinding()
		left = &ast.InfixExpression{
			Token:    token.NewToken(token.PIPE, "|", 0),
			Left:     left,
			Operator: "|",
			Right:    right,
		}
	}
	return left
}

func (p *parser) parsePatternBinding() ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	left := p.parsePatternAtom()
	if p.peekTokenIs(token.HASHROCKET) {
		p.accept(token.HASHROCKET)
		p.nextToken()
		right := p.parsePatternAtom()
		left = &ast.InfixExpression{
			Token:    token.NewToken(token.HASHROCKET, "=>", 0),
			Left:     left,
			Operator: "=>",
			Right:    right,
		}
	}
	return left
}

func (p *parser) parsePatternAtom() ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	switch p.curToken.Type {
	case token.XOR:
		// Pin operator: ^var or ^(expr)
		op := p.curToken
		p.nextToken()
		if p.currentTokenIs(token.LPAREN) {
			p.nextToken()
			p.inPattern = false
			expr := p.parseExpression(precLowest)
			p.inPattern = true
			p.accept(token.RPAREN)
			return &ast.PrefixExpression{Token: op, Operator: "^", Right: expr}
		}
		right := p.parseExpression(precHighest)
		return &ast.PrefixExpression{Token: op, Operator: "^", Right: right}
	case token.ASTERISK:
		// Splat in array pattern: *rest or bare *
		op := p.curToken
		if p.peekTokenOneOf(token.COMMA, token.RBRACKET, token.RBRACE,
			token.NEWLINE, token.SEMICOLON, token.THEN, token.EOF) {
			return &ast.PrefixExpression{Token: op, Operator: "*", Right: nil}
		}
		p.nextToken()
		right := p.parsePatternAtom()
		return &ast.PrefixExpression{Token: op, Operator: "*", Right: right}
	case token.POWER:
		// Double splat in hash pattern: **rest or bare **
		op := p.curToken
		if p.peekTokenOneOf(token.COMMA, token.RBRACKET, token.RBRACE,
			token.NEWLINE, token.SEMICOLON, token.THEN, token.EOF) {
			return &ast.PrefixExpression{Token: op, Operator: "**", Right: nil}
		}
		p.nextToken()
		right := p.parsePatternAtom()
		return &ast.PrefixExpression{Token: op, Operator: "**", Right: right}
	case token.LBRACKET:
		// Array pattern
		return p.parsePatternArray()
	case token.LBRACE:
		// Hash pattern
		return p.parsePatternHash()
	case token.STRING, token.STRING_BEG:
		p.inPattern = false
		expr := p.parseExpression(precHighest)
		p.inPattern = true
		return expr
	default:
		// Use normal expression parsing for literals, constants, identifiers, etc.
		// Parse at precRange-1 so range operators (.. / ...) are included.
		p.inPattern = false
		expr := p.parseExpression(precRange - 1)
		p.inPattern = true
		return expr
	}
}

func (p *parser) parsePatternArray() ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	tok := p.curToken
	elements := []ast.Expression{}
	if p.peekTokenIs(token.RBRACKET) {
		p.accept(token.RBRACKET)
		return &ast.ArrayLiteral{Token: tok, Elements: elements}
	}
	p.nextToken()
	elements = append(elements, p.parsePatternBinding())
	for p.peekTokenIs(token.COMMA) {
		p.accept(token.COMMA)
		if p.peekTokenIs(token.RBRACKET) {
			// Trailing comma marks an implicit-rest match in array patterns:
			// `in [0,]` matches `[0, ...anything]`. MRI tags this with an
			// ImplicitRestNode on the pattern -- preserve via the sentinel.
			elements = append(elements, &ast.ImplicitRest{Token: p.curToken})
			break
		}
		p.nextToken()
		elements = append(elements, p.parsePatternBinding())
	}
	if !p.accept(token.RBRACKET) {
		return nil
	}
	return &ast.ArrayLiteral{Token: tok, Elements: elements}
}

func (p *parser) parsePatternHash() ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	tok := p.curToken
	pairs := ast.NewOrderedExprMap()
	if p.peekTokenIs(token.RBRACE) {
		p.accept(token.RBRACE)
		return &ast.HashLiteral{Token: tok, Map: pairs}
	}
	p.nextToken()
	p.parsePatternHashPair(pairs)
	for {
		p.skipNewlines()
		if p.peekTokenIs(token.RBRACE) {
			break
		}
		if !p.peekTokenIs(token.COMMA) {
			break
		}
		p.accept(token.COMMA)
		p.skipNewlines()
		if p.peekTokenIs(token.RBRACE) {
			break
		}
		p.nextToken()
		p.parsePatternHashPair(pairs)
	}
	if !p.accept(token.RBRACE) {
		return nil
	}
	return &ast.HashLiteral{Token: tok, Map: pairs}
}

func (p *parser) parsePatternHashPair(pairs *ast.OrderedExprMap) {
	defer trace.TraceCtx(p.ctx)()
	if p.currentTokenIs(token.POWER) {
		// **rest or bare **
		op := p.curToken
		if p.peekTokenOneOf(token.COMMA, token.RBRACE) {
			pairs.Set(&ast.PrefixExpression{Token: op, Operator: "**"}, nil)
			return
		}
		p.nextToken()
		rest := p.parsePatternAtom()
		pairs.Set(&ast.PrefixExpression{Token: op, Operator: "**", Right: rest}, nil)
		return
	}
	// label: pattern  (symbol key with pattern value)
	if p.currentTokenIs(token.LABEL) {
		name := strings.TrimSuffix(p.curToken.Literal, ":")
		key := &ast.SymbolLiteral{
			Token: p.curToken,
			Value: &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal},
		}
		if p.peekTokenOneOf(token.COMMA, token.RBRACE, token.THEN) {
			pairs.Set(key, &ast.Identifier{Token: p.curToken, Value: name})
			pairs.SetOmitted(key)
			return
		}
		if p.peekTokenOneOf(token.NEWLINE, token.SEMICOLON) && (p.peek2TokenIs(token.RBRACE) || p.peek2TokenIs(token.COMMA) || p.peek2TokenIs(token.KW_IN) || p.peek2TokenIs(token.WHEN) || p.peek2TokenIs(token.END)) {
			pairs.Set(key, &ast.Identifier{Token: p.curToken, Value: name})
			pairs.SetOmitted(key)
			return
		}
		p.skipNewlines()
		p.nextToken()
		val := p.parsePatternBinding()
		pairs.Set(key, val)
		return
	}
	// String-key pattern: `"foo": pat` or `"foo":` (ruby 3.1+ value omission).
	// Parse the string directly to keep parseExpression from consuming the
	// following `:` / SYMBEG (which would otherwise be picked up as an infix
	// call-argument continuation and fail).
	if p.currentTokenOneOf(token.STRING, token.STRING_BEG) {
		var strKey *ast.StringLiteral
		if p.currentTokenIs(token.STRING_BEG) {
			if sl, ok := p.parseInterpolatedString().(*ast.StringLiteral); ok {
				strKey = sl
			}
		} else {
			if sl, ok := p.parseStringLiteral().(*ast.StringLiteral); ok {
				strKey = sl
			}
		}
		if strKey != nil && p.peekTokenOneOf(token.COLON, token.SYMBEG) {
			p.acceptOneOf(token.COLON, token.SYMBEG)
			symKey := &ast.SymbolLiteral{Token: p.curToken, Value: strKey}
			// Value-omission: `"a":` is shorthand for `"a": a` (ruby 3.1+).
			if p.peekTokenOneOf(token.COMMA, token.RBRACE, token.THEN, token.NEWLINE, token.SEMICOLON) {
				pairs.Set(symKey, &ast.Identifier{Token: strKey.Token, Value: strKey.Value})
				pairs.SetOmitted(symKey)
				return
			}
			p.nextToken()
			val := p.parsePatternBinding()
			pairs.Set(symKey, val)
			return
		}
		// Not a string-key pair; fall through to error/hashrocket path.
		if !p.accept(token.HASHROCKET) {
			return
		}
		p.nextToken()
		val := p.parsePatternBinding()
		pairs.Set(strKey, val)
		return
	}
	// key => pattern
	p.inPattern = false
	prevSuppressHR := p.suppressHashrocket
	p.suppressHashrocket = true
	key := p.parseExpression(precLowest)
	p.suppressHashrocket = prevSuppressHR
	p.inPattern = true
	if !p.accept(token.HASHROCKET) {
		return
	}
	p.nextToken()
	val := p.parsePatternBinding()
	pairs.Set(key, val)
}

func (p *parser) parseCaseExpression() ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	expr := &ast.CaseExpression{Token: p.curToken}
	p.nextToken()
	// Optional case expression: case x
	if !p.currentTokenIs(token.WHEN) && !p.currentTokenIs(token.NEWLINE) && !p.currentTokenIs(token.SEMICOLON) {
		expr.Condition = p.parseExpression(precLowest)
	}
	// Allow optional newline/semicolon after case expression.
	// Also allow `when`/`in` directly after expression (inline case/when).
	if p.peekTokenOneOf(token.WHEN, token.KW_IN) {
		p.nextToken()
	} else if !p.currentTokenOneOf(token.NEWLINE, token.SEMICOLON) {
		p.acceptOneOf(token.NEWLINE, token.SEMICOLON)
	}
	for p.currentTokenOneOf(token.NEWLINE, token.SEMICOLON) {
		p.nextToken()
	}
	// Parse when clauses.
	for p.currentTokenIs(token.WHEN) || p.peekTokenIs(token.WHEN) {
		for p.currentTokenOneOf(token.NEWLINE, token.SEMICOLON) {
			p.nextToken()
		}
		if !p.currentTokenIs(token.WHEN) {
			break
		}
		wc := &ast.WhenClause{Token: p.curToken}
		p.nextToken()
		// Parse one or more when conditions (comma-separated).
		wc.Conditions = []ast.Expression{p.parseExpression(precLowest)}
		for p.peekTokenIs(token.COMMA) {
			p.consume(token.COMMA)
			wc.Conditions = append(wc.Conditions, p.parseExpression(precLowest))
		}
		// Optional then/newline/semicolon.
		if p.peekTokenIs(token.THEN) {
			p.accept(token.THEN)
		} else {
			p.acceptOneOf(token.NEWLINE, token.SEMICOLON)
		}
		// Parse when body until next when, else, or end.
		wc.Body = p.parseBlockStatement(token.END, token.WHEN, token.KW_IN, token.ELSE)
		expr.WhenClauses = append(expr.WhenClauses, wc)
	}
	// Parse in clauses (pattern matching, ruby 2.7+).
	for p.currentTokenIs(token.KW_IN) || p.peekTokenIs(token.KW_IN) {
		for p.currentTokenOneOf(token.NEWLINE, token.SEMICOLON) {
			p.nextToken()
		}
		if !p.currentTokenIs(token.KW_IN) {
			break
		}
		if !p.version.AtLeast(ruby27) {
			p.versionError(ruby27, "pattern matching")
		}
		ic := &ast.WhenClause{Token: p.curToken}
		p.nextToken()
		p.inPattern = true
		ic.Conditions = []ast.Expression{p.parsePattern()}
		p.inPattern = false
		if p.peekTokenOneOf(token.IF, token.UNLESS) {
			p.nextToken()
			p.nextToken()
			guard := p.parseExpression(precLowest)
			_ = guard
		}
		if p.peekTokenIs(token.THEN) {
			p.accept(token.THEN)
		} else {
			p.acceptOneOf(token.NEWLINE, token.SEMICOLON)
		}
		ic.Body = p.parseBlockStatement(token.END, token.WHEN, token.KW_IN, token.ELSE)
		expr.InClauses = append(expr.InClauses, ic)
	}
	// Consume optional newlines/semicolons between when bodies and else.
	for p.currentTokenOneOf(token.NEWLINE, token.SEMICOLON) {
		p.nextToken()
	}
	// Optional else clause.
	if p.currentTokenIs(token.ELSE) {
		if p.peekTokenOneOf(token.NEWLINE, token.SEMICOLON) {
			p.acceptOneOf(token.NEWLINE, token.SEMICOLON)
		}
		expr.ElseBody = p.parseBlockStatement(token.END)
	}
	if p.currentTokenIs(token.END) {
		expr.EndToken = p.curToken
		return expr
	}
	if !p.accept(token.END) {
		return nil
	}
	expr.EndToken = p.curToken
	return expr
}

func (p *parser) parseNilLiteral() ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	return &ast.Nil{Token: p.curToken}
}

func (p *parser) parseIllegal() ast.Expression {
	p.errors = append(p.errors, &parseError{
		Pos:  p.file.Position(p.pos),
		Kind: LexError,
		Msg:  p.curToken.Literal,
	})
	return nil
}

func (p *parser) parseIdentifier() ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	return &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
}

func (p *parser) parseGlobal() ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	return &ast.Global{Token: p.curToken, Value: p.curToken.Literal}
}

func (p *parser) parseScopedIdentifierExpression(outer ast.Expression) ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	ident, ok := outer.(*ast.Identifier)
	if !ok {
		return p.parseMethodCall(outer)
	}

	scopeToken := p.curToken
	scopedIdent := &ast.ScopedIdentifier{Token: scopeToken, Outer: ident}
	if !p.peekTokenOneOf(token.CONST, token.IDENT) {
		p.peekError(token.CONST)
		return nil
	}
	p.nextToken()
	scopedIdent.Inner = p.parseIdentifier()
	for p.peekTokenIs(token.SCOPE) {
		p.nextToken()
		scopeToken = p.curToken
		if !p.peekTokenOneOf(token.CONST, token.IDENT) {
			p.peekError(token.CONST)
			return nil
		}
		p.nextToken()
		outerIdent := &ast.Identifier{
			Token: scopedIdent.Outer.Token,
			Value: scopedIdent.String(),
		}
		scopedIdent = &ast.ScopedIdentifier{
			Token: scopeToken,
			Outer: outerIdent,
			Inner: p.parseIdentifier(),
		}
	}
	return scopedIdent
}

func (p *parser) parseSelf() ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	self := &ast.Self{Token: p.curToken}
	if p.peekTokenOneOf(token.IF, token.UNLESS) {
		return self
	}
	// self followed by an identifier or constant without a dot is an error
	if p.peekTokenOneOf(token.IDENT, token.CONST) {
		p.peekError(token.DOT)
		return nil
	}
	return self
}

func (p *parser) parseKeyword__FILE__() ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	file := &ast.Keyword__FILE__{
		Token:    p.curToken,
		Filename: p.file.Name(),
	}
	return file
}

func (p *parser) parseKeyword__LINE__() ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	line := p.file.Position(p.pos).Line
	return &ast.IntegerLiteral{Token: p.curToken, Value: int64(line)}
}

func (p *parser) parseBeginBlock() ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	block := &ast.BeginBlock{Token: p.curToken}
	if !p.accept(token.LBRACE) {
		return nil
	}
	block.Body = p.parseBlockStatement(token.RBRACE)
	p.nextToken() // consume }
	return block
}

func (p *parser) parseEndBlock() ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	block := &ast.EndBlock{Token: p.curToken}
	if !p.accept(token.LBRACE) {
		return nil
	}
	block.Body = p.parseBlockStatement(token.RBRACE)
	p.nextToken() // consume }
	return block
}

func (p *parser) parseUsing() ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	// `using(...)` (no whitespace before `(`) is a normal method call --
	// MRI doesn't treat `using` as a keyword. Route to the call path so
	// args / block parse the standard way.
	if p.peekTokenIs(token.LPAREN) && !p.peekToken.HadWhitespace {
		ident := &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
		p.nextToken() // -> LPAREN
		return p.parseCallExpressionWithParens(ident)
	}
	expr := &ast.UsingExpression{Token: p.curToken}
	p.nextToken()
	expr.Expr = p.parseExpression(precLowest)
	return expr
}

func (p *parser) parseRefine() ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	// `refine(...)` (no whitespace before `(`) is a normal method call on
	// the surrounding Module -- handle as a paren-less ident so the call
	// args / block parse the standard way (matches MRI, which doesn't
	// treat `refine` as a keyword).
	if p.peekTokenIs(token.LPAREN) && !p.peekToken.HadWhitespace {
		ident := &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
		p.nextToken() // -> LPAREN
		return p.parseCallExpressionWithParens(ident)
	}
	expr := &ast.RefineExpression{Token: p.curToken}
	p.nextToken()
	// Parse the refined class at precBlockBraces so that a trailing
	// do/brace is NOT consumed as a method-call block (parseCallBlock).
	// However, parseMethodCall (DOT handler) unconditionally consumes
	// DO/LBRACE, so a target like String.singleton_class will still
	// absorb the block. Detect that case below.
	expr.Expr = p.parseExpression(precIfUnless)
	if expr.Expr == nil {
		return nil
	}
	// If the expression already has a block (consumed by parseMethodCall
	// for cases like String.singleton_class do...end), the DO/body/end
	// are already consumed. Just record the end token and return.
	if cc, ok := expr.Expr.(*ast.ContextCallExpression); ok && cc.Block != nil {
		expr.Body = cc.Block.Body
		expr.EndToken = cc.Block.EndToken
		cc.Block = nil // Move ownership -- avoid double printing in String()
		return expr
	}
	endToken := token.END
	p.nextToken()
	if p.currentTokenIs(token.LBRACE) {
		endToken = token.RBRACE
	} else if p.currentTokenIs(token.DO) {
		// do...end block
	} else {
		// No block (e.g. `refine c1` without do/{ -- bare call)
		return expr
	}
	expr.Body = p.parseBlockStatement(endToken)
	if !p.accept(endToken) {
		return nil
	}
	expr.EndToken = p.curToken
	return expr
}

func (p *parser) parseKeyword__CALLEE__() ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	return &ast.Keyword__CALLEE__{Token: p.curToken}
}

func (p *parser) parseKeyword__METHOD__() ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	return &ast.Keyword__METHOD__{Token: p.curToken}
}

func (p *parser) parseKeyword__DIR__() ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	return &ast.Keyword__DIR__{Token: p.curToken}
}

func (p *parser) parseEncodingKeyword() ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	return &ast.Keyword__ENCODING__{Token: p.curToken}
}

func (p *parser) parseSplatExpression() ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	expr := &ast.SplatExpression{Token: p.curToken, Operator: p.curToken.Literal}
	// Double-splat for keyword-arg spread arrived in Ruby 2.0.
	if expr.Operator == "**" && !p.version.AtLeast(ruby20) {
		p.versionError(ruby20, "double-splat (**) in arguments")
	}
	// Anonymous forwarding: bare * or ** as argument (ruby 3.2+)
	if p.peekTokenOneOf(token.COMMA, token.RPAREN, token.RBRACKET, token.RBRACE,
		token.NEWLINE, token.SEMICOLON, token.EOF, token.ASSIGN) {
		return expr
	}
	p.nextToken()
	// Splat absorbs the full arg expression up to the next comma -- including
	// range (`*a..z`) and other infix forms MRI treats as one splattable arg.
	// precAssignment stops at COMMA and HASHROCKET, both of which are arg /
	// hash separators, not part of the splatted value.
	expr.Right = p.parseExpression(precAssignment)
	return expr
}

func (p *parser) parseYield() ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	yield := &ast.YieldExpression{Token: p.curToken}
	if p.peekTokenOneOf(token.NEWLINE, token.SEMICOLON, token.END, token.EOF, token.RBRACE, token.RPAREN, token.RBRACKET, token.EMBEXPR_END,
		token.IF, token.UNLESS, token.WHILE, token.UNTIL, token.RESCUE, token.DOT) {
		return yield
	}
	p.nextToken()
	if p.currentTokenOneOf(token.LBRACE, token.DO) {
		yield.Block = p.parseBlockExpr()
		return yield
	}
	if p.currentTokenIs(token.LPAREN) {
		p.nextToken()
		yield.Arguments = p.parseCallArguments(token.RPAREN)
		return yield
	}
	yield.Arguments = p.parseCallArguments(token.SEMICOLON, token.NEWLINE, token.LBRACE, token.DO)
	return yield
}

func (p *parser) parseSuper() ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	sup := &ast.SuperExpression{Token: p.curToken}
	if p.peekTokenOneOf(token.NEWLINE, token.SEMICOLON, token.END, token.EOF, token.RBRACE, token.RPAREN, token.RBRACKET, token.EMBEXPR_END, token.DOT, token.LONELY,
		token.IF, token.UNLESS, token.WHILE, token.UNTIL, token.RESCUE) {
		return sup
	}
	// If peekToken has no prefix handler, it can't start an argument.
	// Return bare super so the expression loop handles it as infix.
	if prefixParseFns[p.peekToken.Type] == nil && !p.peekTokenOneOf(token.LBRACE, token.DO, token.LPAREN) {
		return sup
	}
	// `super + x` (spaced binary operator) is binary infix on bare super,
	// not a paren-less call with unary +x. Same rule as for bare method
	// calls (see parseMethodCall's spaced-operator branch).
	if p.peekToken.HadWhitespace && p.peekToken.Type.IsOperator() &&
		!p.peek2TokenIs(token.NEWLINE) && !p.peek2TokenIs(token.EOF) &&
		p.spacedOperator(p.peekToken, p.peek2Token) {
		return sup
	}
	p.nextToken()
	if p.currentTokenOneOf(token.LBRACE, token.DO) {
		sup.Block = p.parseBlockExpr()
		return sup
	}
	if p.currentTokenIs(token.LPAREN) {
		p.nextToken()
		sup.Arguments = p.parseCallArguments(token.RPAREN)
		if sup.Arguments == nil {
			sup.Arguments = []ast.Expression{}
		}
		if p.peekTokenOneOf(token.LBRACE, token.DO) {
			p.nextToken()
			sup.Block = p.parseBlockExpr()
		}
		return sup
	}
	sup.Arguments = p.parseCallArguments(token.SEMICOLON, token.NEWLINE, token.EOF, token.LBRACE, token.DO, token.RPAREN, token.RBRACKET)
	return sup
}

func (p *parser) parseAliasName() *ast.Identifier {
	if p.currentTokenOneOf(token.IDENT, token.CONST) || p.curToken.Type.IsKeyword() || p.curToken.Type.IsOperator() {
		name := p.curToken.Literal
		if p.peekTokenIs(token.ASSIGN) {
			name += "="
			p.nextToken()
		}
		return &ast.Identifier{Token: p.curToken, Value: name}
	}
	if p.currentTokenIs(token.GLOBAL) {
		return &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
	}
	if p.currentTokenIs(token.LBRACKET) && p.peekTokenIs(token.RBRACKET) {
		p.nextToken()
		name := "[]"
		if p.peekTokenIs(token.ASSIGN) {
			name = "[]="
			p.nextToken()
		}
		return &ast.Identifier{Token: p.curToken, Value: name}
	}
	if p.currentTokenIs(token.SYMBEG) {
		sym := p.parseSymbolLiteral()
		if sym != nil {
			return &ast.Identifier{Token: p.curToken, Value: sym.String()}
		}
	}
	return nil
}

func (p *parser) parseAlias() ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	expr := &ast.AliasExpression{Token: p.curToken}
	p.nextToken()
	expr.NewName = p.parseAliasName()
	if expr.NewName == nil {
		p.errors = append(p.errors, &parseError{
			Pos:  p.file.Position(p.pos),
			Kind: SyntaxError,
			Msg:  "alias requires a new name",
		})
		return nil
	}
	p.nextToken()
	expr.OldName = p.parseAliasName()
	if expr.OldName == nil {
		p.errors = append(p.errors, &parseError{
			Pos:  p.file.Position(p.pos),
			Kind: SyntaxError,
			Msg:  "alias requires an old name",
		})
		return nil
	}
	return expr
}

func (p *parser) parseUndef() ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	expr := &ast.UndefExpression{Token: p.curToken}
	p.nextToken()
	expr.Names = []*ast.Identifier{parseUndefName(p)}
	for p.peekTokenIs(token.COMMA) {
		p.consume(token.COMMA)
		expr.Names = append(expr.Names, parseUndefName(p))
	}
	return expr
}

// parseUndefName parses a single undef target (identifier, symbol, or operator).
func parseUndefName(p *parser) *ast.Identifier {
	if p.currentTokenIs(token.SYMBEG) {
		sym := p.parseSymbolLiteral()
		if sym == nil {
			return &ast.Identifier{Token: p.curToken, Value: ""}
		}
		return &ast.Identifier{Token: p.curToken, Value: sym.(*ast.SymbolLiteral).Value.String()}
	}
	if p.currentTokenIs(token.LBRACKET) && p.peekTokenIs(token.RBRACKET) {
		p.nextToken()
		name := "[]"
		if p.peekTokenIs(token.ASSIGN) {
			name = "[]="
			p.nextToken()
		}
		return &ast.Identifier{Token: p.curToken, Value: name}
	}
	return &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
}

func (p *parser) parseRangeOrForwarding() ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	tok := p.curToken
	// If the next token is a terminator, ... is argument forwarding (ruby 2.7+)
	if p.peekTokenOneOf(token.RPAREN, token.COMMA, token.RBRACKET, token.NEWLINE, token.SEMICOLON, token.EOF) {
		if !p.version.AtLeast(ruby27) {
			p.versionError(ruby27, "argument forwarding (...)")
		}
		return &ast.ArgumentForwarding{Token: tok}
	}
	p.nextToken()
	return &ast.InfixExpression{
		Token:    tok,
		Left:     nil,
		Operator: tok.Literal,
		Right:    p.parseExpression(precLessGreater),
	}
}

func (p *parser) parseBeginlessRange() ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	if !p.version.AtLeast(ruby27) {
		p.versionError(ruby27, "beginless range")
	}
	tok := p.curToken
	p.nextToken()
	return &ast.InfixExpression{
		Token:    tok,
		Left:     nil,
		Operator: tok.Literal,
		Right:    p.parseExpression(precLessGreater),
	}
}

func (p *parser) parseRightwardAssignment(left ast.Expression) ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	tok := p.curToken
	p.nextToken()
	for p.currentTokenIs(token.NEWLINE) {
		p.nextToken()
	}
	p.inPattern = true
	right := p.parsePattern()
	p.inPattern = false
	return &ast.RightwardAssignment{
		Token: tok,
		Left:  left,
		Right: right,
	}
}

func (p *parser) parseLambda() ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	lit := &ast.FunctionLiteral{Token: p.curToken, IsLambda: true}
	// Optional parameters: ->(x, y) or bare ->
	if p.peekTokenIs(token.LPAREN) {
		// MRI 1.9 rejects whitespace between `->` and `(`. 2.0+ accepts both.
		if p.peekToken.HadWhitespace && !p.version.AtLeast(ruby20) {
			p.versionError(ruby20, "whitespace between `->` and parameter list")
		}
		lit.Parameters = p.parseParameters(token.LPAREN, token.RPAREN)
		lit.ExplicitParens = true
	}
	if p.currentTokenOneOf(token.CAPTURE, token.AND) {
		if !p.peekTokenOneOf(token.LBRACE, token.DO) {
			capture := p.parseBlockCapture()
			if capture == nil {
				return nil
			}
			lit.CapturedBlock = capture.(*ast.BlockCapture)
			if p.peekTokenIs(token.RPAREN) {
				p.accept(token.RPAREN)
			}
		}
	}
	// Body must be a block: { ... } or do ... end
	if p.peekTokenOneOf(token.LBRACE, token.DO) {
		p.acceptOneOf(token.LBRACE, token.DO)
		block := p.parseBlock()
		blk, ok := block.(*ast.BlockExpression)
		if !ok {
			return nil
		}
		if lit.Parameters == nil {
			lit.Parameters = blk.Parameters
		}
		lit.Body = blk.Body
		lit.EndToken = blk.EndToken
	}
	return lit
}

var integerLiteralReplacer = strings.NewReplacer("_", "")

func (p *parser) parseIntegerLiteral() ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	lit := &ast.IntegerLiteral{Token: p.curToken}
	s := integerLiteralReplacer.Replace(p.curToken.Literal)
	s = strings.TrimRight(s, "riRI")
	v, bigV, err := parseRubyInt(s)
	if err != nil {
		p.errors = append(p.errors, &parseError{
			Pos:  p.file.Position(p.pos),
			Kind: SyntaxError,
			Msg:  fmt.Sprintf("could not parse %q as integer", p.curToken.Literal),
		})
		return nil
	}
	if bigV != nil {
		lit.BigInt = bigV
	} else {
		lit.Value = v
	}
	return lit
}

func (p *parser) parseFloatLiteral() ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	lit := &ast.FloatLiteral{Token: p.curToken}
	s := integerLiteralReplacer.Replace(p.curToken.Literal)
	// Strip rational/complex suffixes -- the numeric value is the same.
	s = strings.TrimRight(s, "riRI")
	value, err := parseFloat(s)
	if err != nil {
		p.errors = append(p.errors, &parseError{
			Pos:  p.file.Position(p.pos),
			Kind: SyntaxError,
			Msg:  fmt.Sprintf("could not parse %q as float", p.curToken.Literal),
		})
		return nil
	}
	lit.Value = value
	return lit
}

// parseRubyInt parses a Ruby integer literal, handling hex (0x), binary (0b),
// octal (0o, 0O), explicit decimal (0d, 0D), leading-zero decimal (09), and
// underscores. When the value overflows int64, the *big.Int result is non-nil.
func parseRubyInt(s string) (int64, *big.Int, error) {
	base := 10
	switch {
	case len(s) >= 2 && s[0] == '0':
		switch s[1] {
		case 'x', 'X':
			base = 16
			s = s[2:]
		case 'b', 'B':
			base = 2
			s = s[2:]
		case 'o', 'O':
			base = 8
			s = s[2:]
		case 'd', 'D':
			base = 10
			s = s[2:]
		default:
			// Leading zero without base prefix is decimal in Ruby
			// (e.g. 09 is decimal 9, not invalid octal).
			base = 10
		}
	}
	if s == "" {
		return 0, nil, fmt.Errorf("empty number")
	}
	// Fast path: try int64 first to skip the big.Int allocation for values
	// that fit (the common case). On overflow (errors.Is ErrRange) fall back
	// to big.Int. Other errors (invalid digits) are real and reported as-is.
	if v, err := strconv.ParseInt(s, base, 64); err == nil {
		return v, nil, nil
	} else if ne, ok := err.(*strconv.NumError); !ok || ne.Err != strconv.ErrRange {
		return 0, nil, fmt.Errorf("invalid digits for base %d: %q", base, s)
	}
	bi := new(big.Int)
	bi, ok := bi.SetString(s, base)
	if !ok {
		return 0, nil, fmt.Errorf("invalid digits for base %d: %q", base, s)
	}
	if bi.IsInt64() {
		return bi.Int64(), nil, nil
	}
	return 0, bi, nil
}

// parseFloat is like strconv.ParseFloat but handles Ruby float edge cases.
func parseFloat(s string) (float64, error) {
	if s == "" {
		return 0, fmt.Errorf("empty float")
	}
	return strconv.ParseFloat(s, 64)
}

func (p *parser) parseStringLiteral() ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	return &ast.StringLiteral{Token: p.curToken, Value: p.curToken.Literal}
}

func (p *parser) parseInterpolatedString() ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	begToken := p.curToken // STRING_BEG or XSTR_BEG
	p.nextToken()          // advance to first content token

	var parts []ast.Expression
	for !p.currentTokenIs(token.STRING_END) && !p.currentTokenIs(token.XSTR_END) && !p.currentTokenIs(token.EOF) {
		switch p.curToken.Type {
		case token.STRING_CONTENT, token.XSTR_CONTENT:
			parts = append(parts, &ast.StringContent{Token: p.curToken, Value: p.curToken.Literal})
		case token.EMBEXPR_BEG:
			embTok := p.curToken
			p.nextToken() // advance past EMBEXPR_BEG to first expression token
			for p.currentTokenOneOf(token.NEWLINE, token.SEMICOLON) {
				p.nextToken()
			}
			if p.currentTokenIs(token.EMBEXPR_END) {
				// Empty `#{ }` -- preserve as DSTR-shaping placeholder so
				// MRI re-parses to EVSTR(BEGIN(nil)) instead of collapsing
				// to a static STR. See same handling in parseInterpolatedRegex.
				parts = append(parts, &ast.ParenExpression{Token: embTok, Rparen: p.curToken})
				break
			}
			p.embExprDepth++
			var stmts []ast.Expression
			for !p.currentTokenIs(token.EMBEXPR_END) && !p.currentTokenIs(token.EOF) {
				exp := p.parseExpression(precLowest)
				if exp != nil {
					stmts = append(stmts, exp)
				}
				for p.peekTokenOneOf(token.NEWLINE, token.SEMICOLON) {
					p.nextToken()
				}
				if p.peekTokenIs(token.EMBEXPR_END) {
					break
				}
				p.nextToken()
			}
			p.embExprDepth--
			if len(stmts) == 1 {
				parts = append(parts, stmts[0])
			} else if len(stmts) > 1 {
				// Multi-statement interp `"#{a; b; c}"` -- MRI keeps all
				// statements under EVSTR. Wrap in ParenExpression.Stmts.
				parts = append(parts, &ast.ParenExpression{Token: embTok, Rparen: p.curToken, Stmts: stmts})
			}
			if !p.peekTokenIs(token.EMBEXPR_END) {
				p.peekError(token.EMBEXPR_END)
				return nil
			}
			p.nextToken() // consume EMBEXPR_END
		case token.GLOBAL:
			parts = append(parts, &ast.EmbeddedVariable{
				Variable: &ast.Global{Token: p.curToken, Value: p.curToken.Literal},
			})
		case token.AT:
			ivar := &ast.InstanceVariable{Token: p.curToken}
			p.nextToken()
			ivar.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
			parts = append(parts, &ast.EmbeddedVariable{Variable: ivar})
		case token.CLASS_VAR:
			cv := &ast.ClassVariable{Token: p.curToken}
			p.nextToken()
			cv.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
			parts = append(parts, &ast.EmbeddedVariable{Variable: cv})
		default:
			p.expectError(token.STRING_CONTENT, token.EMBEXPR_BEG, token.STRING_END)
			return nil
		}
		p.nextToken()
	}

	// Branch based on percent literal type.
	switch begToken.Literal {
	case "w", "W":
		return p.buildWordArray(begToken, parts, false)
	case "i", "I":
		return p.buildWordArray(begToken, parts, true)
	case "s":
		return p.buildSymbolFromPercent(begToken, parts)
	default: // "", "Q", "q", "x" -- string/regex/xstr
	}

	sl := &ast.StringLiteral{Token: begToken}
	if strings.HasPrefix(begToken.Literal, "<<") {
		sl.HeredocTag = begToken.Literal
		if p.curToken.HeredocStripped || p.embExprDepth > 0 {
			sl.HeredocStripped = true
		}
	}
	// Optimisation: simple string without interpolation.
	if len(parts) == 1 {
		if sc, ok := parts[0].(*ast.StringContent); ok {
			sl.Value = sc.Value
			return sl
		}
	}
	if len(parts) == 0 {
		sl.Value = ""
		return sl
	}
	sl.Parts = parts
	return sl
}

func (p *parser) parseInterpolatedRegex() ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	rl := &ast.RegexLiteral{Token: p.curToken} // REGEX_BEG
	p.nextToken()                              // advance to first content token

	var parts []ast.Expression
	for !p.currentTokenIs(token.REGEX_END) && !p.currentTokenIs(token.EOF) {
		switch p.curToken.Type {
		case token.STRING_CONTENT:
			parts = append(parts, &ast.StringContent{Token: p.curToken, Value: p.curToken.Literal})
		case token.EMBEXPR_BEG:
			embTok := p.curToken
			p.nextToken()
			for p.currentTokenOneOf(token.NEWLINE, token.SEMICOLON) {
				p.nextToken()
			}
			if p.currentTokenIs(token.EMBEXPR_END) {
				// Empty interpolation `#{ }`: MRI emits NODE_EVSTR wrapping
				// a NODE_BEGIN with nil body, which preserves the DREGX/DSTR
				// shape (not a static REGX/STR). Use ParenExpression{Expr:
				// nil} as a placeholder; it prints as `#{()}` which MRI
				// parses back to the same EVSTR(BEGIN(nil)) shape.
				parts = append(parts, &ast.ParenExpression{Token: embTok, Rparen: p.curToken})
				break
			}
			var stmts []ast.Expression
			for !p.currentTokenIs(token.EMBEXPR_END) && !p.currentTokenIs(token.EOF) {
				exp := p.parseExpression(precLowest)
				if exp != nil {
					stmts = append(stmts, exp)
				}
				for p.peekTokenOneOf(token.NEWLINE, token.SEMICOLON) {
					p.nextToken()
				}
				if p.peekTokenIs(token.EMBEXPR_END) {
					break
				}
				p.nextToken()
			}
			if len(stmts) == 1 {
				parts = append(parts, stmts[0])
			} else if len(stmts) > 1 {
				// Multi-statement interp `#{a; b; c}` -- MRI preserves the
				// statement sequence under the EVSTR/DREGX. Wrap in
				// ParenExpression.Stmts so the printer re-emits as `(a; b; c)`
				// inside `#{...}`.
				parts = append(parts, &ast.ParenExpression{Token: embTok, Rparen: p.curToken, Stmts: stmts})
			}
			if !p.peekTokenIs(token.EMBEXPR_END) {
				p.peekError(token.EMBEXPR_END)
				return nil
			}
			p.nextToken()
		case token.AT:
			exp := p.parseInstanceVariable()
			if exp != nil {
				parts = append(parts, &ast.EmbeddedVariable{Variable: exp})
			}
		case token.CLASS_VAR:
			exp := p.parseClassVariable()
			if exp != nil {
				parts = append(parts, &ast.EmbeddedVariable{Variable: exp})
			}
		case token.GLOBAL:
			parts = append(parts, &ast.EmbeddedVariable{Variable: p.parseGlobal()})
		default:
			p.expectError(token.STRING_CONTENT, token.EMBEXPR_BEG, token.REGEX_END)
			return nil
		}
		p.nextToken()
	}

	// REGEX_END literal carries the options flags.
	rl.Options = p.curToken.Literal

	if len(parts) == 1 {
		if sc, ok := parts[0].(*ast.StringContent); ok {
			rl.Value = sc.Value
			return rl
		}
	}
	if len(parts) == 0 {
		rl.Value = ""
		return rl
	}
	rl.Parts = parts
	return rl
}

var symbolOperatorTokens = []token.Type{
	token.PLUS, token.MINUS, token.ASTERISK, token.SLASH, token.MODULO,
	token.POWER, token.LSHIFT, token.RSHIFT,
	token.SPACESHIP, token.EQ, token.NOTEQ, token.MATCH, token.NMATCH,
	token.CASEEQ,
	token.LT, token.GT, token.LTE, token.GTE,
	token.XOR, token.PIPE, token.AND, token.TILDE,
	token.BANG,
}

var symbolKeywordTokens = []token.Type{
	token.DEF, token.SELF, token.END, token.IF, token.THEN, token.ELSE,
	token.UNLESS, token.TRUE, token.FALSE, token.RETURN, token.NIL,
	token.MODULE, token.CLASS, token.DO, token.YIELD,
	token.BEGIN, token.RESCUE, token.WHILE, token.UNTIL, token.CASE, token.WHEN,
	token.BREAK, token.NEXT,
	token.KW_UNDEF, token.KW_SUPER, token.KW_RETRY, token.KW_REDO,
	token.KW_OR, token.KW_NOT, token.KW_IN, token.KW_FOR,
	token.KW_ENSURE, token.KW_ELSIF, token.KW_DEFINED, token.KW_AND, token.KW_ALIAS,
	token.KW_BEGIN, token.KW_END, token.KW_USING, token.KW_REFINE,
}

func (p *parser) parseSymbolLiteral() ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	symbol := &ast.SymbolLiteral{Token: p.curToken}

	accepted := []token.Type{
		token.IDENT, token.CONST, token.AT, token.STRING, token.STRING_BEG,
		token.CLASS_VAR, token.GLOBAL, token.LBRACKET,
	}
	accepted = append(accepted, symbolOperatorTokens...)
	accepted = append(accepted, symbolKeywordTokens...)
	if !p.acceptOneOf(accepted...) {
		return nil
	}
	if p.currentTokenOneOf(symbolOperatorTokens...) {
		name := p.curToken.Literal
		if (name == "+" || name == "-") && p.peekTokenIs(token.AT) {
			name += "@"
			p.nextToken()
		}
		symbol.Value = &ast.Identifier{Token: p.curToken, Value: name}
		return symbol
	}
	if p.currentTokenOneOf(symbolKeywordTokens...) {
		symbol.Value = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
		return symbol
	}
	// Setter symbol: :foo= -- IDENT followed by = with no space
	if p.currentTokenOneOf(token.IDENT, token.CONST) && p.peekTokenIs(token.ASSIGN) {
		name := p.curToken.Literal + "="
		p.nextToken() // consume =
		symbol.Value = &ast.Identifier{Token: p.curToken, Value: name}
		return symbol
	}
	if p.currentTokenIs(token.LBRACKET) {
		lit := "[]"
		if p.peekTokenIs(token.RBRACKET) {
			p.nextToken()
			if p.peekTokenIs(token.ASSIGN) {
				p.nextToken()
				lit = "[]="
			}
		}
		symbol.Value = &ast.Identifier{Token: p.curToken, Value: lit}
		return symbol
	}
	val := p.parseExpression(precHighest)
	symbol.Value = val
	return symbol
}

func (p *parser) parseArrayLiteral() ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	array := &ast.ArrayLiteral{Token: p.curToken}

	p.nextToken()
	array.Elements = p.parseExpressionList(token.RBRACKET)
	array.Rbracket = p.curToken
	return array
}

func (p *parser) parseBoolean() ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	return &ast.Boolean{Token: p.curToken, Value: p.currentTokenIs(token.TRUE)}
}

func (p *parser) parseHash() ast.Expression {
	hash := &ast.HashLiteral{Token: p.curToken, Map: ast.NewOrderedExprMap()}
	defer trace.TraceCtx(p.ctx)()
	p.nextToken()
	// Skip leading newlines/semicolons inside the hash.
	for p.currentTokenOneOf(token.NEWLINE, token.SEMICOLON) {
		p.nextToken()
	}

	if p.currentTokenIs(token.RBRACE) {
		hash.Rbrace = p.curToken
		return hash
	}

	// Handle **expr keyword splat as first hash entry
	if p.currentTokenIs(token.POWER) {
		p.nextToken()
		hash.Splats = append(hash.Splats, p.parseExpression(precAssignment))
	} else {
		k, v, ok, omitted := p.parseKeyValue()
		if !ok {
			return nil
		}
		hash.Map.Set(k, v)
		if omitted {
			hash.Map.SetOmitted(k)
		}
	}

	for p.peekTokenIs(token.COMMA) {
		p.consume(token.COMMA)
		// Skip newlines after commas.
		for p.currentTokenOneOf(token.NEWLINE, token.SEMICOLON) {
			p.nextToken()
		}
		if p.currentTokenIs(token.RBRACE) {
			hash.Rbrace = p.curToken
			return hash
		}
		for p.peekTokenOneOf(token.NEWLINE, token.SEMICOLON) {
			p.acceptOneOf(token.NEWLINE, token.SEMICOLON)
		}
		// Handle **expr keyword splat in hash (or bare ** for anonymous forwarding)
		if p.currentTokenIs(token.POWER) {
			if p.peekTokenOneOf(token.RBRACE, token.COMMA, token.NEWLINE) {
				hash.Splats = append(hash.Splats, &ast.SplatExpression{
					Token: p.curToken, Operator: "**",
				})
				continue
			}
			p.nextToken()
			hash.Splats = append(hash.Splats, p.parseExpression(precAssignment))
			continue
		}
		k, v, ok, omitted := p.parseKeyValue()
		if !ok {
			return nil
		}
		hash.Map.Set(k, v)
		if omitted {
			hash.Map.SetOmitted(k)
		}
	}

	for p.peekTokenOneOf(token.NEWLINE, token.SEMICOLON) {
		p.acceptOneOf(token.NEWLINE, token.SEMICOLON)
	}
	if !p.accept(token.RBRACE) {
		return nil
	}
	hash.Rbrace = p.curToken
	return hash
}

func (p *parser) parseKeyValue() (ast.Expression, ast.Expression, bool, bool) {
	defer trace.TraceCtx(p.ctx)()
	// Label syntax: key: value (Ruby 1.9+)
	if p.currentTokenIs(token.LABEL) {
		name := strings.TrimSuffix(p.curToken.Literal, ":")
		key := &ast.SymbolLiteral{
			Token: p.curToken,
			Value: &ast.StringLiteral{Value: name},
		}
		// Hash value omission (ruby 3.1+): {x:, y:} == {x: x, y: y}
		if p.peekTokenOneOf(token.COMMA, token.RBRACE, token.RPAREN, token.NEWLINE) {
			if !p.version.AtLeast(ruby31) {
				p.versionError(ruby31, "hash value omission")
			}
			val := &ast.Identifier{Token: p.curToken, Value: name}
			return key, val, true, true // isLabel, omitted
		}
		p.nextToken()
		val := p.parseExpression(precAssignment)
		return key, val, true, false // isLabel, not omitted
	}
	// Classic hashrocket syntax: key => value
	key := p.parseExpression(precAssignment)
	// Quoted label syntax (ruby 3.1+): "key": value
	if p.peekTokenOneOf(token.SYMBEG, token.COLON) {
		if _, ok := key.(*ast.StringLiteral); ok {
			p.nextToken()
			if p.peekTokenOneOf(token.COMMA, token.RBRACE, token.RPAREN, token.NEWLINE) {
				val := key
				return &ast.SymbolLiteral{Token: p.curToken, Value: key.(*ast.StringLiteral)}, val, true, false
			}
			p.nextToken()
			val := p.parseExpression(precAssignment)
			return &ast.SymbolLiteral{Token: p.curToken, Value: key.(*ast.StringLiteral)}, val, true, false
		}
	}
	if !p.consume(token.HASHROCKET) {
		return nil, nil, false, false
	}
	val := p.parseExpression(precAssignment)
	return key, val, true, false
}

func (p *parser) parseBlockExpr() *ast.BlockExpression {
	blk := p.parseBlock()
	if blk == nil {
		return nil
	}
	return blk.(*ast.BlockExpression)
}

func (p *parser) parseBlock() ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	// blocks introduce a new statement scope; the paren-less-call kw and/or
	// suppression from the enclosing call must not leak into the body.
	prevAO := p.suppressKwAndOr
	p.suppressKwAndOr = false
	defer func() { p.suppressKwAndOr = prevAO }()
	block := &ast.BlockExpression{Token: p.curToken}
	if p.peekTokenIs(token.NEWLINE) && p.peek2TokenIs(token.PIPE) {
		p.nextToken()
	}
	if p.peekTokenIs(token.PIPE) {
		block.HasParameterBars = true
		block.Parameters = p.parseParameters(token.PIPE, token.PIPE)
		if p.currentTokenOneOf(token.CAPTURE, token.AND) {
			capture := p.parseBlockCapture()
			if capture != nil {
				block.CapturedBlock = capture.(*ast.BlockCapture)
			}
			if p.peekTokenIs(token.PIPE) {
				p.accept(token.PIPE)
			}
		}
		// Block-local variables: |params; locals|
		if (p.peekTokenIs(token.SEMICOLON) || p.currentTokenIs(token.SEMICOLON)) && !p.currentTokenIs(token.PIPE) {
			if p.peekTokenIs(token.SEMICOLON) {
				p.accept(token.SEMICOLON)
			}
			for !p.peekTokenIs(token.PIPE) {
				if !p.accept(token.IDENT) {
					return nil
				}
				block.BlockLocals = append(block.BlockLocals,
					&ast.Identifier{Token: p.curToken, Value: p.curToken.Literal})
				if p.peekTokenIs(token.COMMA) {
					p.accept(token.COMMA)
				}
			}
			p.accept(token.PIPE)
		}
	}

	if p.currentTokenOneOf(token.CAPTURE, token.AND) {
		if !p.peekTokenOneOf(token.NEWLINE, token.SEMICOLON, token.EOF) {
			capture := p.parseBlockCapture()
			if capture == nil {
				return nil
			}
			block.CapturedBlock = capture.(*ast.BlockCapture)
			if p.peekTokenIs(token.PIPE) {
				p.accept(token.PIPE)
			}
		}
	}

	if p.peekTokenOneOf(token.NEWLINE, token.SEMICOLON) {
		p.acceptOneOf(token.NEWLINE, token.SEMICOLON)
	}

	endToken := token.RBRACE
	if block.Token.Type == token.DO {
		endToken = token.END
	}

	if endToken == token.END {
		rescueInBlock := p.version.AtLeast(ruby25)
		if rescueInBlock {
			block.Body = p.parseBlockStatement(token.END, token.RESCUE, token.KW_ENSURE)
		} else {
			block.Body = p.parseBlockStatement(token.END)
		}
		for rescueInBlock && p.peekTokenIs(token.RESCUE) {
			p.accept(token.RESCUE)
			rescue := p.parseRescueBlock()
			block.Rescues = append(block.Rescues, rescue)
		}
		if rescueInBlock && p.peekTokenIs(token.ELSE) {
			p.accept(token.ELSE)
			p.acceptOneOf(token.NEWLINE, token.SEMICOLON)
			block.ElseBody = p.parseBlockStatement(token.END, token.KW_ENSURE)
		}
		if rescueInBlock && p.peekTokenIs(token.KW_ENSURE) {
			p.accept(token.KW_ENSURE)
			p.acceptOneOf(token.NEWLINE, token.SEMICOLON)
			block.EnsureBody = p.parseBlockStatement(token.END)
		}
	} else {
		block.Body = p.parseBlockStatement(endToken)
	}
	p.nextToken()
	block.EndToken = p.curToken
	return block
}

func (p *parser) parsePrefixExpression() ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	expression := &ast.PrefixExpression{
		Token:    p.curToken,
		Operator: p.curToken.Literal,
	}
	// `not(X)` (no space) is call-style syntax for `not X` in MRI -- the
	// parens belong to the not-call, not to a grouping ParenExpression
	// around X. `not (X)` (with space) is real grouping (preserved).
	if expression.Operator == "not" && p.peekTokenIs(token.LPAREN) && !p.peekToken.HadWhitespace {
		p.nextToken() // to LPAREN
		p.nextToken() // past LPAREN
		expression.Right = p.parseExpression(precLowest)
		if p.peekTokenIs(token.RPAREN) {
			p.accept(token.RPAREN)
		}
		return expression
	}
	p.nextToken()
	expression.Right = p.parseExpression(precPrefix)
	return expression
}

func (p *parser) parseInfixExpression(left ast.Expression) ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	expression := &ast.InfixExpression{
		Token:    p.curToken,
		Operator: p.curToken.Literal,
		Left:     left,
	}
	precedence := p.curPrecedence()
	if expression.Operator == ".." || expression.Operator == "..." {
		if p.peekTokenOneOf(token.EOF, token.NEWLINE, token.SEMICOLON,
			token.RPAREN, token.RBRACKET, token.RBRACE, token.COMMA, token.PIPE) {
			if !p.version.AtLeast(ruby26) {
				p.versionError(ruby26, "endless range")
			}
			return expression
		}
	}
	// `and` / `or` have lower precedence than `=` in Ruby, so their RHS
	// must be allowed to absorb assignment. Lower the precedence wholesale
	// for those two; chained occurrences end up right-associative but
	// remain semantically equivalent.
	switch expression.Operator {
	case "and", "or":
		precedence = precAssignment - 1
	case "**":
		// ** is right-associative: a ** b ** c parses as a ** (b ** c).
		precedence--
	}
	p.nextToken()
	for p.currentTokenIs(token.NEWLINE) {
		p.nextToken()
	}
	expression.Right = p.parseExpression(precedence)
	// MRI: &&, ||, << RHS absorbs assignment.
	//   a && b = c  ->  a && (b = c)
	//   a || b = c  ->  a || (b = c)
	//   a << b = c  ->  a << (b = c)
	// Can't lower precedence wholesale -- that would absorb chained operators
	// too, making them right-associative (a || b || c -> a || (b || c)).
	// Post-process to absorb only the assignment.
	switch expression.Operator {
	case "&&", "||", "<<":
		if p.peekTokenIs(token.ASSIGN) && isAssignableTarget(expression.Right) {
			p.nextToken() // = becomes current
			expression.Right = p.parseAssignment(expression.Right)
		}
	case "and", "or":
		// We parsed RHS at precAssignment-1 above so it would absorb `=` /
		// ternary / etc., but that makes chained `and`/`or` right-associative
		// (a and b and c -> a and (b and c), and the mixed case
		// a and b or c -> a and (b or c)). MRI is left-associative for both
		// `and` and `or` (same precedence). Flatten across same-precedence
		// `and`/`or` nodes and rebuild left-leaning. Stops descending at
		// the first non-and/or subtree -- assignment / ternary absorbed
		// into the rightmost leaf stays put.
		var leaves []ast.Expression
		var ops []string
		var collect func(e ast.Expression)
		collect = func(e ast.Expression) {
			if inf, ok := e.(*ast.InfixExpression); ok &&
				(inf.Operator == "and" || inf.Operator == "or") {
				collect(inf.Left)
				ops = append(ops, inf.Operator)
				collect(inf.Right)
				return
			}
			leaves = append(leaves, e)
		}
		collect(expression)
		if len(leaves) > 2 {
			rebuilt := leaves[0]
			for i := 1; i < len(leaves); i++ {
				rebuilt = &ast.InfixExpression{
					Token:    expression.Token,
					Operator: ops[i-1],
					Left:     rebuilt,
					Right:    leaves[i],
				}
			}
			if ri, ok := rebuilt.(*ast.InfixExpression); ok {
				*expression = *ri
			}
		}
	}
	return expression
}

func isAssignableTarget(e ast.Expression) bool {
	switch e.(type) {
	case *ast.Identifier, *ast.Global, *ast.ClassVariable, *ast.IndexExpression,
		*ast.InstanceVariable, *ast.ScopedIdentifier, *ast.ContextCallExpression:
		return true
	}
	return false
}

func (p *parser) parseIndexExpression(left ast.Expression) ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	exp := &ast.IndexExpression{Token: p.curToken, Left: left}

	p.nextToken()
	elements := p.parseExpressionList(token.RBRACKET)
	if !p.currentTokenIs(token.RBRACKET) {
		p.peekError(token.RBRACKET)
		return nil
	}
	exp.Arguments = elements
	return exp
}

func (p *parser) parseGroupedExpression() ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	lparen := p.curToken
	p.nextToken()
	hadLeadingSemi := false
	for p.currentTokenOneOf(token.NEWLINE, token.SEMICOLON) {
		if p.currentTokenIs(token.SEMICOLON) {
			hadLeadingSemi = true
		}
		p.nextToken()
	}
	if p.currentTokenIs(token.RPAREN) {
		// Empty `()` -- MRI parses as ParenthesesNode with body: nil.
		return &ast.ParenExpression{Token: lparen, Rparen: p.curToken}
	}
	exp := p.parseExpression(precLowest)
	var stmts []ast.Expression
	if exp != nil {
		stmts = append(stmts, exp)
	}
	hadTrailingSemi := false
	for p.currentTokenOneOf(token.SEMICOLON, token.NEWLINE) ||
		p.peekTokenOneOf(token.SEMICOLON, token.NEWLINE) {
		for p.currentTokenOneOf(token.SEMICOLON, token.NEWLINE) {
			if p.currentTokenIs(token.SEMICOLON) {
				hadTrailingSemi = true
			}
			p.nextToken()
		}
		for p.peekTokenOneOf(token.SEMICOLON, token.NEWLINE) {
			if p.peekTokenIs(token.SEMICOLON) {
				hadTrailingSemi = true
			}
			p.acceptOneOf(token.SEMICOLON, token.NEWLINE)
		}
		if p.currentTokenIs(token.RPAREN) {
			break
		}
		// peek=RPAREN with current still on a separator means nothing
		// remains to parse. peek=RPAREN with current on a real token
		// means a trailing statement -- parse it before bailing out.
		if p.currentTokenOneOf(token.SEMICOLON, token.NEWLINE) && p.peekTokenIs(token.RPAREN) {
			break
		}
		// Inside the body now -- any `;`/`\n` consumed here was between
		// statements, not trailing void. Reset tracking.
		hadTrailingSemi = false
		if !p.currentTokenOneOf(token.SEMICOLON, token.NEWLINE, token.RPAREN) {
			exp = p.parseExpression(precLowest)
		} else {
			p.nextToken()
			exp = p.parseExpression(precLowest)
		}
		if exp != nil {
			stmts = append(stmts, exp)
		}
	}
	if !p.accept(token.RPAREN) {
		return nil
	}
	if exp == nil {
		return nil
	}
	pe := &ast.ParenExpression{Token: lparen, Rparen: p.curToken, Expr: exp}
	if len(stmts) > 1 {
		pe.Stmts = stmts
	}
	// MRI tags ParenthesesNodeFlags=multiple_statements when source had a
	// leading/trailing void `;` or a real multi-statement body. Newlines
	// alone do not trigger the flag.
	if len(stmts) > 1 || hadLeadingSemi || hadTrailingSemi {
		pe.MultipleStmts = true
	}
	return pe
}

func (p *parser) parseIfExpression() ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	expression := &ast.ConditionalExpression{Token: p.curToken}
	p.nextToken()
	expression.Condition = p.parseExpression(precLowest)
	hasThen := p.peekTokenIs(token.THEN)
	if hasThen {
		p.accept(token.THEN)
	}

	if !hasThen && !p.peekTokenOneOf(token.NEWLINE, token.SEMICOLON) && !p.currentTokenOneOf(token.NEWLINE, token.SEMICOLON) {
		msg := fmt.Sprintf(
			"could not parse if expression: unexpected token %s: '%s'",
			p.peekToken.Type,
			p.peekToken.Literal,
		)
		err := errors.Wrap(
			&unexpectedTokenError{
				expectedTokens: []token.Type{token.NEWLINE, token.SEMICOLON},
				actualToken:    p.peekToken.Type,
			},
			msg,
		)
		p.errors = append(p.errors, err)
		return nil
	}
	if p.currentTokenOneOf(token.NEWLINE, token.SEMICOLON) {
		// Already on the newline (consumed by call-argument parsing in condition).
	} else if p.peekTokenOneOf(token.NEWLINE, token.SEMICOLON) {
		p.acceptOneOf(token.NEWLINE, token.SEMICOLON)
	}
	// Parse the consequence body. Terminators include ELSE, ELSIF, and END.
	expr := expression
	for {
		expr.Consequence = p.parseBlockStatement(token.ELSE, token.KW_ELSIF)
		if p.peekTokenIs(token.KW_ELSIF) {
			p.accept(token.KW_ELSIF)
			// elsif cond -> wrap nested conditional in a BlockStatement
			nested := &ast.ConditionalExpression{Token: p.curToken}
			p.nextToken()
			nested.Condition = p.parseExpression(precLowest)
			if p.peekTokenIs(token.THEN) {
				p.accept(token.THEN)
			}
			if p.peekTokenOneOf(token.NEWLINE, token.SEMICOLON) {
				p.acceptOneOf(token.NEWLINE, token.SEMICOLON)
			}
			expr.Alternative = &ast.BlockStatement{
				Statements: []ast.Statement{
					&ast.ExpressionStatement{Expression: nested},
				},
			}
			expr = nested
			continue
		}
		if p.peekTokenIs(token.ELSE) {
			p.accept(token.ELSE)
			if p.peekTokenOneOf(token.NEWLINE, token.SEMICOLON) {
				p.acceptOneOf(token.NEWLINE, token.SEMICOLON)
			}
			expr.Alternative = p.parseBlockStatement()
		}
		break
	}
	if !p.accept(token.END) {
		return nil
	}
	expression.EndToken = p.curToken
	return expression
}

func (p *parser) parseTenaryIfExpression(condition ast.Expression) ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	expression := &ast.ConditionalExpression{Token: p.curToken}
	p.nextToken()
	for p.currentTokenOneOf(token.NEWLINE, token.SEMICOLON) {
		p.nextToken()
	}
	expression.Condition = condition
	expression.Consequence = &ast.BlockStatement{
		Statements: []ast.Statement{
			&ast.ExpressionStatement{
				Expression: p.parseExpression(precLowest),
			},
		},
	}
	p.consume(token.COLON)
	for p.currentTokenOneOf(token.NEWLINE, token.SEMICOLON) {
		p.nextToken()
	}
	expression.Alternative = &ast.BlockStatement{
		Statements: []ast.Statement{
			&ast.ExpressionStatement{
				Expression: p.parseExpression(precLowest),
			},
		},
	}
	return expression
}

func (p *parser) parseModifierConditionalExpression(left ast.Expression) ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	expression := &ast.ConditionalExpression{Token: p.curToken}
	p.nextToken()
	for p.currentTokenIs(token.NEWLINE) {
		p.nextToken()
	}
	// Parse condition at precIfUnless so a following modifier (`stmt if X
	// while Y`) chains onto the outer expression -- letting the next
	// modifier wrap THIS conditional, not get absorbed into the condition.
	expression.Condition = p.parseExpression(precIfUnless)

	expression.Consequence = &ast.BlockStatement{
		Statements: []ast.Statement{
			&ast.ExpressionStatement{Expression: left},
		},
	}
	return expression
}

func (p *parser) parseModifierLoopExpression(left ast.Expression) ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	loop := &ast.LoopExpression{Token: p.curToken}
	p.nextToken()
	for p.currentTokenIs(token.NEWLINE) {
		p.nextToken()
	}
	// Same as parseModifierConditionalExpression: bound the condition at
	// precIfUnless so a chained outer modifier wraps this loop instead of
	// being absorbed.
	loop.Condition = p.parseExpression(precIfUnless)
	loop.Block = &ast.BlockStatement{
		Statements: []ast.Statement{
			&ast.ExpressionStatement{Expression: left},
		},
	}
	// `begin ... end while cond` is a do-while (post-test) loop in MRI,
	// distinct from a normal pre-test `while`. Flag so the printer can
	// emit the post-test form.
	if _, ok := left.(*ast.ExceptionHandlingBlock); ok {
		loop.PostTest = true
	}
	return loop
}

func (p *parser) parseLoopExpression() ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	loop := &ast.LoopExpression{Token: p.curToken}
	p.nextToken()
	prev := p.suppressDoBlock
	p.suppressDoBlock = true
	loop.Condition = p.parseExpression(precIfUnless)
	p.suppressDoBlock = prev
	if p.peekTokenIs(token.DO) {
		p.accept(token.DO)
	}
	loop.Block = p.parseBlockStatement(token.END)
	p.nextToken()
	return loop
}

func (p *parser) parseScopedConstName() *ast.Identifier {
	if !p.accept(token.CONST) {
		return nil
	}
	nameTok := p.curToken
	name := p.curToken.Literal
	for p.peekTokenIs(token.SCOPE) {
		p.nextToken() // consume ::
		if !p.accept(token.CONST) {
			return nil
		}
		name += "::" + p.curToken.Literal
	}
	return &ast.Identifier{Token: nameTok, Value: name}
}

func (p *parser) parseModule() ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	expr := &ast.ModuleExpression{Token: p.curToken}
	expr.Name = p.parseScopedConstName()
	if expr.Name == nil {
		return nil
	}

	if !p.acceptOneOf(token.NEWLINE, token.SEMICOLON) {
		return nil
	}

	expr.Body = p.parseBlockStatement(token.END, token.RESCUE)
	expr.Rescues = []*ast.RescueBlock{}
	for p.peekTokenIs(token.RESCUE) {
		p.accept(token.RESCUE)
		rescue := p.parseRescueBlock()
		expr.Rescues = append(expr.Rescues, rescue)
	}

	if !p.accept(token.END) {
		return nil
	}
	expr.EndToken = p.curToken
	return expr
}

func (p *parser) parseClass() ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	if p.peekTokenIs(token.LSHIFT) {
		return p.parseSingletonClass()
	}
	expr := &ast.ClassExpression{Token: p.curToken}
	expr.Name = p.parseScopedConstName()
	if expr.Name == nil {
		return nil
	}

	if p.peekTokenIs(token.LT) {
		p.consume(token.LT)
		expr.SuperClass = p.parseExpression(precLowest)
	}

	if !p.acceptOneOf(token.NEWLINE, token.SEMICOLON) {
		return nil
	}

	expr.Body = p.parseBlockStatement(token.END, token.RESCUE)
	expr.Rescues = []*ast.RescueBlock{}
	for p.peekTokenIs(token.RESCUE) {
		p.accept(token.RESCUE)
		rescue := p.parseRescueBlock()
		expr.Rescues = append(expr.Rescues, rescue)
	}

	if !p.accept(token.END) {
		return nil
	}
	expr.EndToken = p.curToken
	return expr
}

func (p *parser) parseSingletonClass() ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	expr := &ast.SingletonClassExpression{Token: p.curToken}

	if !p.consume(token.LSHIFT) {
		return nil
	}

	expr.Expr = p.parseExpression(precLowest)

	if !p.acceptOneOf(token.NEWLINE, token.SEMICOLON) {
		return nil
	}

	expr.Body = p.parseBlockStatement(token.END, token.RESCUE)
	expr.Rescues = []*ast.RescueBlock{}
	for p.peekTokenIs(token.RESCUE) {
		p.accept(token.RESCUE)
		rescue := p.parseRescueBlock()
		expr.Rescues = append(expr.Rescues, rescue)
	}

	if !p.accept(token.END) {
		return nil
	}
	expr.EndToken = p.curToken
	return expr
}

func (p *parser) parseFunctionLiteral() ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	lit := &ast.FunctionLiteral{Token: p.curToken}

	if !p.peekTokenOneOf(token.IDENT, token.SELF, token.CONST, token.GLOBAL, token.LBRACKET, token.AT, token.CLASS_VAR, token.LPAREN, token.XSTR_BEG) && !p.peekToken.Type.IsOperator() && !p.peekToken.Type.IsKeyword() {
		p.peekError(token.IDENT, token.CONST)
		return nil
	}

	// Singleton method on parenthesized expression: def (expr).method
	if p.peekTokenIs(token.LPAREN) {
		p.accept(token.LPAREN)
		p.nextToken()
		receiver := p.parseExpression(precLowest)
		if !p.accept(token.RPAREN) {
			return nil
		}
		if receiver == nil {
			return nil
		}
		lit.Receiver = &ast.Identifier{Token: p.curToken, Value: receiver.String()}
		if !p.accept(token.DOT) {
			return nil
		}
		p.nextToken()
		if p.curToken.Type.IsKeyword() || p.currentTokenOneOf(token.IDENT, token.CONST) {
			lit.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
		} else if p.currentTokenIs(token.LBRACKET) {
			lit.Name = &ast.Identifier{Token: p.curToken, Value: p.parseBracketMethodName()}
		} else if p.curToken.Type.IsOperator() {
			lit.Name = p.parseOperatorMethodName()
		}
		goto parseParams
	}

	// Singleton method on instance/class variable: def @obj.method
	if p.peekTokenOneOf(token.AT, token.CLASS_VAR) {
		p.nextToken()
		ivar := p.parseInstanceVariable()
		if ivar == nil {
			return nil
		}
		lit.Receiver = &ast.Identifier{Token: p.curToken, Value: ivar.String()}
		if !p.accept(token.DOT) {
			return nil
		}
		p.nextToken()
		if p.curToken.Type.IsKeyword() || p.currentTokenOneOf(token.IDENT, token.CONST) {
			lit.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
		} else if p.currentTokenIs(token.LBRACKET) {
			lit.Name = &ast.Identifier{Token: p.curToken, Value: p.parseBracketMethodName()}
		} else if p.curToken.Type.IsOperator() {
			lit.Name = p.parseOperatorMethodName()
		}
	} else if p.peekTokenOneOf(token.IDENT, token.SELF, token.CONST, token.GLOBAL, token.NIL, token.TRUE, token.FALSE) {
		p.acceptOneOf(token.IDENT, token.SELF, token.CONST, token.GLOBAL, token.NIL, token.TRUE, token.FALSE)
		if p.peekTokenIs(token.DOT) {
			lit.Receiver = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
			p.accept(token.DOT)
			if !p.peekTokenOneOf(token.IDENT, token.SELF, token.CONST, token.GLOBAL, token.LBRACKET) && !p.peekToken.Type.IsOperator() && !p.peekToken.Type.IsKeyword() {
				p.peekError(token.IDENT, token.CONST)
				return nil
			}
			p.nextToken()
			if p.currentTokenIs(token.LBRACKET) {
				lit.Name = &ast.Identifier{Token: p.curToken, Value: p.parseBracketMethodName()}
			} else if p.curToken.Type.IsOperator() {
				lit.Name = p.parseOperatorMethodName()
			} else {
				lit.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
			}
		} else {
			if p.currentTokenIs(token.LBRACKET) {
				lit.Name = &ast.Identifier{Token: p.curToken, Value: p.parseBracketMethodName()}
			} else if p.curToken.Type.IsOperator() {
				lit.Name = p.parseOperatorMethodName()
			} else {
				lit.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
			}
		}
	} else {
		p.nextToken()
		if p.currentTokenIs(token.LBRACKET) {
			lit.Name = &ast.Identifier{Token: p.curToken, Value: p.parseBracketMethodName()}
		} else if p.curToken.Type.IsOperator() {
			lit.Name = p.parseOperatorMethodName()
		} else {
			lit.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
		}
	}

parseParams:
	if lit.Name != nil && p.peekTokenIs(token.ASSIGN) &&
		p.curToken.Type == token.IDENT &&
		p.peekToken.Pos == p.curToken.Pos+len(p.curToken.Literal) {
		p.accept(token.ASSIGN)
		lit.Name.Value += "="
	}
	// Record whether the def has an explicit `()` param list -- MRI keeps
	// NODE_ARGS / ParametersNode on the tree even when empty, distinct
	// from a paren-less def.
	if p.peekTokenIs(token.LPAREN) {
		lit.ExplicitParens = true
	}
	lit.Parameters = p.parseParameters(token.LPAREN, token.RPAREN)

	if p.currentTokenOneOf(token.CAPTURE, token.AND) {
		bc := &ast.BlockCapture{Token: p.curToken}
		if p.peekTokenOneOf(token.NEWLINE, token.SEMICOLON, token.EOF, token.RPAREN) {
			if !p.version.AtLeast(ruby31) {
				p.versionError(ruby31, "anonymous block forwarding")
			}
			lit.CapturedBlock = bc
			if p.peekTokenIs(token.RPAREN) {
				p.accept(token.RPAREN)
			}
		} else {
			if !p.acceptOneOf(token.IDENT, token.NIL) {
				return nil
			}
			bc.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
			lit.CapturedBlock = bc
			if p.peekTokenIs(token.RPAREN) {
				p.acceptOneOf(token.RPAREN)
			}
		}
	}

	// Endless method: def name = expr (ruby 3.0+)
	if p.peekTokenIs(token.ASSIGN) {
		if !p.version.AtLeast(ruby30) {
			p.versionError(ruby30, "endless method definition")
		}
		p.consume(token.ASSIGN)
		expr := p.parseExpression(precLowest)
		if expr == nil {
			return nil
		}
		lit.Body = &ast.BlockStatement{
			Token: p.curToken,
			Statements: []ast.Statement{
				&ast.ExpressionStatement{Token: p.curToken, Expression: expr},
			},
		}
		if p.peekTokenIs(token.SEMICOLON) {
			p.accept(token.SEMICOLON)
			if p.peekTokenIs(token.END) {
				p.accept(token.END)
			}
		}
		lit.EndToken = p.curToken
		return lit
	}

	if p.peekTokenOneOf(token.NEWLINE, token.SEMICOLON) {
		p.nextToken()
	} else if !p.currentTokenOneOf(token.RPAREN, token.PIPE) {
		p.peekError(token.NEWLINE, token.SEMICOLON)
		return nil
	}
	lit.Body = p.parseBlockStatement(token.END, token.RESCUE, token.KW_ENSURE)
	lit.Rescues = []*ast.RescueBlock{}
	for p.peekTokenIs(token.RESCUE) {
		p.accept(token.RESCUE)
		rescue := p.parseRescueBlock()
		lit.Rescues = append(lit.Rescues, rescue)
	}
	if p.peekTokenIs(token.ELSE) {
		p.accept(token.ELSE)
		p.acceptOneOf(token.NEWLINE, token.SEMICOLON)
		lit.ElseBody = p.parseBlockStatement(token.END, token.KW_ENSURE)
	}
	if p.peekTokenIs(token.KW_ENSURE) {
		p.accept(token.KW_ENSURE)
		p.acceptOneOf(token.NEWLINE, token.SEMICOLON)
		lit.EnsureBody = p.parseBlockStatement(token.END)
	}
	if !p.accept(token.END) {
		return nil
	}
	lit.EndToken = p.curToken
	return lit
}

// parseBracketMethodName consumes the token stream for [] or []= method names.
func (p *parser) parseBracketMethodName() string {
	defer trace.TraceCtx(p.ctx)()
	name := p.curToken.Literal
	if !p.accept(token.RBRACKET) {
		p.peekError(token.RBRACKET)
		return ""
	}
	name += p.curToken.Literal
	if p.peekTokenIs(token.ASSIGN) {
		p.nextToken()
		name += p.curToken.Literal
	}
	return name
}

// parseOperatorMethodName handles unary +@ / -@ method names.
func (p *parser) parseOperatorMethodName() *ast.Identifier {
	defer trace.TraceCtx(p.ctx)()
	name := p.curToken.Literal
	if p.peekTokenIs(token.AT) {
		p.nextToken()
		name += p.curToken.Literal
	}
	return &ast.Identifier{Token: p.curToken, Value: name}
}

func (p *parser) parseParametersTail(identifiers []*ast.FunctionParameter, hasDelimiters bool, endToken token.Type) []*ast.FunctionParameter {
	defer trace.TraceCtx(p.ctx)()
	tailDefPrec := precAssignment
	if endToken == token.PIPE {
		tailDefPrec = precOr
	}
	for p.peekTokenIs(token.COMMA) {
		p.accept(token.COMMA)
		p.skipNewlines()
		if p.peekTokenIs(token.POWER) {
			if !p.version.AtLeast(ruby20) {
				p.versionError(ruby20, "keyword rest parameter")
			}
			p.accept(token.POWER)
			if p.peekTokenIs(token.NIL) {
				if !p.version.AtLeast(ruby27) {
					p.versionError(ruby27, "**nil parameter")
				}
				p.accept(token.NIL)
				identifiers = append(identifiers, &ast.FunctionParameter{
					Name: &ast.Identifier{Token: p.curToken, Value: "nil"}, IsKeywordRest: true, IsNoKeywords: true,
				})
			} else if p.peekTokenIs(token.IDENT) || p.peekTokenIs(token.CONST) {
				p.accept(token.IDENT)
				identifiers = append(identifiers, &ast.FunctionParameter{
					Name: &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}, IsKeywordRest: true,
				})
			} else {
				identifiers = append(identifiers, &ast.FunctionParameter{
					Name: &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}, IsKeywordRest: true,
				})
			}
			continue
		}
		if p.peekTokenOneOf(token.CAPTURE, token.AND) {
			// Block capture (&block). Leave cur at `&` for the caller
			// (parseFunctionLiteral / parseBlockExpression) to build the
			// CapturedBlock node; otherwise the `&` is lost in String().
			p.acceptOneOf(token.CAPTURE, token.AND)
			return identifiers
		}
		// Keyword parameter: `name: default` (ruby 2.0+)
		if p.peekTokenIs(token.LABEL) {
			if !p.version.AtLeast(ruby20) {
				p.versionError(ruby20, "keyword argument")
			}
			p.accept(token.LABEL)
			name := strings.TrimSuffix(p.curToken.Literal, ":")
			param := &ast.FunctionParameter{
				Name:      &ast.Identifier{Token: p.curToken, Value: name},
				IsKeyword: true,
			}
			if !p.peekTokenOneOf(token.COMMA, endToken, token.NEWLINE, token.SEMICOLON, token.PIPE, token.EOF) {
				p.nextToken()
				param.Default = p.parseExpression(tailDefPrec)
			} else if !p.version.AtLeast(ruby21) {
				p.versionError(ruby21, "required keyword argument")
			}
			identifiers = append(identifiers, param)
			continue
		}
		// Regular parameter
		if p.peekTokenOneOf(token.IDENT, token.CONST) {
			p.acceptOneOf(token.IDENT, token.CONST)
			name := p.curToken.Literal
			param := &ast.FunctionParameter{
				Name: &ast.Identifier{Token: p.curToken, Value: name},
			}
			if p.peekTokenIs(token.ASSIGN) {
				p.consume(token.ASSIGN)
				param.Default = p.parseExpression(tailDefPrec)
			}
			identifiers = append(identifiers, param)
			continue
		}
	}
	if hasDelimiters {
		p.skipNewlines()
		p.accept(endToken)
	}
	return identifiers
}

func (p *parser) parseOneParameter(endToken token.Type) []*ast.FunctionParameter {
	// Destructured param: (a, b) or ((a, b), c) or (a, *b)
	if p.peekTokenIs(token.LPAREN) {
		p.accept(token.LPAREN)
		inner := []*ast.FunctionParameter{}
		for !p.peekTokenIs(token.RPAREN) && !p.peekTokenIs(token.EOF) {
			if len(inner) > 0 {
				if !p.accept(token.COMMA) {
					break
				}
			}
			if p.peekTokenIs(token.ASTERISK) {
				p.accept(token.ASTERISK)
				name := "*"
				if p.peekTokenOneOf(token.IDENT, token.CONST) {
					p.acceptOneOf(token.IDENT, token.CONST)
					name += p.curToken.Literal
				}
				inner = append(inner, &ast.FunctionParameter{
					Name:    &ast.Identifier{Token: p.curToken, Value: name},
					IsSplat: true,
				})
				continue
			}
			one := p.parseOneParameter(token.RPAREN)
			if one == nil {
				break
			}
			inner = append(inner, one...)
		}
		p.accept(token.RPAREN)
		names := []string{}
		for _, ip := range inner {
			if ip.Name != nil {
				names = append(names, ip.Name.Value)
			}
		}
		destructName := "(" + strings.Join(names, ", ") + ")"
		return []*ast.FunctionParameter{{
			Name: &ast.Identifier{Token: p.curToken, Value: destructName},
		}}
	}
	if p.peekTokenIs(token.LABEL) {
		p.accept(token.LABEL)
	} else if !p.acceptOneOf(token.IDENT, token.CONST) {
		return nil
	}
	name := p.curToken.Literal
	isKeyword := strings.HasSuffix(name, ":")
	if isKeyword {
		if !p.version.AtLeast(ruby20) {
			p.versionError(ruby20, "keyword argument")
		}
		name = strings.TrimSuffix(name, ":")
	}
	ident := &ast.FunctionParameter{Name: &ast.Identifier{Token: p.curToken, Value: name}, IsKeyword: isKeyword}
	if isKeyword {
		if !p.peekTokenOneOf(token.COMMA, token.NEWLINE, token.SEMICOLON, token.PIPE, token.RPAREN, token.EOF) {
			kwDefPrec := precAssignment
			if endToken == token.PIPE {
				kwDefPrec = precOr
			}
			p.nextToken()
			ident.Default = p.parseExpression(kwDefPrec)
		} else if !p.version.AtLeast(ruby21) {
			p.versionError(ruby21, "required keyword argument")
		}
	} else if p.peekTokenIs(token.ASSIGN) {
		p.consume(token.ASSIGN)
		defPrec := precAssignment
		if endToken == token.PIPE {
			defPrec = precOr
		}
		ident.Default = p.parseExpression(defPrec)
	}
	return []*ast.FunctionParameter{ident}
}

func (p *parser) parseParameters(startToken, endToken token.Type) []*ast.FunctionParameter {
	defer trace.TraceCtx(p.ctx)()
	hasDelimiters := false
	if p.peekTokenIs(startToken) {
		hasDelimiters = true
		p.accept(startToken)
		p.skipNewlines()
	}

	identifiers := []*ast.FunctionParameter{}

	if hasDelimiters && p.peekTokenIs(token.SEMICOLON) {
		p.accept(token.SEMICOLON)
		return identifiers
	}

	if !hasDelimiters && p.peekTokenIs(endToken) {
		p.peekError(token.NEWLINE, token.SEMICOLON)
		return nil
	}

	if hasDelimiters && p.peekTokenIs(endToken) {
		p.accept(endToken)
		return identifiers
	}

	// Block-local separator: |; x| -- no regular params
	if hasDelimiters && p.peekTokenIs(token.SEMICOLON) {
		p.accept(token.SEMICOLON)
		return identifiers
	}

	if !hasDelimiters && p.peekTokenOneOf(token.NEWLINE, token.SEMICOLON, token.ASSIGN) {
		return identifiers
	}

	// Forwarding: def foo(...) -- ruby 2.7+
	if p.peekTokenIs(token.RANGEEX) {
		if !p.version.AtLeast(ruby27) {
			p.versionError(ruby27, "argument forwarding (...)")
		}
		p.accept(token.RANGEEX)
		identifiers = append(identifiers, &ast.FunctionParameter{IsForwarding: true})
		if hasDelimiters {
			p.accept(endToken)
		}
		return identifiers
	}

	if p.peekTokenIs(token.POWER) {
		if !p.version.AtLeast(ruby20) {
			p.versionError(ruby20, "keyword rest parameter")
		}
		p.accept(token.POWER)
		if p.peekTokenIs(token.NIL) {
			if !p.version.AtLeast(ruby27) {
				p.versionError(ruby27, "**nil parameter")
			}
			p.accept(token.NIL)
			identifiers = append(identifiers, &ast.FunctionParameter{
				Name:          &ast.Identifier{Token: p.curToken, Value: "nil"},
				IsKeywordRest: true,
				IsNoKeywords:  true,
			})
		} else {
			if p.peekTokenOneOf(token.IDENT, token.CONST) {
				p.acceptOneOf(token.IDENT, token.CONST)
			}
			identifiers = append(identifiers, &ast.FunctionParameter{
				Name:          &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal},
				IsKeywordRest: true,
			})
		}
		return p.parseParametersTail(identifiers, hasDelimiters, endToken)
	}

	if p.peekTokenIs(token.ASTERISK) {
		p.accept(token.ASTERISK)
		// Anonymous rest: bare * as first param
		if p.peekTokenOneOf(token.COMMA, token.RPAREN, token.PIPE, token.NEWLINE, token.SEMICOLON, token.EOF) {
			identifiers = append(identifiers, &ast.FunctionParameter{IsSplat: true})
			return p.parseParametersTail(identifiers, hasDelimiters, endToken)
		}
		// Named rest: *x
		if p.peekTokenOneOf(token.IDENT, token.CONST) {
			p.acceptOneOf(token.IDENT, token.CONST)
		}
		identifiers = append(identifiers, &ast.FunctionParameter{
			Name:    &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal},
			IsSplat: true,
		})
		return p.parseParametersTail(identifiers, hasDelimiters, endToken)
	}
	if p.peekTokenOneOf(token.CAPTURE, token.AND) {
		p.acceptOneOf(token.CAPTURE, token.AND)
		return identifiers
	}
	identifiers = append(identifiers, p.parseOneParameter(endToken)...)

	for p.peekTokenIs(token.COMMA) {
		p.accept(token.COMMA)
		p.skipNewlines()
		// Trailing comma: |a,| or (a,) -- MRI's ImplicitRestNode marker.
		if hasDelimiters && p.peekTokenIs(endToken) {
			identifiers = append(identifiers, &ast.FunctionParameter{IsImplicitRest: true})
			p.accept(endToken)
			return identifiers
		}
		// Forwarding: def foo(a, ...) -- ruby 2.7+
		if p.peekTokenIs(token.RANGEEX) {
			if !p.version.AtLeast(ruby27) {
				p.versionError(ruby27, "argument forwarding (...)")
			}
			p.accept(token.RANGEEX)
			identifiers = append(identifiers, &ast.FunctionParameter{IsForwarding: true})
			if hasDelimiters {
				p.accept(endToken)
			}
			return identifiers
		}
		if p.peekTokenIs(token.POWER) {
			if !p.version.AtLeast(ruby20) {
				p.versionError(ruby20, "keyword rest parameter")
			}
			p.accept(token.POWER)
			if p.peekTokenIs(token.NIL) {
				if !p.version.AtLeast(ruby27) {
					p.versionError(ruby27, "**nil parameter")
				}
				p.accept(token.NIL)
				identifiers = append(identifiers, &ast.FunctionParameter{
					Name:          &ast.Identifier{Token: p.curToken, Value: "nil"},
					IsKeywordRest: true,
					IsNoKeywords:  true,
				})
			} else {
				if p.peekTokenIs(token.IDENT) || p.peekTokenIs(token.CONST) {
					p.accept(token.IDENT)
				}
				identifiers = append(identifiers, &ast.FunctionParameter{
					Name:          &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal},
					IsKeywordRest: true,
				})
			}
			if !p.peekTokenIs(token.COMMA) {
				if hasDelimiters {
					p.accept(endToken)
				}
				return identifiers
			}
			continue
		}
		if p.peekTokenIs(token.ASTERISK) {
			p.accept(token.ASTERISK)
			if p.peekTokenOneOf(token.COMMA, token.RPAREN, token.PIPE, token.NEWLINE, token.SEMICOLON, token.EOF) {
				identifiers = append(identifiers, &ast.FunctionParameter{IsSplat: true})
				continue
			}
			// Named rest: *x
			if p.peekTokenOneOf(token.IDENT, token.CONST) {
				p.acceptOneOf(token.IDENT, token.CONST)
			}
			identifiers = append(identifiers, &ast.FunctionParameter{
				Name:    &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal},
				IsSplat: true,
			})
			continue
		}
		if p.peekTokenOneOf(token.CAPTURE, token.AND) {
			p.acceptOneOf(token.CAPTURE, token.AND)
			// Leave curToken at `&` so parseFunctionLiteral /
			// parseBlock can build the CapturedBlock node. Anonymous
			// (ruby 3.1+) and named both fall through to the caller.
			return identifiers
		}
		// Destructured or regular param
		if p.peekTokenIs(token.LPAREN) {
			identifiers = append(identifiers, p.parseOneParameter(endToken)...)
			continue
		}
		isKw := false
		if p.peekTokenIs(token.LABEL) {
			if !p.version.AtLeast(ruby20) {
				p.versionError(ruby20, "keyword argument")
			}
			p.accept(token.LABEL)
			isKw = true
		} else {
			p.accept(token.IDENT)
		}
		pName := p.curToken.Literal
		if isKw {
			pName = strings.TrimSuffix(pName, ":")
		}
		pIdent := &ast.FunctionParameter{Name: &ast.Identifier{Token: p.curToken, Value: pName}, IsKeyword: isKw}
		defPrecLoop := precAssignment
		if endToken == token.PIPE {
			defPrecLoop = precOr
		}
		if isKw {
			if !p.peekTokenOneOf(token.COMMA, token.NEWLINE, token.SEMICOLON, token.PIPE, token.RPAREN, token.EOF) {
				p.nextToken()
				pIdent.Default = p.parseExpression(defPrecLoop)
			} else if !p.version.AtLeast(ruby21) {
				p.versionError(ruby21, "required keyword argument")
			}
		} else if p.peekTokenIs(token.ASSIGN) {
			p.consume(token.ASSIGN)
			pIdent.Default = p.parseExpression(defPrecLoop)
		}
		identifiers = append(identifiers, pIdent)
	}

	if hasDelimiters {
		p.skipNewlines()
	}

	if !hasDelimiters && p.peekTokenIs(endToken) {
		p.peekError(endToken)
		return nil
	}

	if hasDelimiters && p.peekTokenIs(token.SEMICOLON) {
		return identifiers
	}

	if hasDelimiters && p.peekTokenIs(endToken) {
		p.accept(endToken)
	}

	return identifiers
}

func (p *parser) parseBlockStatement(t ...token.Type) *ast.BlockStatement {
	defer trace.TraceCtx(p.ctx)()
	block := &ast.BlockStatement{Token: p.curToken}

	for p.peekToken.Type != token.END && !p.peekTokenOneOf(t...) {
		if p.peekTokenIs(token.EOF) {
			p.peekError(token.EOF)
			return block
		}
		// Modifier if/unless/while/until after return/break/next: `return unless cond`
		// Skip the modifier condition to avoid parsing it as standalone block.
		if p.currentTokenOneOf(token.IF, token.UNLESS, token.WHILE, token.UNTIL) && len(block.Statements) > 0 {
			lastStmt := block.Statements[len(block.Statements)-1]
			isJump := false
			if _, ok := lastStmt.(*ast.ReturnStatement); ok {
				isJump = true
			} else if es, ok := lastStmt.(*ast.ExpressionStatement); ok {
				if _, ok := es.Expression.(*ast.JumpExpression); ok {
					isJump = true
				}
			}
			if isJump {
				p.nextToken()
				p.parseExpression(precLowest)
				continue
			}
		}
		// If curToken starts a compound expression, parse it first
		// before advancing. This handles nested case/when where the inner
		// when would otherwise terminate the outer when-body.
		if p.currentTokenOneOf(token.CASE, token.IF, token.UNLESS, token.WHILE, token.UNTIL, token.BEGIN, token.CLASS, token.MODULE, token.DEF, token.STRING_BEG) {
			saved := p.curToken
			stmt := p.parseStatement()
			if stmt != nil {
				block.Statements = append(block.Statements, stmt)
			}
			if p.curToken == saved {
				p.nextToken()
			}
			continue
		}
		p.nextToken()
		stmt := p.parseStatement()
		if stmt != nil {
			block.Statements = append(block.Statements, stmt)
		}
	}

	return block
}

func (p *parser) parseMethodCall(context ast.Expression) ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	contextCallExpression := &ast.ContextCallExpression{Token: p.curToken, Context: context}

	p.nextToken()
	for p.currentTokenIs(token.NEWLINE) {
		p.nextToken()
	}

	// .[] method call
	if p.currentTokenIs(token.LBRACKET) {
		p.nextToken()
		args := p.parseExpressionList(token.RBRACKET)
		methodName := "[]"
		if p.peekTokenIs(token.ASSIGN) {
			p.accept(token.ASSIGN)
			methodName = "[]="
		}
		contextCallExpression.Function = &ast.Identifier{Token: p.curToken, Value: methodName}
		contextCallExpression.Arguments = args
		if p.peekTokenIs(token.LPAREN) {
			p.accept(token.LPAREN)
			p.nextToken()
			contextCallExpression.Arguments = append(contextCallExpression.Arguments, p.parseExpressionList(token.RPAREN)...)
		}
		if p.peekTokenOneOf(token.LBRACE, token.DO) {
			p.acceptOneOf(token.LBRACE, token.DO)
			contextCallExpression.Block = p.parseBlockExpr()
		}
		return contextCallExpression
	}

	// .() call syntax (implicit .call)
	if p.currentTokenIs(token.LPAREN) {
		contextCallExpression.Function = &ast.Identifier{Token: p.curToken, Value: "call"}
		p.nextToken()
		contextCallExpression.Arguments = p.parseExpressionList(token.RPAREN)
		return contextCallExpression
	}

	if !p.currentTokenOneOf(token.IDENT, token.CONST) && !p.curToken.Type.IsKeyword() && !p.curToken.Type.IsOperator() {
		p.expectError(token.IDENT, token.CONST, token.CLASS)
		return nil
	}

	function := &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
	contextCallExpression.Function = function

	if p.peekTokenOneOf(token.SEMICOLON, token.NEWLINE, token.EOF, token.DOT, token.SCOPE, token.LONELY, token.END, token.HASHROCKET) {
		contextCallExpression.Arguments = []ast.Expression{}
		return contextCallExpression
	}

	if p.suppressDoBlock && p.peekTokenIs(token.DO) {
		contextCallExpression.Arguments = []ast.Expression{}
		return contextCallExpression
	}

	if p.peekTokenIs(token.LPAREN) && !p.peekToken.HadWhitespace {
		p.accept(token.LPAREN)
		p.nextToken()
		contextCallExpression.Arguments = p.parseExpressionList(token.RPAREN)
		contextCallExpression.ExplicitParens = true
		if p.peekTokenOneOf(token.LBRACE, token.DO) {
			if p.suppressDoBlock && p.peekTokenIs(token.DO) {
				return contextCallExpression
			}
			p.acceptOneOf(token.LBRACE, token.DO)
			contextCallExpression.Block = p.parseBlockExpr()
		}
		return contextCallExpression
	}

	if p.peekTokenOneOf(append(tokensNotPossibleInCallArgs, token.RBRACE, token.RPAREN, token.EMBEXPR_END, token.SEMICOLON, token.EOF, token.LONELY)...) {
		return contextCallExpression
	}

	// Binary +/- at end of line: not a call argument.
	if p.peekTokenOneOf(token.PLUS, token.MINUS) && p.peek2TokenIs(token.NEWLINE) {
		contextCallExpression.Arguments = []ast.Expression{}
		return contextCallExpression
	}

	// `[` without preceding whitespace is index access, not an array arg
	// without parens. `a.b[x]` -> index; `a.b [x]` -> arg list.
	if p.peekTokenIs(token.LBRACKET) && !p.peekToken.HadWhitespace {
		contextCallExpression.Arguments = []ast.Expression{}
		return contextCallExpression
	}

	// Spaced binary operator after bare method call on the same line:
	// a.b + c is infix, a.b +\n c is a call argument.
	if p.peekToken.HadWhitespace && p.peekToken.Type.IsOperator() &&
		!p.peek2TokenIs(token.NEWLINE) && !p.peek2TokenIs(token.EOF) &&
		p.spacedOperator(p.peekToken, p.peek2Token) {
		contextCallExpression.Arguments = []ast.Expression{}
		return contextCallExpression
	}

	// `a.b-x` / `a.b+x` (no whitespace before - / +) is subtraction /
	// addition on the call's result, not a call with -x / +x as the
	// arg. MRI's rule: a unary - / + requires whitespace before the
	// operator; with no leading whitespace it's infix.
	if (p.peekTokenIs(token.MINUS) || p.peekTokenIs(token.PLUS)) &&
		!p.peekToken.HadWhitespace {
		contextCallExpression.Arguments = []ast.Expression{}
		return contextCallExpression
	}

	// Same rule for `a.b*x` / `a.b**x`: with no leading whitespace the
	// * / ** is infix (multiplication / power), not a (kw)splat arg.
	if (p.peekTokenIs(token.ASTERISK) || p.peekTokenIs(token.POWER)) &&
		!p.peekToken.HadWhitespace {
		contextCallExpression.Arguments = []ast.Expression{}
		return contextCallExpression
	}

	p.nextToken()

	blockStops := []token.Type{token.LBRACE, token.DO}
	if p.suppressDoBlock {
		blockStops = []token.Type{token.LBRACE}
	}
	// `do` binds to the leftmost call -- the outer (this) call -- not to
	// inner paren-less calls inside its arg list. Suppress `do`-block on
	// nested calls so the outer parseBlockExpr below captures the block.
	// `{...}` still binds tight (high precedence) and stays with the inner.
	// `and`/`or` are below paren-less call args -- `foo.m x and y` is
	// `foo.m(x) and y`, not `foo.m(x and y)`. Suppress them too.
	prevSDB := p.suppressDoBlock
	prevAO := p.suppressKwAndOr
	p.suppressDoBlock = true
	p.suppressKwAndOr = true
	contextCallExpression.Arguments = p.parseCallArguments(
		blockStops...,
	)
	p.suppressDoBlock = prevSDB
	p.suppressKwAndOr = prevAO
	if p.currentTokenOneOf(blockStops...) {
		contextCallExpression.Block = p.parseBlockExpr()
	}
	return contextCallExpression
}

func (p *parser) parseContextCallExpression(context ast.Expression) ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	contextCallExpression := &ast.ContextCallExpression{Token: p.curToken, Context: context}
	if _, ok := context.(*ast.Self); ok && !p.currentTokenOneOf(token.DOT, token.SCOPE) {
		p.expectError(token.DOT, token.SCOPE)
		return nil
	}
	if p.currentTokenOneOf(token.DOT, token.SCOPE) {
		p.nextToken()
		for p.currentTokenIs(token.NEWLINE) {
			p.nextToken()
		}
	}

	if !p.currentTokenOneOf(token.IDENT, token.CONST, token.CLASS) {
		p.expectError(token.IDENT, token.CONST, token.CLASS)
		return nil
	}

	function := p.parseIdentifier()
	ident := function.(*ast.Identifier)
	contextCallExpression.Function = ident

	if p.peekTokenOneOf(token.SEMICOLON, token.NEWLINE, token.EOF, token.DOT, token.SCOPE, token.END, token.HASHROCKET) {
		contextCallExpression.Arguments = []ast.Expression{}
		return contextCallExpression
	}

	if p.peekTokenIs(token.LPAREN) {
		p.accept(token.LPAREN)
		p.nextToken()
		contextCallExpression.Arguments = p.parseExpressionList(token.RPAREN)
		if p.peekTokenOneOf(token.LBRACE, token.DO) {
			p.acceptOneOf(token.LBRACE, token.DO)
			contextCallExpression.Block = p.parseBlockExpr()
		}
		return contextCallExpression
	}

	if p.peekTokenOneOf(append(tokensNotPossibleInCallArgs, token.RBRACE, token.RPAREN, token.EMBEXPR_END, token.SEMICOLON, token.EOF)...) {
		return contextCallExpression
	}

	// `obj.method**x` / `obj.method*x` (no space before * / **) is the infix
	// power / multiplication operator, not a paren-less call with a (kw)splat
	// arg. With no whitespace separating method-name from operator, treat it
	// as infix and let the outer parseExpression loop pick it up.
	if !p.peekToken.HadWhitespace && p.peekTokenOneOf(token.POWER, token.ASTERISK) {
		contextCallExpression.Arguments = []ast.Expression{}
		return contextCallExpression
	}

	if p.peekToken.HadWhitespace && p.peekToken.Type.IsOperator() &&
		!p.peek2TokenIs(token.NEWLINE) && !p.peek2TokenIs(token.EOF) &&
		p.spacedOperator(p.peekToken, p.peek2Token) {
		contextCallExpression.Arguments = []ast.Expression{}
		return contextCallExpression
	}

	p.nextToken()
	contextCallExpression.Arguments = p.parseCallArguments(
		token.LBRACE, token.DO,
	)
	if p.currentTokenOneOf(token.LBRACE, token.DO) {
		contextCallExpression.Block = p.parseBlockExpr()
	}
	return contextCallExpression
}

func (p *parser) parseCallArgument(function ast.Expression) ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	switch fn := function.(type) {
	case *ast.Identifier:
		// plain function call: foo arg1, arg2
	case *ast.ScopedIdentifier:
		// Scoped call like Foo::bar arg1, arg2 -- treat as context call.
		innerIdent, ok := fn.Inner.(*ast.Identifier)
		if !ok {
			return p.parseContextCallExpression(function)
		}
		exp := &ast.ContextCallExpression{
			Token:    innerIdent.Token,
			Context:  fn.Outer,
			Function: innerIdent,
		}
		prevAO := p.suppressKwAndOr
		prevSDB := p.suppressDoBlock
		p.suppressKwAndOr = true
		p.suppressDoBlock = true
		exp.Arguments = p.parseExpressionList(token.SEMICOLON, token.NEWLINE, token.SCOPE)
		p.suppressKwAndOr = prevAO
		p.suppressDoBlock = prevSDB
		if p.peekTokenOneOf(token.LBRACE, token.DO) {
			p.acceptOneOf(token.LBRACE, token.DO)
			exp.Block = p.parseBlockExpr()
		}
		return exp
	default:
		return p.parseContextCallExpression(function)
	}
	ident := function.(*ast.Identifier)
	exp := &ast.ContextCallExpression{Token: ident.Token, Function: ident}
	if p.currentTokenOneOf(token.LBRACE, token.DO) {
		exp.Block = p.parseBlockExpr()
		return exp
	}

	prevAO := p.suppressKwAndOr
	prevSDB := p.suppressDoBlock
	p.suppressKwAndOr = true
	// `do` binds to the outermost call; suppress on nested calls in the
	// arg list so the outer call below captures it. `{...}` still attaches
	// inner (high precedence) -- matches MRI.
	p.suppressDoBlock = true
	exp.Arguments = p.parseExpressionList(token.SEMICOLON, token.NEWLINE, token.SCOPE)
	p.suppressKwAndOr = prevAO
	p.suppressDoBlock = prevSDB
	// Block attach only if contiguous: `foo a, b { ... }` attaches, but
	// `foo a, b\n{...}` is two statements (the brace starts a fresh hash /
	// expression), matching MRI.
	if p.peekTokenOneOf(token.LBRACE, token.DO) && !p.currentTokenOneOf(token.NEWLINE, token.SEMICOLON) {
		p.acceptOneOf(token.LBRACE, token.DO)
		exp.Block = p.parseBlockExpr()
	}
	return exp
}

func (p *parser) concatStringPart(str *ast.StringLiteral, rstr *ast.StringLiteral) {
	// Keep adjacent literals as separate StringLiteral nodes. MRI parses
	// `"a" "b"` as an InterpolatedString with two parts; merging into one
	// flat string loses that shape on re-parse.
	str.Adjacent = append(str.Adjacent, rstr)
}

func (p *parser) parseStringConcat(left ast.Expression) ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	str, ok := left.(*ast.StringLiteral)
	if !ok {
		return p.parseCallArgument(left)
	}
	// Parse the adjacent string at precCall so a trailing infix DOT (or
	// other higher-prec operator) is left to the outer Pratt loop --
	// `"a" "b".c` is `("a" "b").c`, not `"a" + ("b".c)`.
	right := p.parseExpression(precCall)
	rstr, ok := right.(*ast.StringLiteral)
	if !ok {
		return left
	}
	p.concatStringPart(str, rstr)
	for p.peekTokenOneOf(token.STRING, token.STRING_BEG) {
		p.nextToken()
		next := p.parseExpression(precCall)
		if ns, ok := next.(*ast.StringLiteral); ok {
			p.concatStringPart(str, ns)
		}
	}
	return str
}

func (p *parser) parseCallBlock(function ast.Expression) ast.Expression {
	defer trace.TraceCtx(p.ctx)()

	exp := &ast.ContextCallExpression{Token: p.curToken}
	exp.Block = p.parseBlockExpr()
	switch fn := function.(type) {
	case *ast.Identifier:
		exp.Function = fn
		return exp
	case *ast.YieldExpression:
		fn.Block = exp.Block
		return fn
	case *ast.SuperExpression:
		fn.Block = exp.Block
		return fn
	case *ast.ContextCallExpression:
		fn.Block = exp.Block
		return fn
	case *ast.Assignment:
		if rhs, ok := fn.Right.(*ast.Identifier); ok {
			call := &ast.ContextCallExpression{Token: rhs.Token, Function: rhs}
			call.Block = exp.Block
			fn.Right = call
			return fn
		}
		if rhs, ok := fn.Right.(*ast.ContextCallExpression); ok {
			rhs.Block = exp.Block
			return fn
		}
	case ast.ExpressionList:
		if len(fn) > 0 {
			last := fn[len(fn)-1]
			if ident, ok := last.(*ast.Identifier); ok {
				call := &ast.ContextCallExpression{Token: ident.Token, Function: ident}
				call.Block = exp.Block
				fn[len(fn)-1] = call
				return fn
			}
			if cc, ok := last.(*ast.ContextCallExpression); ok {
				cc.Block = exp.Block
				return fn
			}
		}
	case *ast.ArrayLiteral:
		// Array construction before block, e.g. `[items].each do...end`
		// -- block can't attach to array, fall through to error
	case *ast.InfixExpression:
		ident, ok := fn.Right.(*ast.Identifier)
		if !ok {
			if rcc, ok := fn.Right.(*ast.ContextCallExpression); ok {
				rcc.Block = exp.Block
				return fn
			}
			break
		}
		exp.Function = ident
		fn.Right = exp
		return fn
	case *ast.SplatExpression:
		if inner, ok := fn.Right.(*ast.Identifier); ok {
			call := &ast.ContextCallExpression{Token: inner.Token, Function: inner}
			call.Block = exp.Block
			fn.Right = call
			return fn
		}
		if inner, ok := fn.Right.(*ast.ContextCallExpression); ok {
			inner.Block = exp.Block
			return fn
		}
	}
	p.errors = append(p.errors, &parseError{
		Pos:  p.file.Position(p.pos),
		Kind: SyntaxError,
		Msg:  fmt.Sprintf("could not parse call expression: expected identifier, got token '%T'", function),
	})
	return nil
}

func (p *parser) parseCallExpressionWithParens(function ast.Expression) ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	exp := &ast.ContextCallExpression{Token: p.curToken, ExplicitParens: true}
	switch fn := function.(type) {
	case *ast.Identifier:
		exp.Function = fn
	case *ast.ScopedIdentifier:
		// `Foo::bar(args)` is a method call on Foo, not a Proc/method
		// retrieval. Split into Context=Foo, Function=bar.
		if innerIdent, ok := fn.Inner.(*ast.Identifier); ok {
			exp.Context = fn.Outer
			exp.Function = innerIdent
		} else {
			exp.Context = function
			exp.Function = &ast.Identifier{Token: p.curToken, Value: "call"}
		}
	default:
		// Non-identifier callable: @ivar(args), method_returning_proc(args)
		exp.Context = function
		exp.Function = &ast.Identifier{Token: p.curToken, Value: "call"}
	}
	p.nextToken()
	exp.Arguments = p.parseExpressionList(token.RPAREN)
	if p.peekTokenOneOf(token.LBRACE, token.DO) {
		// `do` binds to the outermost call -- if we're inside an outer
		// paren-less call's arg list, leave the `do` for the outer to grab.
		// `{...}` still binds tight here.
		if p.suppressDoBlock && p.peekTokenIs(token.DO) {
			return exp
		}
		p.acceptOneOf(token.LBRACE, token.DO)
		exp.Block = p.parseBlockExpr()
	}
	return exp
}

func (p *parser) parseCallArguments(end ...token.Type) []ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	prevSuppressHR := p.suppressHashrocket
	p.suppressHashrocket = true
	defer func() { p.suppressHashrocket = prevSuppressHR }()
	list := []ast.Expression{}
	if p.currentTokenOneOf(end...) {
		return list
	}

	first := p.parseExpression(precComma)
	if p.peekTokenIs(token.HASHROCKET) {
		hash := p.parseImplicitHash(first, end...)
		list = append(list, hash)
		return list
	}
	list = append(list, first)

	for p.peekTokenIs(token.COMMA) {
		p.consume(token.COMMA)
		for p.currentTokenOneOf(token.NEWLINE, token.SEMICOLON) {
			p.nextToken()
		}
		arg := p.parseExpression(precComma)
		if p.peekTokenIs(token.HASHROCKET) {
			hash := p.parseImplicitHash(arg, end...)
			list = append(list, hash)
			return list
		}
		list = append(list, arg)
	}

	if p.peekTokenOneOf(end...) {
		p.acceptOneOf(end...)
	}

	return list
}

func (p *parser) parseImplicitHash(firstKey ast.Expression, end ...token.Type) ast.Expression {
	hash := &ast.HashLiteral{Token: p.curToken, Map: ast.NewOrderedExprMap(), Implicit: true}
	p.accept(token.HASHROCKET)
	p.nextToken()
	for p.currentTokenOneOf(token.NEWLINE, token.SEMICOLON) {
		p.nextToken()
	}
	val := p.parseExpression(precAssignment)
	hash.Map.Set(firstKey, val)
	for {
		for p.peekTokenOneOf(token.NEWLINE, token.SEMICOLON) {
			if p.peekTokenOneOf(end...) {
				break
			}
			p.acceptOneOf(token.NEWLINE, token.SEMICOLON)
		}
		if p.peekTokenOneOf(end...) {
			p.acceptOneOf(end...)
			return hash
		}
		if !p.peekTokenIs(token.COMMA) {
			break
		}
		p.consume(token.COMMA)
		for p.currentTokenOneOf(token.NEWLINE, token.SEMICOLON) {
			p.nextToken()
		}
		if p.currentTokenOneOf(end...) {
			return hash
		}
		if p.currentTokenOneOf(token.CAPTURE, token.AND) {
			break
		}
		key := p.parseExpression(precAssignment)
		if p.peekTokenIs(token.HASHROCKET) {
			p.accept(token.HASHROCKET)
			p.nextToken()
			for p.currentTokenOneOf(token.NEWLINE, token.SEMICOLON) {
				p.nextToken()
			}
			v := p.parseExpression(precAssignment)
			hash.Map.Set(key, v)
		} else {
			hash.Map.Set(key, nil)
		}
	}
	if p.peekTokenOneOf(end...) {
		p.acceptOneOf(end...)
	}
	return hash
}

func (p *parser) parseExpressionList(end ...token.Type) []ast.Expression {
	defer trace.TraceCtx(p.ctx)()
	prevSuppressHR := p.suppressHashrocket
	p.suppressHashrocket = true
	defer func() { p.suppressHashrocket = prevSuppressHR }()
	list := []ast.Expression{}
	// Skip leading newlines/semicolons inside parens/brackets.
	for p.currentTokenOneOf(token.NEWLINE, token.SEMICOLON) {
		p.nextToken()
	}
	if p.currentTokenOneOf(end...) {
		return list
	}

	next := p.parseExpression(precComma)
	// `do/end` normally binds to the outermost call, not the inner arg.
	// Exception: `proc`/`lambda` bare identifiers as arg are conventionally
	// a proc literal -- attach the block to them.
	if id, ok := next.(*ast.Identifier); ok && p.peekTokenIs(token.DO) &&
		(id.Value == "proc" || id.Value == "lambda") {
		p.nextToken()
		next = p.parseCallBlock(next)
	}
	if call, ok := next.(*ast.ContextCallExpression); ok && call.Block == nil && p.peekTokenIs(token.LBRACE) {
		p.accept(token.LBRACE)
		call.Block = p.parseBlockExpr()
	}
	if p.peekTokenIs(token.HASHROCKET) {
		hash := p.parseImplicitHash(next, end...)
		list = append(list, hash)
		if p.currentTokenOneOf(token.CAPTURE, token.AND) {
			list = append(list, p.parseExpression(precComma))
			if p.peekTokenOneOf(end...) {
				p.acceptOneOf(end...)
			}
			return list
		}
		// parseImplicitHash already consumed the end token. Don't accept
		// it again -- doing so would steal a matching token belonging to
		// an outer construct (e.g. \`[b[3=>4]]\` -- the outer \`]\` is the
		// array literal's, not the inner index's).
		if !p.currentTokenOneOf(end...) && p.peekTokenOneOf(end...) {
			p.acceptOneOf(end...)
		}
		return list
	} else if _, isStr := next.(*ast.StringLiteral); isStr && p.peekTokenOneOf(token.COLON, token.SYMBEG) {
		hash := p.parseStringLabelHash(next, end...)
		list = append(list, hash)
		if p.currentTokenOneOf(token.CAPTURE, token.AND) {
			list = append(list, p.parseExpression(precComma))
			if p.peekTokenOneOf(end...) {
				p.acceptOneOf(end...)
			}
			return list
		}
		if !p.currentTokenOneOf(end...) && p.peekTokenOneOf(end...) {
			p.acceptOneOf(end...)
		}
		return list
	} else {
		list = append(list, next)
	}

	// Handle comma-separated elements with newlines between them.
	for {
		// Skip newlines (but not end-marker tokens) before checking for comma or end.
		for p.peekTokenOneOf(token.NEWLINE, token.SEMICOLON) {
			if p.peekTokenOneOf(end...) {
				break
			}
			p.acceptOneOf(token.NEWLINE, token.SEMICOLON)
		}
		if p.peekTokenOneOf(end...) {
			p.acceptOneOf(end...)
			return list
		}
		if !p.peekTokenIs(token.COMMA) {
			break
		}
		p.consume(token.COMMA)
		// Comma-as-continuation: skip newlines/semicolons after a comma
		// so multi-line arg lists keep parsing -- NEWLINE / SEMICOLON
		// may be end markers normally, but a trailing comma defeats them.
		for p.currentTokenOneOf(token.NEWLINE, token.SEMICOLON) {
			p.nextToken()
		}
		// after comma, SCOPE starts a top-level scoped ident (::Foo) as next arg;
		// not a list terminator.
		endNoScope := make([]token.Type, 0, len(end))
		for _, e := range end {
			if e != token.SCOPE {
				endNoScope = append(endNoScope, e)
			}
		}
		if p.currentTokenOneOf(endNoScope...) {
			return list
		}
		next = p.parseExpression(precComma)
		// `proc`/`lambda` arg do/end -- attach block to proc literal.
		if id, ok := next.(*ast.Identifier); ok && p.peekTokenIs(token.DO) &&
			(id.Value == "proc" || id.Value == "lambda") {
			p.nextToken()
			next = p.parseCallBlock(next)
		}
		if call, ok := next.(*ast.ContextCallExpression); ok && call.Block == nil && p.peekTokenIs(token.LBRACE) {
			p.accept(token.LBRACE)
			call.Block = p.parseBlockExpr()
		}
		if p.peekTokenIs(token.HASHROCKET) {
			hash := p.parseImplicitHash(next, end...)
			list = append(list, hash)
			if p.currentTokenOneOf(token.CAPTURE, token.AND) {
				list = append(list, p.parseExpression(precComma))
				if p.peekTokenOneOf(end...) {
					p.acceptOneOf(end...)
				}
				return list
			}
			if p.peekTokenOneOf(end...) {
				p.acceptOneOf(end...)
			}
			return list
		}
		if _, isStr := next.(*ast.StringLiteral); isStr && p.peekTokenOneOf(token.COLON, token.SYMBEG) {
			hash := p.parseStringLabelHash(next, end...)
			list = append(list, hash)
			if p.currentTokenOneOf(token.CAPTURE, token.AND) {
				list = append(list, p.parseExpression(precComma))
				if p.peekTokenOneOf(end...) {
					p.acceptOneOf(end...)
				}
				return list
			}
			if p.peekTokenOneOf(end...) {
				p.acceptOneOf(end...)
			}
			return list
		}
		list = append(list, next)
	}

	if p.peekTokenOneOf(end...) {
		p.acceptOneOf(end...)
	}

	return list
}

func (p *parser) parseStringLabelHash(firstKey ast.Expression, end ...token.Type) ast.Expression {
	hash := &ast.HashLiteral{Token: p.curToken, Map: ast.NewOrderedExprMap(), Implicit: true}
	p.acceptOneOf(token.COLON, token.SYMBEG)
	key := &ast.SymbolLiteral{Token: p.curToken, Value: firstKey.(*ast.StringLiteral)}
	if p.peekTokenOneOf(token.COMMA, token.RPAREN, token.RBRACE, token.NEWLINE) {
		hash.Map.Set(key, firstKey)
	} else {
		p.nextToken()
		hash.Map.Set(key, p.parseExpression(precAssignment))
	}
	for p.peekTokenIs(token.COMMA) {
		p.consume(token.COMMA)
		for p.currentTokenOneOf(token.NEWLINE, token.SEMICOLON) {
			p.nextToken()
		}
		if p.currentTokenOneOf(end...) {
			break
		}
		k, v, ok, omitted := p.parseKeyValue()
		if !ok {
			break
		}
		hash.Map.Set(k, v)
		if omitted {
			hash.Map.SetOmitted(k)
		}
	}
	if p.peekTokenOneOf(end...) {
		p.acceptOneOf(end...)
	}
	return hash
}

// buildWordArray produces an ArrayLiteral from the parts of a %w/%W (word list)
// or %i/%I (symbol list) percent literal. isSymbol controls whether elements are
// wrapped in SymbolLiteral (for %i/%I) or left as StringLiteral (for %w/%W).
func (p *parser) buildWordArray(beg token.Token, parts []ast.Expression, isSymbol bool) ast.Expression {
	var elements []ast.Expression
	var curWord []ast.Expression // parts comprising the current word (for %W/%I interpolation)

	flushWord := func() {
		if len(curWord) == 0 {
			return
		}
		var elem ast.Expression
		switch {
		case len(curWord) == 1:
			// Single-part word. Pick the cheapest representation that
			// prints with proper escaping:
			//   - StringLiteral / Symbol -> use as-is
			//   - StringContent (pure literal text) -> wrap in Value form
			//     so the printer's quote-aware escape handles apostrophes
			//     (`'s` -> `'\'s'`); Parts-form doesn't escape `'`
			//   - anything else (bare EMBEXPR) -> wrap in Parts form so it
			//     prints as `"#{x}"` (preserves to_s semantics)
			switch w := curWord[0].(type) {
			case *ast.StringLiteral:
				elem = w
			case *ast.SymbolLiteral:
				elem = w
			case *ast.StringContent:
				elem = &ast.StringLiteral{Token: beg, Value: w.Value}
			default:
				elem = &ast.StringLiteral{Token: beg, Parts: curWord}
			}
		default:
			elem = &ast.StringLiteral{Token: beg, Parts: curWord}
		}
		// %I element with interpolation: the SymbolLiteral wrapping above
		// only ran for single-content words. Multi-part word collected
		// SymbolLiteral + EMBEXPR as a flat list; wrap the whole sequence
		// in one SymbolLiteral with an interpolated StringLiteral value
		// so it prints as `:"hello_#{name}"`.
		if isSymbol {
			if sl, ok := elem.(*ast.StringLiteral); ok && len(sl.Parts) > 1 {
				// Strip the leading SymbolLiteral wrap (added per-content
				// piece above) and use its inner value as the string content.
				flat := make([]ast.Expression, 0, len(sl.Parts))
				for _, p := range sl.Parts {
					if sym, ok := p.(*ast.SymbolLiteral); ok {
						if scv, ok2 := sym.Value.(*ast.StringLiteral); ok2 {
							flat = append(flat, &ast.StringContent{Token: sym.Token, Value: scv.Value})
						} else if id, ok2 := sym.Value.(*ast.Identifier); ok2 {
							flat = append(flat, &ast.StringContent{Token: sym.Token, Value: id.Value})
						} else {
							flat = append(flat, p)
						}
					} else {
						flat = append(flat, p)
					}
				}
				elem = &ast.SymbolLiteral{
					Token: beg,
					Value: &ast.StringLiteral{Token: beg, Parts: flat},
				}
			}
		}
		elements = append(elements, elem)
		curWord = nil
	}

	for _, part := range parts {
		switch pt := part.(type) {
		case *ast.StringContent:
			hasLeadingWS := len(pt.Value) > 0 && isSpace(rune(pt.Value[0]))
			hasTrailingWS := len(pt.Value) > 0 && isSpace(rune(pt.Value[len(pt.Value)-1]))

			words := splitWordList(pt.Value)
			for j, w := range words {
				if j == 0 {
					// Only flush on a real word boundary -- leading WS in
					// this content. Following an EMBEXPR with no leading
					// WS means the StringContent continues the same word.
					if hasLeadingWS {
						flushWord()
					}
				} else {
					flushWord()
				}
				if isSymbol {
					var symValue ast.Expression = &ast.StringLiteral{Token: beg, Value: w}
					if isSimpleIdent(w) {
						symValue = &ast.Identifier{Value: w}
					}
					curWord = append(curWord, &ast.SymbolLiteral{
						Token: pt.Token,
						Value: symValue,
					})
				} else {
					// Use StringContent (not StringLiteral) so when this
					// word is wrapped in an outer StringLiteral with Parts
					// the literal text emits inline, not as `#{"..."}`.
					curWord = append(curWord, &ast.StringContent{Token: pt.Token, Value: w})
				}
				if j == len(words)-1 && hasTrailingWS {
					flushWord()
				}
			}
			if len(words) == 0 {
				// All-whitespace content -- forces a word boundary.
				flushWord()
			}
		default:
			// EMBEXPR expression -- part of current word if no whitespace separates them.
			curWord = append(curWord, pt)
		}
	}
	flushWord()

	multiline := false
	for _, part := range parts {
		if sc, ok := part.(*ast.StringContent); ok && strings.Contains(sc.Value, "\n") {
			multiline = true
			break
		}
	}
	return &ast.ArrayLiteral{
		Token:     beg,
		Rbracket:  p.curToken, // STRING_END
		Elements:  elements,
		Multiline: multiline,
	}
}

// splitWordList splits s by unicode whitespace, respecting backslash escapes.
// isSimpleIdent reports whether s is a valid bare symbol name (no quotes needed).
func isSimpleIdent(s string) bool {
	if len(s) == 0 {
		return false
	}
	// Allow trailing ? ! = (e.g. :foo?, :bar!, :baz=).
	body := s
	if last := s[len(s)-1]; last == '?' || last == '!' || last == '=' {
		body = s[:len(s)-1]
	}
	if len(body) == 0 {
		return false // bare ?, !, = are not valid bare symbols
	}
	first := rune(body[0])
	if !(first == '_' || (first >= 'a' && first <= 'z') || (first >= 'A' && first <= 'Z')) {
		return false
	}
	for _, r := range body[1:] {
		if r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return true
}

func splitWordList(s string) []string {
	var words []string
	var cur strings.Builder
	escaping := false
	inUnicodeBrace := false
	for _, r := range s {
		if escaping {
			if !isSpace(r) {
				cur.WriteByte('\\')
			}
			cur.WriteRune(r)
			if r == 'u' {
				// Check for \u{...} multi-codepoint escape; handled by
				// looking for '{' as the NEXT character after 'u'. We set
				// a flag here and check for '{' on the next iteration.
			}
			escaping = false
			continue
		}
		if inUnicodeBrace {
			cur.WriteRune(r)
			if r == '}' {
				inUnicodeBrace = false
			}
			continue
		}
		if r == '\\' {
			escaping = true
			continue
		}
		if r == '{' && cur.Len() >= 2 {
			b := cur.String()
			if b[len(b)-1] == 'u' && b[len(b)-2] == '\\' {
				inUnicodeBrace = true
				cur.WriteRune(r)
				continue
			}
		}
		if isSpace(r) {
			if cur.Len() > 0 {
				words = append(words, cur.String())
				cur.Reset()
			}
			continue
		}
		cur.WriteRune(r)
	}
	if escaping {
		cur.WriteByte('\\')
	}
	if cur.Len() > 0 {
		words = append(words, cur.String())
	}
	return words
}

func isSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r'
}

// buildSymbolFromPercent produces a SymbolLiteral from a %s(...) percent literal.
func (p *parser) buildSymbolFromPercent(beg token.Token, parts []ast.Expression) ast.Expression {
	if len(parts) == 0 {
		return &ast.SymbolLiteral{Token: beg, Value: &ast.StringLiteral{Value: ""}}
	}
	if len(parts) == 1 {
		if sc, ok := parts[0].(*ast.StringContent); ok {
			var symValue ast.Expression = &ast.StringLiteral{Token: beg, Value: sc.Value}
			if isSimpleIdent(sc.Value) {
				symValue = &ast.Identifier{Value: sc.Value}
			}
			return &ast.SymbolLiteral{
				Token: beg,
				Value: symValue,
			}
		}
	}
	p.errors = append(p.errors, &parseError{
		Pos:  p.file.Position(p.pos),
		Kind: SyntaxError,
		Msg:  "invalid %s literal",
	})
	return nil
}

func (p *parser) peekPrecedence() int {
	return precedenceForToken(p.peekToken.Type)
}

func (p *parser) curPrecedence() int {
	return precedenceForToken(p.curToken.Type)
}

func precedenceForToken(t token.Type) int {
	if prec, ok := precedences[t]; ok {
		return prec
	}
	return precLowest
}

func (p *parser) currentTokenOneOf(types ...token.Type) bool {
	for _, typ := range types {
		if p.curToken.Type == typ {
			return true
		}
	}
	return false
}

func (p *parser) currentTokenIs(t token.Type) bool {
	return p.curToken.Type == t
}

func (p *parser) peekTokenOneOf(types ...token.Type) bool {
	for _, typ := range types {
		if p.peekToken.Type == typ {
			return true
		}
	}
	return false
}

func (p *parser) peekTokenIs(t token.Type) bool {
	return p.peekToken.Type == t
}

func (p *parser) peek2TokenIs(t token.Type) bool {
	return p.peek2Token.Type == t
}

// accept moves to the next Token
// if it's from the valid set.
func (p *parser) accept(t token.Type) bool {
	if p.peekTokenIs(t) {
		p.nextToken()
		return true
	}

	p.peekError(t)
	return false
}

// acceptOneOf moves to the next Token
// if it's from the valid set.
func (p *parser) acceptOneOf(t ...token.Type) bool {
	if p.peekTokenOneOf(t...) {
		p.nextToken()
		return true
	}

	p.peekError(t...)
	return false
}

// consume consumes the next token
// if it's from the valid set.
func (p *parser) consume(t token.Type) bool {
	isRightToken := p.accept(t)
	if isRightToken {
		p.nextToken()
	}
	return isRightToken
}

func (p *parser) skipNewlines() {
	for p.peekTokenIs(token.NEWLINE) {
		p.nextToken()
	}
}
