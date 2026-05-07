package parser

import (
	"fmt"
	gotoken "go/token"
	"math/big"
	"strconv"
	"strings"

	"github.com/lczyk/goruby/ast"
	"github.com/lczyk/goruby/lexer"
	"github.com/lczyk/goruby/token"
	"github.com/pkg/errors"
)

// Possible precendece values
const (
	_ int = iota
	precLowest
	precBlockDo     // do
	precBlockBraces // { |x| }
	precIfUnless    // modifier-if, modifier-unless
	precAssignment  // x = 5
	precTenary      // ?, :
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
	token.HASHROCKET:         precAssignment,
	token.COMMA:             precAssignment,
	token.THEN:              precHighest,
	token.NEWLINE:           precHighest,
	token.PIPE:              precOr,
	token.AND:               precAnd,
	token.LOGICALOR:         precLogicalOr,
	token.LOGICALAND:        precLogicalAnd,
	token.LABEL:             precCallArg,
	token.CLASS_VAR:         precCallArg,
	token.CAPTURE:           precCapture,
	token.POWER:             precProduct,
	token.RANGE:             precLessGreater,
	token.RANGEEX:           precLessGreater,
	token.LONELY:            precCall,
	token.KW_AND:            precLogicalAnd,
	token.KW_OR:             precLogicalOr,
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
	token.LT,
	token.LTE,
	token.GT,
	token.GTE,
	token.SPACESHIP,
	token.LSHIFT,
	token.RSHIFT,
	token.CASEEQ,
	token.LSHIFTASSIGN,
	token.RSHIFTASSIGN,
	token.ANDASSIGN_BITWISE,
	token.ORASSIGN_BITWISE,
	token.XORASSIGN,
	token.EQ,
	token.NOTEQ,
	token.MATCH,
	token.NMATCH,
	token.IF,
	token.UNLESS,
	token.COLON,
	token.RBRACKET,
	token.COMMA,
}

type (
	prefixParseFn func() ast.Expression
	infixParseFn  func(ast.Expression) ast.Expression
)

var defaultExpressionTerminators = []token.Type{
	token.SEMICOLON,
	token.NEWLINE,
}

// A parser parses the token emitted by the provided lexer.Lexer and returns an
// AST describing the parsed program.
type parser struct {
	file   *gotoken.File
	l      *lexer.Lexer
	errors []error

	// Tracing/debugging
	mode   Mode // parsing mode
	trace  bool // == (mode & Trace != 0)
	indent int  // indentation used for tracing output

	pos       gotoken.Pos
	lastLine  string
	curToken  token.Token
	peekToken token.Token

	prefixParseFns map[token.Type]prefixParseFn
	infixParseFns  map[token.Type]infixParseFn
}

func (p *parser) init(fset *gotoken.FileSet, filename string, src []byte, mode Mode) {
	p.file = fset.AddFile(filename, -1, len(src))

	p.l = lexer.New(string(src))
	p.errors = []error{}

	p.mode = mode
	p.trace = mode&Trace != 0 // for convenience (p.trace is used frequently)

	p.prefixParseFns = make(map[token.Type]prefixParseFn)
	p.registerPrefix(token.IDENT, p.parseIdentifier)
	p.registerPrefix(token.CONST, p.parseIdentifier)
	p.registerPrefix(token.AT, p.parseInstanceVariable)
	p.registerPrefix(token.INT, p.parseIntegerLiteral)
	p.registerPrefix(token.FLOAT, p.parseFloatLiteral)
	p.registerPrefix(token.STRING, p.parseStringLiteral)
	p.registerPrefix(token.STRING_BEG, p.parseInterpolatedString)
	p.registerPrefix(token.XSTR_BEG, p.parseInterpolatedString)
	p.registerPrefix(token.REGEX_BEG, p.parseInterpolatedRegex)
	p.registerPrefix(token.REGEX, p.parseStringLiteral)
	p.registerPrefix(token.XSTR, p.parseStringLiteral)
	p.registerPrefix(token.BANG, p.parsePrefixExpression)
	p.registerPrefix(token.PLUS, p.parsePrefixExpression)
	p.registerPrefix(token.MINUS, p.parsePrefixExpression)
	p.registerPrefix(token.ASTERISK, p.parseSplatExpression)
	p.registerPrefix(token.TILDE, p.parsePrefixExpression)
	p.registerPrefix(token.LOGICALAND, p.parsePrefixExpression)
	p.registerPrefix(token.LOGICALOR, p.parsePrefixExpression)
	p.registerPrefix(token.TRUE, p.parseBoolean)
	p.registerPrefix(token.FALSE, p.parseBoolean)
	p.registerPrefix(token.LPAREN, p.parseGroupedExpression)
	p.registerPrefix(token.IF, p.parseIfExpression)
	p.registerPrefix(token.UNLESS, p.parseIfExpression)
	p.registerPrefix(token.WHILE, p.parseLoopExpression)
	p.registerPrefix(token.UNTIL, p.parseLoopExpression)
	p.registerPrefix(token.KW_FOR, p.parseLoopExpression)
	p.registerPrefix(token.CASE, p.parseCaseExpression)
	p.registerPrefix(token.WHEN, p.parseErrorSkip) // when outside case is an error
	p.registerPrefix(token.ELSE, p.parseErrorSkip)  // else outside if/case is an error
	p.registerPrefix(token.KW_ELSIF, p.parseErrorSkip)
	p.registerPrefix(token.KW_IN, p.parseErrorSkip)
	p.registerPrefix(token.BREAK, p.parseJumpExpression)
	p.registerPrefix(token.NEXT, p.parseJumpExpression)
	p.registerPrefix(token.KW_REDO, p.parseJumpExpression)
	p.registerPrefix(token.KW_RETRY, p.parseJumpExpression)
	p.registerPrefix(token.KW_ENSURE, p.parseErrorSkip)
	p.registerPrefix(token.DEF, p.parseFunctionLiteral)
	p.registerPrefix(token.SCOPE, p.parseTopLevelScope)
	p.registerPrefix(token.LABEL, p.parseLabelExpression)
	p.registerPrefix(token.SYMBEG, p.parseSymbolLiteral)
	p.registerPrefix(token.LBRACKET, p.parseArrayLiteral)
	p.registerPrefix(token.NIL, p.parseNilLiteral)
	p.registerPrefix(token.SELF, p.parseSelf)
	p.registerPrefix(token.MODULE, p.parseModule)
	p.registerPrefix(token.CLASS, p.parseClass)
	p.registerPrefix(token.LBRACE, p.parseHash)
	p.registerPrefix(token.DO, p.parseBlock)
	p.registerPrefix(token.YIELD, p.parseYield)
	p.registerPrefix(token.GLOBAL, p.parseGlobal)
	p.registerPrefix(token.KEYWORD__FILE__, p.parseKeyword__FILE__)
	p.registerPrefix(token.KEYWORD__LINE__, p.parseKeyword__LINE__)
	p.registerPrefix(token.KEYWORD__ENCODING__, p.parseEncodingKeyword)
	p.registerPrefix(token.RPAREN, p.parseErrorSkip)
	p.registerPrefix(token.RBRACKET, p.parseErrorSkip)
	p.registerPrefix(token.RBRACE, p.parseErrorSkip)
	p.registerPrefix(token.NEWLINE, p.parseErrorSkip)
	p.registerPrefix(token.EMBEXPR_END, p.parseErrorSkip)
	p.registerPrefix(token.HASHROCKET, p.parseErrorSkip)
	p.registerPrefix(token.RESCUE, p.parseExceptionHandlingBlock)
	p.registerPrefix(token.BEGIN, p.parseExceptionHandlingBlock)
	p.registerPrefix(token.CLASS_VAR, p.parseClassVariable)
	p.registerPrefix(token.AND, p.parseBlockCapture) // &:to_s, &block
	p.registerPrefix(token.CAPTURE, p.parseBlockCapture)
	p.registerPrefix(token.KW_SUPER, p.parseSuper)
	p.registerPrefix(token.KW_UNDEF, p.parseUndef)
	p.registerPrefix(token.KW_NOT, p.parsePrefixExpression)
	p.registerPrefix(token.KW_DEFINED, p.parseDefinedExpression)
	p.registerPrefix(token.KW_ALIAS, p.parseAlias)
	p.registerPrefix(token.LAMBDA, p.parseLambda)
	p.registerPrefix(token.RANGE, p.parseBeginlessRange)
	p.registerPrefix(token.RANGEEX, p.parseBeginlessRange)

	p.infixParseFns = make(map[token.Type]infixParseFn)
	p.registerInfix(token.PLUS, p.parseInfixExpression)
	p.registerInfix(token.MINUS, p.parseInfixExpression)
	p.registerInfix(token.SLASH, p.parseInfixExpression)
	p.registerInfix(token.ASTERISK, p.parseInfixExpression)
	p.registerInfix(token.MODULO, p.parseInfixExpression)
	p.registerInfix(token.AND, p.parseInfixExpression)
	p.registerInfix(token.PIPE, p.parseInfixExpression)
	p.registerInfix(token.EQ, p.parseInfixExpression)
	p.registerInfix(token.NOTEQ, p.parseInfixExpression)
	p.registerInfix(token.LT, p.parseInfixExpression)
	p.registerInfix(token.GT, p.parseInfixExpression)
	p.registerInfix(token.LTE, p.parseInfixExpression)
	p.registerInfix(token.GTE, p.parseInfixExpression)
	p.registerInfix(token.LOGICALOR, p.parseInfixExpression)
	p.registerInfix(token.LOGICALAND, p.parseInfixExpression)
	p.registerInfix(token.MATCH, p.parseInfixExpression)
	p.registerInfix(token.NMATCH, p.parseInfixExpression)
	p.registerInfix(token.SPACESHIP, p.parseInfixExpression)
	p.registerInfix(token.LSHIFT, p.parseInfixExpression)
	p.registerInfix(token.RSHIFT, p.parseInfixExpression)
	p.registerInfix(token.CASEEQ, p.parseInfixExpression)
	p.registerInfix(token.HASHROCKET, p.parseInfixExpression)
	p.registerInfix(token.ASSIGN, p.parseAssignment)
	p.registerInfix(token.ADDASSIGN, p.parseAssignmentOperator)
	p.registerInfix(token.SUBASSIGN, p.parseAssignmentOperator)
	p.registerInfix(token.MULASSIGN, p.parseAssignmentOperator)
	p.registerInfix(token.DIVASSIGN, p.parseAssignmentOperator)
	p.registerInfix(token.MODASSIGN, p.parseAssignmentOperator)
	p.registerInfix(token.LSHIFTASSIGN, p.parseAssignmentOperator)
	p.registerInfix(token.RSHIFTASSIGN, p.parseAssignmentOperator)
	p.registerInfix(token.ANDASSIGN_BITWISE, p.parseAssignmentOperator)
	p.registerInfix(token.ORASSIGN_BITWISE, p.parseAssignmentOperator)
	p.registerInfix(token.XORASSIGN, p.parseAssignmentOperator)
	p.registerInfix(token.IF, p.parseModifierConditionalExpression)
	p.registerInfix(token.UNLESS, p.parseModifierConditionalExpression)
	p.registerInfix(token.QMARK, p.parseTenaryIfExpression)
	p.registerInfix(token.LPAREN, p.parseCallExpressionWithParens)
	p.registerInfix(token.IDENT, p.parseCallArgument)
	p.registerInfix(token.CONST, p.parseCallArgument)
	p.registerInfix(token.GLOBAL, p.parseCallArgument)
	p.registerInfix(token.INT, p.parseCallArgument)
	p.registerInfix(token.FLOAT, p.parseCallArgument)
	p.registerInfix(token.STRING, p.parseCallArgument)
	p.registerInfix(token.STRING_BEG, p.parseCallArgument)
	p.registerInfix(token.XSTR_BEG, p.parseCallArgument)
	p.registerInfix(token.REGEX_BEG, p.parseCallArgument)
	p.registerInfix(token.REGEX, p.parseCallArgument)
	p.registerInfix(token.XSTR, p.parseCallArgument)
	p.registerInfix(token.LABEL, p.parseCallArgument)
	p.registerInfix(token.SYMBEG, p.parseCallArgument)
	p.registerInfix(token.CLASS_VAR, p.parseCallArgument)
	p.registerInfix(token.CAPTURE, p.parseCallArgument)
	p.registerInfix(token.SELF, p.parseCallArgument)
	p.registerInfix(token.LAMBDA, p.parseCallArgument)
	p.registerInfix(token.LBRACE, p.parseCallBlock)
	p.registerInfix(token.DO, p.parseCallBlock)
	p.registerInfix(token.DOT, p.parseMethodCall)
	p.registerInfix(token.COMMA, p.parseExpressions)
	p.registerInfix(token.LBRACKET, p.parseIndexExpression)
	p.registerInfix(token.POWER, p.parseInfixExpression)
	p.registerInfix(token.RANGE, p.parseInfixExpression)
	p.registerInfix(token.RANGEEX, p.parseInfixExpression)
	p.registerInfix(token.RESCUE, p.parseRescueModifier)
	p.registerInfix(token.LONELY, p.parseMethodCall)
	p.registerInfix(token.KW_AND, p.parseInfixExpression)
	p.registerInfix(token.KW_OR, p.parseInfixExpression)
	p.registerInfix(token.POWERASSIGN, p.parseAssignmentOperator)
	p.registerInfix(token.ORASSIGN, p.parseAssignmentOperator)
	p.registerInfix(token.ANDASSIGN, p.parseAssignmentOperator)
	p.registerInfix(token.SCOPE, p.parseScopedIdentifierExpression)

	// Read two tokens, so curToken and peekToken are both set
	p.nextToken()
	p.nextToken()
}

func (p *parser) printTrace(a ...interface{}) {
	const dots = ". . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . "
	const n = len(dots)
	pos := p.file.Position(p.pos)
	fmt.Printf("%5d:%3d: ", pos.Line, pos.Column)
	i := 2 * p.indent
	for i > n {
		fmt.Print(dots)
		i -= n
	}
	// i <= n
	fmt.Print(dots[0:i])
	fmt.Println(a...)
}

func trace(p *parser, msg string) *parser {
	p.printTrace(msg, "(")
	p.indent++
	return p
}

// Usage pattern: defer un(trace(p, "..."))
func un(p *parser) {
	p.indent--
	p.printTrace(")")
}

func (p *parser) registerPrefix(tokenType token.Type, fn prefixParseFn) {
	p.prefixParseFns[tokenType] = fn
}

func (p *parser) registerInfix(tokenType token.Type, fn infixParseFn) {
	p.infixParseFns[tokenType] = fn
}

func (p *parser) nextToken() {
	// Because of one-token look-ahead, print the previous token
	// when tracing as it provides a more readable output. The
	// very first token (!p.pos.IsValid()) is not initialized
	// (it is token.ILLEGAL), so don't print it .
	if p.trace && p.pos.IsValid() {
		s := p.curToken.Type.String()
		switch {
		case p.curToken.IsLiteral():
			p.printTrace(s, p.curToken.Literal)
		case p.curToken.IsOperator(), p.curToken.IsKeyword():
			p.printTrace("\"" + s + "\"")
		default:
			p.printTrace(s)
		}
	}
	p.curToken = p.peekToken
	p.pos = gotoken.Pos(p.curToken.Pos)
	p.lastLine += p.curToken.Literal
	if p.curToken.Type == token.NEWLINE {
		p.file.AddLine(int(p.pos))
		p.lastLine = ""
	}
	if p.l.HasNext() {
		p.peekToken = p.l.NextToken()
		// Skip comment tokens when not in ParseComments mode.
		if p.mode&ParseComments == 0 {
			for p.peekToken.Type == token.HASH {
				if p.l.HasNext() {
					p.l.NextToken() // consume STRING content
				}
				if p.l.HasNext() {
					p.peekToken = p.l.NextToken()
				} else {
					p.peekToken = token.NewToken(token.EOF, "", -1)
					break
				}
			}
		}
	} else {
		p.peekToken = token.NewToken(token.EOF, "", -1)
	}
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
	}
	p.errors = append(p.errors, errors.WithStack(err))
}

func (p *parser) expectError(t ...token.Type) {
	epos := p.file.Position(p.pos)
	err := &unexpectedTokenError{
		Pos:            epos,
		expectedTokens: t,
		actualToken:    p.curToken.Type,
	}
	p.errors = append(p.errors, errors.WithStack(err))
}

func (p *parser) noPrefixParseFnError(t token.Type) {
	msg := fmt.Sprintf("no prefix parse function for type %s found", t)
	epos := p.file.Position(p.pos)
	if epos.Filename != "" || epos.IsValid() {
		msg = epos.String() + ": " + msg
	}
	p.errors = append(p.errors, errors.New(msg))
}

// ParseProgram returns the parsed program AST and all errors which occured
// during the parse process. If the error is not nil the AST may be incomplete
// and callers should always check if they can handle the error with providing
// more input by checking with e.g. IsEOFError.
func (p *parser) ParseProgram() (*ast.Program, error) {
	program := &ast.Program{File: p.file}
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
	if len(p.errors) != 0 {
		return program, NewErrors("Parsing errors", p.errors...)
	}
	return program, nil
}

func (p *parser) parseStatement() ast.Statement {
	if p.trace {
		defer un(trace(p, "parseStatement"))
	}
	switch p.curToken.Type {
	case token.ILLEGAL:
		msg := p.curToken.Literal
		epos := p.file.Position(p.pos)
		if epos.Filename != "" || epos.IsValid() {
			msg = epos.String() + ": " + msg
		}
		p.errors = append(p.errors, errors.New(msg))
		return nil
	case token.EOF:
		p.expectError(token.NEWLINE)
		return nil
	case token.NEWLINE:
		return nil
	case token.RETURN:
		return p.parseReturnStatement()
	case token.HASH:
		return p.parseComment()
	default:
		return p.parseExpressionStatement()
	}
}

func (p *parser) parseReturnStatement() *ast.ReturnStatement {
	if p.trace {
		defer un(trace(p, "parseReturnStatement"))
	}
	stmt := &ast.ReturnStatement{Token: p.curToken}
	p.nextToken()

	if p.currentTokenOneOf(token.NEWLINE, token.SEMICOLON) {
		p.nextToken()
		return stmt
	}

	valToken := p.curToken
	stmt.ReturnValue = p.parseExpression(precLowest)
	if list, ok := stmt.ReturnValue.(ast.ExpressionList); ok {
		stmt.ReturnValue = &ast.ArrayLiteral{Elements: list}
	}

	if p.peekTokenOneOf(token.NEWLINE, token.SEMICOLON) {
		p.nextToken()
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

func (p *parser) parseExpressionStatement() *ast.ExpressionStatement {
	if p.trace {
		defer un(trace(p, "parseExpressionStatement"))
	}
	stmt := &ast.ExpressionStatement{Token: p.curToken}
	stmt.Expression = p.parseExpression(precLowest)
	if p.peekTokenOneOf(token.SEMICOLON, token.NEWLINE) {
		p.nextToken()
	}
	return stmt
}

func (p *parser) parseExpression(precedence int) ast.Expression {
	if p.trace {
		defer un(trace(p, "parseExpression"))
	}
	prefix := p.prefixParseFns[p.curToken.Type]
	if prefix == nil {
		p.noPrefixParseFnError(p.curToken.Type)
		return nil
	}
	leftExp := prefix()
	for precedence < p.peekPrecedence() {
		if leftExp == nil {
			return nil // fail early and stop parsing
		}
		if p.currentTokenOneOf(token.NEWLINE, token.SEMICOLON) {
			return leftExp
		}
		infix := p.infixParseFns[p.peekToken.Type]
		if infix == nil {
			return leftExp
		}
		p.nextToken()
		leftExp = infix(leftExp)
	}
	return leftExp
}

func (p *parser) parseComment() ast.Statement {
	if p.trace {
		defer un(trace(p, "parseComment"))
	}
	comment := &ast.Comment{Token: p.curToken}
	if !p.accept(token.STRING) {
		return nil
	}
	comment.Value = p.curToken.Literal
	if !p.peekTokenOneOf(token.NEWLINE, token.EOF) {
		epos := p.file.Position(p.pos)
		msg := fmt.Errorf("%s: Expected newline or eof after comment", epos.String())
		p.errors = append(p.errors, msg)
		return nil
	}

	if p.mode&ParseComments == 0 {
		return nil
	}
	return comment
}

func (p *parser) parseExceptionHandlingBlock() ast.Expression {
	if p.trace {
		defer un(trace(p, "parseExceptionHandlingBlock"))
	}
	block := &ast.ExceptionHandlingBlock{BeginToken: p.curToken}
	if !p.accept(token.NEWLINE) {
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
	if p.trace {
		defer un(trace(p, "parseRescueBlock"))
	}
	block := &ast.RescueBlock{Token: p.curToken}
	classes := []*ast.Identifier{}
	for p.peekTokenIs(token.CONST) {
		p.accept(token.CONST)
		class := &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
		classes = append(classes, class)
		if p.peekTokenIs(token.COMMA) {
			p.accept(token.COMMA)
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
	if !p.accept(token.NEWLINE) {
		return nil
	}
	block.Body = p.parseBlockStatement(token.END)
	return block
}

func (p *parser) parseExpressions(left ast.Expression) ast.Expression {
	if p.trace {
		defer un(trace(p, "parseExpressions"))
	}
	p.nextToken()
	// Skip newlines after the comma that triggered this handler.
	for p.currentTokenOneOf(token.NEWLINE, token.SEMICOLON) {
		p.nextToken()
	}
	elements := []ast.Expression{left}
	next := p.parseExpression(precAssignment)
	elements = append(elements, next)
	for p.peekTokenIs(token.COMMA) {
		p.consume(token.COMMA)
		// Skip newlines after each comma.
		for p.currentTokenOneOf(token.NEWLINE, token.SEMICOLON) {
			p.nextToken()
		}
		next = p.parseExpression(precAssignment)
		elements = append(elements, next)
	}
	return ast.ExpressionList(elements)
}

func (p *parser) parseBlockCapture() ast.Expression {
	if p.trace {
		defer un(trace(p, "parseBlockCapture"))
	}
	capture := &ast.BlockCapture{Token: p.curToken}
	if !p.accept(token.IDENT) {
		return nil
	}
	capture.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
	return capture
}

func (p *parser) parseAssignmentOperator(left ast.Expression) ast.Expression {
	if p.trace {
		defer un(trace(p, "parseAssignmentOperator"))
	}
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
	newInf.Right = p.parseExpression(precLowest)
	assign.Right = newInf
	return assign
}

func (p *parser) parseAssignment(left ast.Expression) ast.Expression {
	if p.trace {
		defer un(trace(p, "parseAssignment"))
	}

	switch leftNode := left.(type) {
	case *ast.Identifier:
	case *ast.Global:
	case *ast.IndexExpression:
	case *ast.InstanceVariable:
	case ast.ExpressionList:
	case *ast.Keyword__FILE__:
		epos := p.file.Position(p.pos)
		msg := fmt.Errorf("%s: Can't assign to __FILE__", epos.String())
		p.errors = append(p.errors, msg)
		return nil
	case *ast.ContextCallExpression:
		// obj.method = value => obj.method=(value)
		leftNode.Function.Value += "="
		leftNode.Function.Token.Literal += "="
		p.nextToken()
		right := p.parseExpression(precLowest)
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
	expr := p.parseExpression(precLowest)
	right, ok := expr.(*ast.ConditionalExpression)
	if !ok {
		assign.Right = expr
		return assign
	}
	expStmt, ok := right.Consequence.Statements[0].(*ast.ExpressionStatement)
	if !ok {
		p.errors = append(p.errors, fmt.Errorf("malformed AST in assignment"))
		return nil
	}
	assign.Right = expStmt.Expression
	cond := &ast.ConditionalExpression{
		Token:     right.Token,
		Condition: right.Condition,
		Consequence: &ast.BlockStatement{
			Statements: []ast.Statement{
				&ast.ExpressionStatement{Expression: assign},
			},
		},
	}

	return cond
}

func (p *parser) parseInstanceVariable() ast.Expression {
	if p.trace {
		defer un(trace(p, "parseInstanceVariable"))
	}
	instanceVariable := &ast.InstanceVariable{Token: p.curToken}
	if !p.accept(token.IDENT) {
		return nil
	}
	instanceVariable.Name = p.parseIdentifier().(*ast.Identifier)
	return instanceVariable
}

func (p *parser) parseClassVariable() ast.Expression {
	if p.trace {
		defer un(trace(p, "parseClassVariable"))
	}
	cv := &ast.ClassVariable{Token: p.curToken}
	if !p.accept(token.IDENT) {
		return nil
	}
	cv.Name = p.parseIdentifier().(*ast.Identifier)
	return cv
}

func (p *parser) parseLabelExpression() ast.Expression {
	if p.trace {
		defer un(trace(p, "parseLabelExpression"))
	}
	// The LABEL token's literal is e.g. "foo:"
	name := strings.TrimSuffix(p.curToken.Literal, ":")
	key := &ast.SymbolLiteral{
		Token: p.curToken,
		Value: &ast.StringLiteral{Value: name},
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
	if p.trace {
		defer un(trace(p, "parseRescueModifier"))
	}
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
	if p.trace {
		defer un(trace(p, "parseTopLevelScope"))
	}
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
	if p.trace {
		defer un(trace(p, "parseDefinedExpression"))
	}
	expr := &ast.DefinedExpression{Token: p.curToken}
	// defined? can be: defined?(expr) or defined? expr
	if p.peekTokenIs(token.LPAREN) {
		p.accept(token.LPAREN)
		p.nextToken()
		expr.Expr = p.parseExpression(precLowest)
		if !p.accept(token.RPAREN) {
			return nil
		}
	} else {
		p.nextToken()
		expr.Expr = p.parseExpression(precLowest)
	}
	return expr
}

func (p *parser) parseErrorSkip() ast.Expression {
	p.expectError(token.IDENT) // generic expected error
	return &ast.Nil{Token: p.curToken}
}

func (p *parser) parseJumpExpression() ast.Expression {
	if p.trace {
		defer un(trace(p, "parseJumpExpression"))
	}
	jmp := &ast.JumpExpression{Token: p.curToken}
	// break/next can take an optional value: break expr, next expr
	// redo/retry take no value
	if p.currentTokenIs(token.BREAK) || p.currentTokenIs(token.NEXT) {
		if !p.peekTokenOneOf(token.NEWLINE, token.SEMICOLON, token.EOF) {
			p.nextToken()
			jmp.Value = p.parseExpression(precLowest)
		}
	}
	return jmp
}

func (p *parser) parseCaseExpression() ast.Expression {
	if p.trace {
		defer un(trace(p, "parseCaseExpression"))
	}
	expr := &ast.CaseExpression{Token: p.curToken}
	p.nextToken()
	// Optional case expression: case x
	if !p.currentTokenIs(token.WHEN) && !p.currentTokenIs(token.NEWLINE) && !p.currentTokenIs(token.SEMICOLON) {
		expr.Condition = p.parseExpression(precLowest)
	}
	// Allow optional newline/semicolon after case expression.
	if !p.currentTokenOneOf(token.NEWLINE, token.SEMICOLON) {
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
			p.consume(token.THEN)
		}
		if !p.currentTokenOneOf(token.NEWLINE, token.SEMICOLON) {
			p.acceptOneOf(token.NEWLINE, token.SEMICOLON)
		}
		for p.currentTokenOneOf(token.NEWLINE, token.SEMICOLON) {
			p.nextToken()
		}
		// Parse when body until next when, else, or end.
		wc.Body = p.parseBlockStatement(token.END, token.WHEN, token.KW_IN, token.ELSE)
		expr.WhenClauses = append(expr.WhenClauses, wc)
	}
	// Parse in clauses (pattern matching).
	for p.currentTokenIs(token.KW_IN) || p.peekTokenIs(token.KW_IN) {
		for p.currentTokenOneOf(token.NEWLINE, token.SEMICOLON) {
			p.nextToken()
		}
		if !p.currentTokenIs(token.KW_IN) {
			break
		}
		ic := &ast.WhenClause{Token: p.curToken}
		p.nextToken()
		ic.Conditions = []ast.Expression{p.parseExpression(precLowest)}
		for p.peekTokenIs(token.COMMA) {
			p.consume(token.COMMA)
			ic.Conditions = append(ic.Conditions, p.parseExpression(precLowest))
		}
		if p.peekTokenIs(token.THEN) {
			p.consume(token.THEN)
		}
		if !p.currentTokenOneOf(token.NEWLINE, token.SEMICOLON) {
			p.acceptOneOf(token.NEWLINE, token.SEMICOLON)
		}
		for p.currentTokenOneOf(token.NEWLINE, token.SEMICOLON) {
			p.nextToken()
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
		if !p.currentTokenOneOf(token.NEWLINE, token.SEMICOLON) {
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
	if p.trace {
		defer un(trace(p, "parseNilLiteral"))
	}
	return &ast.Nil{Token: p.curToken}
}

func (p *parser) parseIdentifier() ast.Expression {
	if p.trace {
		defer un(trace(p, "parseIdentifier"))
	}
	return &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
}

func (p *parser) parseGlobal() ast.Expression {
	if p.trace {
		defer un(trace(p, "parseGlobal"))
	}
	return &ast.Global{Token: p.curToken, Value: p.curToken.Literal}
}

func (p *parser) parseScopedIdentifierExpression(outer ast.Expression) ast.Expression {
	if p.trace {
		defer un(trace(p, "parseScopedIdentifierExpression"))
	}
	ident, ok := outer.(*ast.Identifier)
	if !ok {
		return p.parseMethodCall(outer)
	}

	scopedIdent := &ast.ScopedIdentifier{Token: p.curToken, Outer: ident}
	p.nextToken()
	scopedIdent.Inner = p.parseExpression(precLowest)
	return scopedIdent
}

func (p *parser) parseSelf() ast.Expression {
	if p.trace {
		defer un(trace(p, "parseSelf"))
	}
	self := &ast.Self{Token: p.curToken}
	if p.peekTokenOneOf(token.IF, token.UNLESS) {
		return self
	}
	if !p.peekTokenOneOf(token.NEWLINE, token.SEMICOLON, token.DOT, token.EOF) {
		p.peekError(token.NEWLINE, token.SEMICOLON, token.DOT, token.EOF)
		return nil
	}
	return self
}

func (p *parser) parseKeyword__FILE__() ast.Expression {
	if p.trace {
		defer un(trace(p, "parseKeyword__FILE__"))
	}
	file := &ast.Keyword__FILE__{
		Token:    p.curToken,
		Filename: p.file.Name(),
	}
	return file
}

func (p *parser) parseKeyword__LINE__() ast.Expression {
	if p.trace {
		defer un(trace(p, "parseKeyword__LINE__"))
	}
	line := p.file.Position(p.pos).Line
	return &ast.IntegerLiteral{Token: p.curToken, Value: int64(line)}
}

func (p *parser) parseEncodingKeyword() ast.Expression {
	if p.trace {
		defer un(trace(p, "parseEncodingKeyword"))
	}
	return &ast.StringLiteral{Token: p.curToken, Value: "UTF-8"}
}

func (p *parser) parseSplatExpression() ast.Expression {
	if p.trace {
		defer un(trace(p, "parseSplatExpression"))
	}
	expr := &ast.SplatExpression{Token: p.curToken, Operator: p.curToken.Literal}
	p.nextToken()
	expr.Right = p.parseExpression(precPrefix)
	return expr
}

func (p *parser) parseYield() ast.Expression {
	if p.trace {
		defer un(trace(p, "parseYield"))
	}
	yield := &ast.YieldExpression{Token: p.curToken}
	p.nextToken()
	if p.currentTokenIs(token.LPAREN) {
		p.nextToken()
		yield.Arguments = p.parseCallArguments(token.RPAREN)
		p.nextToken()
		return yield
	}
	yield.Arguments = p.parseCallArguments(token.SEMICOLON, token.NEWLINE)
	return yield
}

func (p *parser) parseSuper() ast.Expression {
	if p.trace {
		defer un(trace(p, "parseSuper"))
	}
	sup := &ast.SuperExpression{Token: p.curToken}
	p.nextToken()
	if p.currentTokenIs(token.LPAREN) {
		p.nextToken()
		sup.Arguments = p.parseCallArguments(token.RPAREN)
		p.nextToken()
		return sup
	}
	sup.Arguments = p.parseCallArguments(token.SEMICOLON, token.NEWLINE, token.EOF)
	return sup
}

func (p *parser) parseAlias() ast.Expression {
	if p.trace {
		defer un(trace(p, "parseAlias"))
	}
	expr := &ast.AliasExpression{Token: p.curToken}
	p.nextToken()
	if !p.currentTokenIs(token.IDENT) {
		return nil
	}
	expr.NewName = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
	p.nextToken()
	if !p.currentTokenIs(token.IDENT) {
		return nil
	}
	expr.OldName = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
	p.nextToken()
	return expr
}

func (p *parser) parseUndef() ast.Expression {
	if p.trace {
		defer un(trace(p, "parseUndef"))
	}
	expr := &ast.UndefExpression{Token: p.curToken}
	p.nextToken()
	expr.Names = []*ast.Identifier{{Token: p.curToken, Value: p.curToken.Literal}}
	for p.peekTokenIs(token.COMMA) {
		p.consume(token.COMMA)
		expr.Names = append(expr.Names, &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal})
	}
	return expr
}

func (p *parser) parseBeginlessRange() ast.Expression {
	if p.trace {
		defer un(trace(p, "parseBeginlessRange"))
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

func (p *parser) parseLambda() ast.Expression {
	if p.trace {
		defer un(trace(p, "parseLambda"))
	}
	lit := &ast.FunctionLiteral{Token: p.curToken, IsLambda: true}
	// Optional parameters: ->(x, y) or bare ->
	if p.peekTokenIs(token.LPAREN) {
		lit.Parameters = p.parseParameters(token.LPAREN, token.RPAREN)
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
	if p.trace {
		defer un(trace(p, "parseIntegerLiteral"))
	}
	lit := &ast.IntegerLiteral{Token: p.curToken}
	s := integerLiteralReplacer.Replace(p.curToken.Literal)
	s = strings.TrimRight(s, "riRI")
	v, bigV, err := parseRubyInt(s)
	if err != nil {
		msg := fmt.Errorf("could not parse %q as integer", p.curToken.Literal)
		p.errors = append(p.errors, msg)
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
	if p.trace {
		defer un(trace(p, "parseFloatLiteral"))
	}
	lit := &ast.FloatLiteral{Token: p.curToken}
	s := integerLiteralReplacer.Replace(p.curToken.Literal)
	// Strip rational/complex suffixes -- the numeric value is the same.
	s = strings.TrimRight(s, "riRI")
	value, err := parseFloat(s)
	if err != nil {
		msg := fmt.Errorf("could not parse %q as float", p.curToken.Literal)
		p.errors = append(p.errors, msg)
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
	if p.trace {
		defer un(trace(p, "parseStringLiteral"))
	}
	return &ast.StringLiteral{Token: p.curToken, Value: p.curToken.Literal}
}

func (p *parser) parseInterpolatedString() ast.Expression {
	if p.trace {
		defer un(trace(p, "parseInterpolatedString"))
	}
	sl := &ast.StringLiteral{Token: p.curToken} // STRING_BEG
	p.nextToken()                               // advance to first content token

	var parts []ast.Expression
	for !p.currentTokenIs(token.STRING_END) && !p.currentTokenIs(token.XSTR_END) && !p.currentTokenIs(token.EOF) {
		switch p.curToken.Type {
		case token.STRING_CONTENT, token.XSTR_CONTENT:
			parts = append(parts, &ast.StringContent{Token: p.curToken, Value: p.curToken.Literal})
		case token.EMBEXPR_BEG:
			p.nextToken() // advance past EMBEXPR_BEG to first expression token
			exp := p.parseExpression(precLowest)
			if exp != nil {
				parts = append(parts, exp)
			}
			if !p.peekTokenIs(token.EMBEXPR_END) {
				p.peekError(token.EMBEXPR_END)
				return nil
			}
			p.nextToken() // consume EMBEXPR_END
		default:
			p.expectError(token.STRING_CONTENT, token.EMBEXPR_BEG, token.STRING_END)
			return nil
		}
		p.nextToken()
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
	if p.trace {
		defer un(trace(p, "parseInterpolatedRegex"))
	}
	rl := &ast.RegexLiteral{Token: p.curToken} // REGEX_BEG
	p.nextToken()                              // advance to first content token

	var parts []ast.Expression
	for !p.currentTokenIs(token.REGEX_END) && !p.currentTokenIs(token.EOF) {
		switch p.curToken.Type {
		case token.STRING_CONTENT:
			parts = append(parts, &ast.StringContent{Token: p.curToken, Value: p.curToken.Literal})
		case token.EMBEXPR_BEG:
			p.nextToken()
			exp := p.parseExpression(precLowest)
			if exp != nil {
				parts = append(parts, exp)
			}
			if !p.peekTokenIs(token.EMBEXPR_END) {
				p.peekError(token.EMBEXPR_END)
				return nil
			}
			p.nextToken()
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

func (p *parser) parseSymbolLiteral() ast.Expression {
	if p.trace {
		defer un(trace(p, "parseSymbolLiteral"))
	}
	symbol := &ast.SymbolLiteral{Token: p.curToken}
	if !p.acceptOneOf(token.IDENT, token.CONST, token.AT, token.STRING, token.STRING_BEG, token.CLASS_VAR, token.GLOBAL) {
		return nil
	}
	val := p.parseExpression(precHighest)
	symbol.Value = val
	return symbol
}

func (p *parser) parseArrayLiteral() ast.Expression {
	if p.trace {
		defer un(trace(p, "parseArrayLiteral"))
	}
	array := &ast.ArrayLiteral{Token: p.curToken}

	p.nextToken()
	array.Elements = p.parseExpressionList(token.RBRACKET)
	array.Rbracket = p.curToken
	return array
}

func (p *parser) parseBoolean() ast.Expression {
	if p.trace {
		defer un(trace(p, "parseBoolean"))
	}
	return &ast.Boolean{Token: p.curToken, Value: p.currentTokenIs(token.TRUE)}
}

func (p *parser) parseHash() ast.Expression {
	hash := &ast.HashLiteral{Token: p.curToken, Map: make(map[ast.Expression]ast.Expression)}
	if p.trace {
		defer un(trace(p, "parseHash"))
	}
	p.nextToken()

	if p.currentTokenIs(token.RBRACE) {
		hash.Rbrace = p.curToken
		return hash
	}

	k, v, ok := p.parseKeyValue()
	if !ok {
		return nil
	}
	hash.Map[k] = v

	for p.peekTokenIs(token.COMMA) {
		p.consume(token.COMMA)
		k, v, ok := p.parseKeyValue()
		if !ok {
			return nil
		}
		hash.Map[k] = v
	}

	if !p.accept(token.RBRACE) {
		return nil
	}
	hash.Rbrace = p.curToken
	return hash
}

func (p *parser) parseKeyValue() (ast.Expression, ast.Expression, bool) {
	// Label syntax: key: value (Ruby 1.9+)
	if p.currentTokenIs(token.LABEL) {
		name := strings.TrimSuffix(p.curToken.Literal, ":")
		key := &ast.SymbolLiteral{
			Token: p.curToken,
			Value: &ast.StringLiteral{Value: name},
		}
		p.nextToken()
		val := p.parseExpression(precAssignment)
		return key, val, true
	}
	// Classic hashrocket syntax: key => value
	key := p.parseExpression(precAssignment)
	if !p.consume(token.HASHROCKET) {
		return nil, nil, false
	}
	val := p.parseExpression(precAssignment)
	return key, val, true
}

func (p *parser) parseBlock() ast.Expression {
	if p.trace {
		defer un(trace(p, "parseBlock"))
	}
	block := &ast.BlockExpression{Token: p.curToken}
	if p.peekTokenIs(token.PIPE) {
		block.Parameters = p.parseParameters(token.PIPE, token.PIPE)
	}

	if p.peekTokenOneOf(token.NEWLINE, token.SEMICOLON) {
		p.acceptOneOf(token.NEWLINE, token.SEMICOLON)
	}

	endToken := token.RBRACE
	if block.Token.Type == token.DO {
		endToken = token.END
	}

	block.Body = p.parseBlockStatement(endToken)
	p.nextToken()
	block.EndToken = p.curToken
	return block
}

func (p *parser) parsePrefixExpression() ast.Expression {
	if p.trace {
		defer un(trace(p, "parsePrefixExpression"))
	}
	expression := &ast.PrefixExpression{
		Token:    p.curToken,
		Operator: p.curToken.Literal,
	}
	p.nextToken()
	expression.Right = p.parseExpression(precPrefix)
	return expression
}

func (p *parser) parseInfixExpression(left ast.Expression) ast.Expression {
	if p.trace {
		defer un(trace(p, "parseInfixExpression"))
	}
	expression := &ast.InfixExpression{
		Token:    p.curToken,
		Operator: p.curToken.Literal,
		Left:     left,
	}
	precedence := p.curPrecedence()
	p.nextToken()
	expression.Right = p.parseExpression(precedence)
	return expression
}

func (p *parser) parseIndexExpression(left ast.Expression) ast.Expression {
	if p.trace {
		defer un(trace(p, "parseIndexExpression"))
	}
	exp := &ast.IndexExpression{Token: p.curToken, Left: left}

	p.nextToken()
	// Empty index: x[] -- ] immediately follows [
	if p.currentTokenIs(token.RBRACKET) {
		p.nextToken() // consume ]
		return exp
	}
	exp.Index = p.parseExpression(precLowest)
	if elist, ok := exp.Index.(ast.ExpressionList); ok {
		exp.Index = elist[0]
		exp.Length = elist[1]
	}

	// Endless range like x[3..] leaves ] as curToken.
	if p.currentTokenIs(token.RBRACKET) {
		// already consumed by the parse, just skip it
	} else if !p.accept(token.RBRACKET) {
		return nil
	}
	return exp
}

func (p *parser) parseGroupedExpression() ast.Expression {
	if p.trace {
		defer un(trace(p, "parseGroupedExpression"))
	}
	p.nextToken()
	exp := p.parseExpression(precLowest)
	if !p.accept(token.RPAREN) {
		return nil
	}
	return exp
}

func (p *parser) parseIfExpression() ast.Expression {
	if p.trace {
		defer un(trace(p, "parseIfExpression"))
	}
	expression := &ast.ConditionalExpression{Token: p.curToken}
	p.nextToken()
	expression.Condition = p.parseExpression(precLowest)
	if p.peekTokenIs(token.THEN) {
		p.accept(token.THEN)
	}

	if !p.peekTokenOneOf(token.NEWLINE, token.SEMICOLON) {
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
	p.acceptOneOf(token.NEWLINE, token.SEMICOLON)
	consequence := p.parseBlockStatement(token.ELSE)
	expression.Consequence = consequence
	if p.peekTokenIs(token.ELSE) {
		p.accept(token.ELSE)
		p.accept(token.NEWLINE)
		expression.Alternative = p.parseBlockStatement()
	}
	p.accept(token.END)
	expression.EndToken = p.curToken
	return expression
}

func (p *parser) parseTenaryIfExpression(condition ast.Expression) ast.Expression {
	if p.trace {
		defer un(trace(p, "parseTenaryIfExpression"))
	}
	expression := &ast.ConditionalExpression{Token: p.curToken}
	p.nextToken()
	expression.Condition = condition
	expression.Consequence = &ast.BlockStatement{
		Statements: []ast.Statement{
			&ast.ExpressionStatement{
				Expression: p.parseExpression(precTenary),
			},
		},
	}
	p.consume(token.COLON)
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
	if p.trace {
		defer un(trace(p, "parseModifierConditionalExpression"))
	}
	expression := &ast.ConditionalExpression{Token: p.curToken}
	p.nextToken()
	expression.Condition = p.parseExpression(precLowest)

	expression.Consequence = &ast.BlockStatement{
		Statements: []ast.Statement{
			&ast.ExpressionStatement{Expression: left},
		},
	}
	return expression
}

func (p *parser) parseLoopExpression() ast.Expression {
	if p.trace {
		defer un(trace(p, "parseLoopExpression"))
	}
	loop := &ast.LoopExpression{Token: p.curToken}
	p.nextToken()
	loop.Condition = p.parseExpression(precBlockDo)
	if p.peekTokenIs(token.DO) {
		p.accept(token.DO)
	}
	loop.Block = p.parseBlockStatement(token.END)
	p.nextToken()
	return loop
}

func (p *parser) parseModule() ast.Expression {
	if p.trace {
		defer un(trace(p, "parseModule"))
	}
	expr := &ast.ModuleExpression{Token: p.curToken}
	if !p.accept(token.CONST) {
		return nil
	}
	expr.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}

	if !p.acceptOneOf(token.NEWLINE, token.SEMICOLON) {
		return nil
	}

	expr.Body = p.parseBlockStatement()

	if !p.accept(token.END) {
		return nil
	}
	expr.EndToken = p.curToken
	return expr
}

func (p *parser) parseClass() ast.Expression {
	if p.trace {
		defer un(trace(p, "parseClass"))
	}
	if p.peekTokenIs(token.LSHIFT) {
		return p.parseSingletonClass()
	}
	expr := &ast.ClassExpression{Token: p.curToken}
	if !p.accept(token.CONST) {
		return nil
	}
	expr.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}

	if p.peekTokenIs(token.LT) {
		p.consume(token.LT)
		expr.SuperClass = p.parseExpression(precLowest)
	}

	if !p.acceptOneOf(token.NEWLINE, token.SEMICOLON) {
		return nil
	}

	expr.Body = p.parseBlockStatement()

	if !p.accept(token.END) {
		return nil
	}
	expr.EndToken = p.curToken
	return expr
}

func (p *parser) parseSingletonClass() ast.Expression {
	if p.trace {
		defer un(trace(p, "parseSingletonClass"))
	}
	expr := &ast.SingletonClassExpression{Token: p.curToken}

	if !p.consume(token.LSHIFT) {
		return nil
	}

	expr.Expr = p.parseExpression(precLowest)

	if !p.acceptOneOf(token.NEWLINE, token.SEMICOLON) {
		return nil
	}

	expr.Body = p.parseBlockStatement()

	if !p.accept(token.END) {
		return nil
	}
	expr.EndToken = p.curToken
	return expr
}

func (p *parser) parseFunctionLiteral() ast.Expression {
	if p.trace {
		defer un(trace(p, "parseFunctionLiteral"))
	}
	lit := &ast.FunctionLiteral{Token: p.curToken}

	if !p.peekTokenOneOf(token.IDENT, token.SELF, token.CONST) && !p.peekToken.Type.IsOperator() {
		p.peekError(token.IDENT, token.CONST)
		return nil
	}

	if p.peekTokenOneOf(token.IDENT, token.SELF, token.CONST) {
		p.acceptOneOf(token.IDENT, token.SELF, token.CONST)
		if p.peekTokenIs(token.DOT) {
			lit.Receiver = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
			p.accept(token.DOT)
			if !p.peekTokenOneOf(token.IDENT, token.SELF, token.CONST) && !p.peekToken.Type.IsOperator() {
				p.peekError(token.IDENT, token.CONST)
				return nil
			}
			p.nextToken()
			lit.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
		} else {
			lit.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
		}
	} else {
		p.nextToken()
		lit.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
	}

	lit.Parameters = p.parseParameters(token.LPAREN, token.RPAREN)

	if p.currentTokenOneOf(token.CAPTURE, token.AND) {
		// Anonymous block forwarding: & without name
		if p.peekTokenOneOf(token.NEWLINE, token.SEMICOLON, token.EOF, token.RPAREN) {
			if p.peekTokenIs(token.RPAREN) {
				p.accept(token.RPAREN)
			}
		} else {
			capture := p.parseBlockCapture()
			if capture == nil {
				return nil
			}
			lit.CapturedBlock = capture.(*ast.BlockCapture)
			if p.peekTokenIs(token.RPAREN) {
				p.acceptOneOf(token.RPAREN)
			}
		}
	}

	if !p.acceptOneOf(token.NEWLINE, token.SEMICOLON) {
		return nil
	}
	lit.Body = p.parseBlockStatement(token.END, token.RESCUE)
	lit.Rescues = []*ast.RescueBlock{}
	for p.peekTokenIs(token.RESCUE) {
		p.accept(token.RESCUE)
		rescue := p.parseRescueBlock()
		lit.Rescues = append(lit.Rescues, rescue)
	}
	if !p.accept(token.END) {
		return nil
	}
	lit.EndToken = p.curToken
	inspect := func(n ast.Node) bool {
		x, ok := n.(*ast.Assignment)
		if !ok {
			return true
		}
		switch left := x.Left.(type) {
		case *ast.Identifier:
			if left.IsConstant() {
				p.errors = append(p.errors, fmt.Errorf("dynamic constant assignment"))
			}
		case ast.ExpressionList:
			for _, expr := range left {
				if ident, ok := expr.(*ast.Identifier); ok {
					if ident.IsConstant() {
						p.errors = append(p.errors, fmt.Errorf("dynamic constant assignment"))
					}
				}
			}
		}
		return true
	}
	if lit.Body != nil {
		ast.Inspect(lit.Body, inspect)
	}
	return lit
}

func (p *parser) parseParameters(startToken, endToken token.Type) []*ast.FunctionParameter {
	if p.trace {
		defer un(trace(p, "parseParameters"))
	}
	hasDelimiters := false
	if p.peekTokenIs(startToken) {
		hasDelimiters = true
		p.accept(startToken)
	}

	identifiers := []*ast.FunctionParameter{}

	if !hasDelimiters && p.peekTokenIs(endToken) {
		p.peekError(token.NEWLINE, token.SEMICOLON)
		return nil
	}

	if hasDelimiters && p.peekTokenIs(endToken) {
		p.accept(endToken)
		return identifiers
	}

	if !hasDelimiters && p.peekTokenOneOf(token.NEWLINE, token.SEMICOLON) {
		return identifiers
	}

	// Forwarding: def foo(...)
	if p.peekTokenIs(token.RANGEEX) {
		p.accept(token.RANGEEX)
		identifiers = append(identifiers, &ast.FunctionParameter{IsForwarding: true})
		if hasDelimiters {
			p.accept(endToken)
		}
		return identifiers
	}

	if p.peekTokenIs(token.POWER) {
		p.accept(token.POWER)
		if p.peekTokenIs(token.IDENT) || p.peekTokenIs(token.CONST) {
			p.accept(token.IDENT)
		}
		kp := &ast.FunctionParameter{
			Name:          &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal},
			IsKeywordRest: true,
		}
		identifiers = append(identifiers, kp)
		if hasDelimiters {
			p.accept(endToken)
		}
		return identifiers
	}

	if p.peekTokenIs(token.ASTERISK) {
		p.accept(token.ASTERISK)
	}
	if p.peekTokenOneOf(token.CAPTURE, token.AND) {
		p.acceptOneOf(token.CAPTURE, token.AND)
		// Anonymous block forwarding: def foo(&)
		if p.peekTokenOneOf(token.COMMA, token.RPAREN, token.NEWLINE, token.SEMICOLON, token.EOF) {
			if hasDelimiters {
				p.accept(endToken)
			}
			return identifiers
		}
		// Named block capture: let parseFunctionLiteral handle it
		return identifiers
	}
	// First param: accept IDENT or LABEL (def foo(a:))
	if p.peekTokenIs(token.LABEL) {
		p.accept(token.LABEL)
	} else {
		p.accept(token.IDENT)
	}

	name := p.curToken.Literal
	isKeyword := strings.HasSuffix(name, ":")
	if isKeyword {
		name = strings.TrimSuffix(name, ":")
	}
	ident := &ast.FunctionParameter{Name: &ast.Identifier{Token: p.curToken, Value: name}, IsKeyword: isKeyword}
	if p.peekTokenIs(token.ASSIGN) {
		p.consume(token.ASSIGN)
		ident.Default = p.parseExpression(precAssignment)
	}
	identifiers = append(identifiers, ident)

	for p.peekTokenIs(token.COMMA) {
		p.accept(token.COMMA)
		if p.peekTokenIs(token.POWER) {
			p.accept(token.POWER)
			if p.peekTokenIs(token.IDENT) || p.peekTokenIs(token.CONST) {
				p.accept(token.IDENT)
			}
			kp := &ast.FunctionParameter{
				Name:          &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal},
				IsKeywordRest: true,
			}
			identifiers = append(identifiers, kp)
			if hasDelimiters {
				p.accept(endToken)
			}
			return identifiers
		}
		if p.peekTokenIs(token.ASTERISK) {
			p.accept(token.ASTERISK)
		}
		if p.peekTokenOneOf(token.CAPTURE, token.AND) {
			p.acceptOneOf(token.CAPTURE, token.AND)
			// Anonymous block forwarding: & without name
			if p.peekTokenOneOf(token.COMMA, token.RPAREN, token.NEWLINE, token.SEMICOLON, token.EOF) {
				if hasDelimiters {
					p.accept(endToken)
				}
				return identifiers
			}
			// Named block capture: let parseFunctionLiteral handle it
			return identifiers
		}
		isKw := false
		if p.peekTokenIs(token.LABEL) {
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
		if p.peekTokenIs(token.ASSIGN) {
			p.consume(token.ASSIGN)
			pIdent.Default = p.parseExpression(precPrefix)
		}
		identifiers = append(identifiers, pIdent)
	}

	if !hasDelimiters && p.peekTokenIs(endToken) {
		p.peekError(endToken)
		return nil
	}

	if hasDelimiters && p.peekTokenIs(endToken) {
		p.accept(endToken)
	}

	return identifiers
}

func (p *parser) parseBlockStatement(t ...token.Type) *ast.BlockStatement {
	if p.trace {
		defer un(trace(p, "parseBlockStatement"))
	}
	terminatorTokens := append(
		[]token.Type{
			token.END,
		},
		t...,
	)
	block := &ast.BlockStatement{Token: p.curToken}
	block.Statements = []ast.Statement{}

	for !p.peekTokenOneOf(terminatorTokens...) {
		if p.peekTokenIs(token.EOF) {
			p.peekError(token.EOF)
			return block
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
	if p.trace {
		defer un(trace(p, "parseMethodCall"))
	}
	contextCallExpression := &ast.ContextCallExpression{Token: p.curToken, Context: context}

	p.nextToken()

	if !p.currentTokenOneOf(token.IDENT, token.CLASS) && !p.curToken.Type.IsOperator() {
		p.expectError(token.IDENT, token.CLASS)
		return nil
	}

	function := &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
	contextCallExpression.Function = function

	if p.peekTokenOneOf(token.SEMICOLON, token.NEWLINE, token.EOF, token.DOT, token.SCOPE) {
		contextCallExpression.Arguments = []ast.Expression{}
		return contextCallExpression
	}

	if p.peekTokenIs(token.LPAREN) {
		p.accept(token.LPAREN)
		p.nextToken()
		contextCallExpression.Arguments = p.parseExpressionList(token.RPAREN)
		if p.peekTokenOneOf(token.LBRACE, token.DO) {
			p.acceptOneOf(token.LBRACE, token.DO)
			contextCallExpression.Block = p.parseBlock().(*ast.BlockExpression)
		}
		return contextCallExpression
	}

	if p.peekTokenOneOf(append(tokensNotPossibleInCallArgs, token.RBRACE, token.RPAREN, token.EMBEXPR_END, token.SEMICOLON, token.EOF)...) {
		return contextCallExpression
	}

	p.nextToken()

	contextCallExpression.Arguments = p.parseCallArguments(
		token.LBRACE, token.DO,
	)
	if p.currentTokenOneOf(token.LBRACE, token.DO) {
		contextCallExpression.Block = p.parseBlock().(*ast.BlockExpression)
	}
	return contextCallExpression
}

func (p *parser) parseContextCallExpression(context ast.Expression) ast.Expression {
	if p.trace {
		defer un(trace(p, "parseContextCallExpression"))
	}
	contextCallExpression := &ast.ContextCallExpression{Token: p.curToken, Context: context}
	if _, ok := context.(*ast.Self); ok && !p.currentTokenOneOf(token.DOT, token.SCOPE) {
		p.expectError(token.DOT, token.SCOPE)
		return nil
	}
	if p.currentTokenOneOf(token.DOT, token.SCOPE) {
		p.nextToken()
	}

	if !p.currentTokenOneOf(token.IDENT, token.CLASS) {
		p.expectError(token.IDENT, token.CLASS)
		return nil
	}

	function := p.parseIdentifier()
	ident := function.(*ast.Identifier)
	contextCallExpression.Function = ident

	if p.peekTokenOneOf(token.SEMICOLON, token.NEWLINE, token.DOT, token.SCOPE) {
		contextCallExpression.Arguments = []ast.Expression{}
		return contextCallExpression
	}

	if p.peekTokenIs(token.LPAREN) {
		p.accept(token.LPAREN)
		p.nextToken()
		contextCallExpression.Arguments = p.parseExpressionList(token.RPAREN)
		if p.peekTokenOneOf(token.LBRACE, token.DO) {
			p.acceptOneOf(token.LBRACE, token.DO)
			contextCallExpression.Block = p.parseBlock().(*ast.BlockExpression)
		}
		return contextCallExpression
	}

	if p.peekTokenOneOf(append(tokensNotPossibleInCallArgs, token.RBRACE, token.RPAREN, token.EMBEXPR_END, token.SEMICOLON, token.EOF)...) {
		return contextCallExpression
	}

	p.nextToken()
	contextCallExpression.Arguments = p.parseCallArguments(
		token.LBRACE, token.DO,
	)
	if p.currentTokenOneOf(token.LBRACE, token.DO) {
		contextCallExpression.Block = p.parseBlock().(*ast.BlockExpression)
	}
	return contextCallExpression
}

func (p *parser) parseCallArgument(function ast.Expression) ast.Expression {
	if p.trace {
		defer un(trace(p, "parseCallArgument"))
	}
	ident, ok := function.(*ast.Identifier)
	if !ok {
		// method call on any other object
		return p.parseContextCallExpression(function)
	}
	exp := &ast.ContextCallExpression{Token: ident.Token, Function: ident}
	if p.currentTokenOneOf(token.LBRACE, token.DO) {
		exp.Block = p.parseBlock().(*ast.BlockExpression)
		return exp
	}

	exp.Arguments = p.parseExpressionList(token.SEMICOLON, token.NEWLINE, token.SCOPE)
	if p.peekTokenOneOf(token.LBRACE, token.DO) {
		p.acceptOneOf(token.LBRACE, token.DO)
		exp.Block = p.parseBlock().(*ast.BlockExpression)
	}
	return exp
}

func (p *parser) parseCallBlock(function ast.Expression) ast.Expression {
	if p.trace {
		defer un(trace(p, "parseCallBlock"))
	}

	exp := &ast.ContextCallExpression{Token: p.curToken}
	exp.Block = p.parseBlock().(*ast.BlockExpression)
	switch fn := function.(type) {
	case *ast.Identifier:
		exp.Function = fn
		return exp
	case *ast.InfixExpression:
		ident, ok := fn.Right.(*ast.Identifier)
		if !ok {
			break
		}
		exp.Function = ident
		fn.Right = exp
		return fn
	}
	msg := fmt.Errorf("could not parse call expression: expected identifier, got token '%T'", function)
	p.errors = append(p.errors, msg)
	return nil
}

func (p *parser) parseCallExpressionWithParens(function ast.Expression) ast.Expression {
	if p.trace {
		defer un(trace(p, "parseCallExpressionWithParens"))
	}
	exp := &ast.ContextCallExpression{Token: p.curToken}
	if ident, ok := function.(*ast.Identifier); ok {
		exp.Function = ident
	} else {
		// Non-identifier callable: @ivar(args), method_returning_proc(args)
		exp.Context = function
		exp.Function = &ast.Identifier{Token: p.curToken, Value: "call"}
	}
	p.nextToken()
	exp.Arguments = p.parseExpressionList(token.RPAREN)
	if p.peekTokenOneOf(token.LBRACE, token.DO) {
		p.acceptOneOf(token.LBRACE, token.DO)
		exp.Block = p.parseBlock().(*ast.BlockExpression)
	}
	return exp
}

func (p *parser) parseCallArguments(end ...token.Type) []ast.Expression {
	if p.trace {
		defer un(trace(p, "parseCallArguments"))
	}
	list := []ast.Expression{}
	if p.currentTokenOneOf(end...) {
		return list
	}

	list = append(list, p.parseExpression(precAssignment))

	for p.peekTokenIs(token.COMMA) {
		p.consume(token.COMMA)
		list = append(list, p.parseExpression(precAssignment))
	}

	if p.peekTokenOneOf(end...) {
		p.acceptOneOf(end...)
	}

	return list
}

func (p *parser) parseExpressionList(end ...token.Type) []ast.Expression {
	if p.trace {
		defer un(trace(p, "parseExpressionList"))
	}
	list := []ast.Expression{}
	// Skip leading newlines/semicolons inside parens/brackets.
	for p.currentTokenOneOf(token.NEWLINE, token.SEMICOLON) {
		p.nextToken()
	}
	if p.currentTokenOneOf(end...) {
		return list
	}

	next := p.parseExpression(precIfUnless)
	if elist, ok := next.(ast.ExpressionList); ok {
		if p.peekTokenOneOf(end...) {
			p.acceptOneOf(end...)
		}
		return elist
	}
	list = append(list, next)

	// After a single expression, check for end.
	if p.peekTokenOneOf(end...) {
		p.acceptOneOf(end...)
	}

	return list
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
