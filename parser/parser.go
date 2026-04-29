package parser

import (
	"fmt"
	gotoken "go/token"
	"strconv"
	"strings"

	"math/rand"

	"github.com/lczyk/goruby/ast"
	"github.com/lczyk/goruby/ast/infix"
	"github.com/lczyk/goruby/lexer"
	"github.com/lczyk/goruby/token"
	"github.com/lczyk/goruby/trace"
	"github.com/pkg/errors"
)

// Possible precedence values
const (
	_ int = iota
	precLowest
	precBlockBraces // { |x| }
	precIfUnless    // modifier-if, modifier-unless
	precAssignment  // x = 5
	precTernary     // ?, :
	precRange       // .., ...
	precLogicalOr   // ||
	precLogicalAnd  // &&
	precEquals      // ==, !=, <=>
	precLessGreater // >, <, >=, <=
	precOr          // |
	precAnd         // &
	precShift       // <<
	precSum         // + or -
	precProduct     // *, /, %
	precPower       // **
	precPrefix      // -X or !X
	precSplat       // x = [*y, 1]
	precCallArg     // func x
	precCall        // foo.myFunction(X)
	precIndex       // array[index]
	precSymbol      // :Symbol
	precHighest
)

var precedences = map[token.Type]int{
	token.IF:         precIfUnless,
	token.UNLESS:     precIfUnless,
	token.EQ:         precEquals,
	token.NOTEQ:      precEquals,
	token.SPACESHIP:  precEquals,
	token.LSHIFT:     precShift,
	token.QMARK:      precTernary,
	token.COLON:      precTernary,
	token.LT:         precLessGreater,
	token.GT:         precLessGreater,
	token.LTE:        precLessGreater,
	token.GTE:        precLessGreater,
	token.PLUS:       precSum,
	token.MINUS:      precSum,
	token.SLASH:      precProduct,
	token.ASTERISK:   precProduct,
	token.POW:        precPower,
	token.MODULO:     precProduct,
	token.ASSIGN:     precAssignment,
	token.ADDASSIGN:  precAssignment,
	token.SUBASSIGN:  precAssignment,
	token.MULASSIGN:  precAssignment,
	token.DIVASSIGN:  precAssignment,
	token.MODASSIGN:  precAssignment,
	token.LPAREN:     precCall,
	token.DOT:        precCall,
	token.DDOT:       precRange,
	token.DDDOT:      precRange,
	token.IDENT:      precCallArg,
	token.INT:        precCallArg,
	token.STRING:     precCallArg,
	token.SLBRACKET:  precCallArg,
	token.LBRACKET:   precIndex,
	token.LBRACE:     precBlockBraces,
	token.SYMBOL:     precSymbol,
	token.COMMA:      precAssignment,
	token.THEN:       precHighest,
	token.NEWLINE:    precHighest,
	token.PIPE:       precOr,
	token.AND:        precAnd,
	token.LOGICALOR:  precLogicalOr,
	token.LOGICALAND: precLogicalAnd,
}

var tokensNotPossibleInCallArgs = []token.Type{
	token.ASSIGN,
	token.LT,
	token.LTE,
	token.GT,
	token.GTE,
	token.SPACESHIP,
	token.LSHIFT,
	token.EQ,
	token.NOTEQ,
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

// A parser parses the token emitted by the provided lexer.Lexer and returns an
// AST describing the parsed program.
type parser struct {
	file   *gotoken.File
	l      *lexer.Lexer
	errors []error

	tracer trace.Tracer

	pos       gotoken.Pos
	lastLine  string
	curToken  token.Token
	peekToken token.Token

	prefixParseFns map[token.Type]prefixParseFn
	infixParseFns  map[token.Type]infixParseFn
}

func (p *parser) init(fset *gotoken.FileSet, filename string, src []byte, trace_parse bool) {
	p.file = fset.AddFile(filename, -1, len(src))

	p.l = lexer.New(string(src))
	p.errors = []error{}
	if trace_parse {
		p.tracer = trace.NewTracer()
	}

	p.prefixParseFns = make(map[token.Type]prefixParseFn)
	p.registerPrefix(token.IDENT, p.parseIdentifier)
	p.registerPrefix(token.INT, p.parseIntegerLiteral)
	p.registerPrefix(token.FLOAT, p.parseFloatLiteral)
	p.registerPrefix(token.STRING, p.parseStringLiteral)
	p.registerPrefix(token.BANG, p.parsePrefixExpression)
	p.registerPrefix(token.MINUS, p.parsePrefixExpression)
	p.registerPrefix(token.TRUE, p.parseBoolean)
	p.registerPrefix(token.FALSE, p.parseBoolean)
	p.registerPrefix(token.LPAREN, p.parseGroupedExpression)
	p.registerPrefix(token.IF, p.parseIfExpression)
	p.registerPrefix(token.UNLESS, p.parseIfExpression)
	p.registerPrefix(token.LOOP, p.parseLoopExpression)
	p.registerPrefix(token.DEF, p.parseFunctionLiteral)
	p.registerPrefix(token.SYMBOL, p.parseSymbolLiteral)
	p.registerPrefix(token.LBRACKET, p.parseArrayLiteral)
	p.registerPrefix(token.SLBRACKET, p.parseArrayLiteral)
	p.registerPrefix(token.NIL, p.parseNilLiteral)
	p.registerPrefix(token.LBRACE, p.parseHash)
	p.registerPrefix(token.LAMBDAROCKET, p.parseLambdaLiteral)
	p.registerPrefix(token.ASTERISK, p.parseSplat)

	p.infixParseFns = make(map[token.Type]infixParseFn)
	p.registerInfix(token.PLUS, p.parseInfixExpression)
	p.registerInfix(token.MINUS, p.parseInfixExpression)
	p.registerInfix(token.SLASH, p.parseInfixExpression)
	p.registerInfix(token.ASTERISK, p.parseInfixExpression)
	p.registerInfix(token.POW, p.parseInfixExpression)
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
	p.registerInfix(token.SPACESHIP, p.parseInfixExpression)
	p.registerInfix(token.LSHIFT, p.parseInfixExpression)
	p.registerInfix(token.DDOT, p.parseRangeLiteral)
	p.registerInfix(token.DDDOT, p.parseRangeLiteral)
	p.registerInfix(token.ASSIGN, p.parseAssignment)
	p.registerInfix(token.ADDASSIGN, p.parseAssignmentOperator)
	p.registerInfix(token.SUBASSIGN, p.parseAssignmentOperator)
	p.registerInfix(token.MULASSIGN, p.parseAssignmentOperator)
	p.registerInfix(token.DIVASSIGN, p.parseAssignmentOperator)
	p.registerInfix(token.MODASSIGN, p.parseAssignmentOperator)
	p.registerInfix(token.IF, p.parseModifierConditionalExpression)
	p.registerInfix(token.UNLESS, p.parseModifierConditionalExpression)
	p.registerInfix(token.QMARK, p.parseTernaryIfExpression)
	p.registerInfix(token.LPAREN, p.parseCallExpressionWithParens)
	p.registerInfix(token.IDENT, p.parseCallArgument)
	p.registerInfix(token.INT, p.parseCallArgument)
	p.registerInfix(token.STRING, p.parseCallArgument)
	p.registerInfix(token.SYMBOL, p.parseCallArgument)
	p.registerInfix(token.LBRACE, p.parseCallBlock)
	p.registerInfix(token.DOT, p.parseMethodCall)
	p.registerInfix(token.COMMA, p.parseExpressions)
	p.registerInfix(token.LBRACKET, p.parseIndexExpression)
	p.registerInfix(token.SLBRACKET, p.parseCallArgument)

	// Read two tokens, so curToken and peekToken are both set
	p.nextToken()
	p.nextToken()
}

// func (p *parser) printTrace(a ...interface{}) string {
// 	const dots = ". . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . "
// 	const n = len(dots)
// 	pos := p.file.Position(p.pos)
// 	fmt.Printf("%5d:%3d: ", pos.Line, pos.Column)
// 	i := 2 * p.indent
// 	for i > n {
// 		fmt.Print(dots)
// 		i -= n
// 	}
// 	// i <= n
// 	// fmt.Print(dots[0:i])
// 	// fmt.Println(a...)
// }

func (p *parser) registerPrefix(tokenType token.Type, fn prefixParseFn) {
	p.prefixParseFns[tokenType] = fn
}

func (p *parser) registerInfix(tokenType token.Type, fn infixParseFn) {
	p.infixParseFns[tokenType] = fn
}

func (p *parser) nextToken() {
	// The very first token (!p.pos.IsValid()) is not initialized
	// (it is token.ILLEGAL), so don't print it .
	if p.pos.IsValid() {
		// s := p.peekToken.Type.String()
		// switch {
		// case p.peekToken.IsLiteral():
		// 	p.printTrace(s, p.peekToken.Literal)
		// case p.peekToken.IsOperator(), p.peekToken.IsKeyword():
		// 	p.printTrace("\"" + s + "\"")
		// default:
		// 	p.printTrace(s)
		// }
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
	} else {
		p.peekToken = token.NewToken(token.EOF, "", -1)
	}
}

func (p *parser) parseIdentifier() ast.Expression {
	if p.tracer != nil {
		defer p.tracer.Un(p.tracer.Trace(trace.Here()))
		p.tracer.Message(p.curToken.Literal)
	}
	return &ast.Identifier{Value: p.curToken.Literal}
}

// Errors returns all errors which happened during the parsing of the input.
func (p *parser) Errors() []error {
	return p.errors
}

func (p *parser) Error(err error) {
	if p.tracer != nil {
		defer p.tracer.Un(p.tracer.Trace(trace.Here()))
		p.tracer.Message(err)
	}
	p.errors = append(p.errors, err)
}

func (p *parser) unexpectedTokenError(actual token.Type, description string, expected ...token.Type) {
	epos := p.file.Position(p.pos)
	err := &UnexpectedTokenError{
		Pos:            epos,
		ExpectedTokens: expected,
		ActualToken:    actual,
		Description:    description,
	}
	p.Error(err)
}

func (p *parser) noPrefixParseFnError(t token.Type) {
	msg := fmt.Sprintf("no prefix parse function for type %s found", t)
	epos := p.file.Position(p.pos)
	if epos.Filename != "" || epos.IsValid() {
		msg = epos.String() + ": " + msg
	}
	p.Error(errors.Errorf("%s", msg))
}

// ParseProgram returns the parsed program AST and all errors which occurred
// during the parse process. If the error is not nil the AST may be incomplete
// and callers should always check if they can handle the error with providing
// more input by checking with e.g. IsEOFError.
func (p *parser) ParseProgram() (*ast.Program, error) {
	program := &ast.Program{}
	program.Statements = []ast.Statement{}
	for !p.currentIs(token.EOF) {
		if p.currentIs(token.NEWLINE) {
			// Early exit
			p.nextToken()
			continue
		}
		// fmt.Println(p.curToken, p.errors)
		stmt := p.parseStatement()
		if stmt != nil {
			program.Statements = append(program.Statements, stmt)
		}
		p.nextToken()
	}
	// fmt.Println("Last token:", p.curToken)
	if len(p.errors) != 0 {
		return program, NewErrors("Parsing errors", p.errors...)
	}
	// fmt.Println("Program:", program)
	return program, nil
}

func (p *parser) parseStatement() ast.Statement {
	if p.tracer != nil {
		defer p.tracer.Un(p.tracer.Trace(trace.Here()))
	}
	switch p.curToken.Type {
	case token.ILLEGAL:
		msg := p.curToken.Literal
		epos := p.file.Position(p.pos)
		if epos.Filename != "" || epos.IsValid() {
			msg = epos.String() + ": " + msg
		}
		p.Error(fmt.Errorf("%s", msg))
		return nil
	case token.EOF:
		p.unexpectedTokenError(p.curToken.Type, "", token.NEWLINE)
		return nil
	case token.NEWLINE:
		return nil
	case token.RETURN:
		return p.parseReturnStatement()
	case token.COMMENT:
		return p.parseComment()
	case token.BREAK:
		return p.parseBreakStatement()
	default:
		return p.parseExpressionStatement()
	}
}

func (p *parser) parseReturnStatement() *ast.ReturnStatement {
	if p.tracer != nil {
		defer p.tracer.Un(p.tracer.Trace(trace.Here()))
	}
	stmt := &ast.ReturnStatement{}
	p.nextToken()

	if p.currentIs(token.NEWLINE, token.SEMICOLON) {
		p.nextToken()
		return stmt
	}

	stmt.ReturnValue = p.parseExpression(precLowest)
	if list, ok := stmt.ReturnValue.(ast.ExpressionList); ok {
		stmt.ReturnValue = &ast.ArrayLiteral{Elements: list}
	}

	if p.peekIs(token.NEWLINE, token.SEMICOLON) {
		p.nextToken()
		return stmt
	}
	if p.currentIs(token.NEWLINE, token.SEMICOLON) {
		return stmt
	}

	if !p.peekIs(token.COMMA) {
		p.unexpectedTokenError(p.peekToken.Type, "", token.COMMA)
		return nil
	}

	arr := &ast.ArrayLiteral{Elements: []ast.Expression{stmt.ReturnValue}}
	for p.peekIs(token.COMMA) {
		p.consume(token.COMMA)
		arr.Elements = append(arr.Elements, p.parseExpression(precLowest))
	}
	stmt.ReturnValue = arr

	if !p.accept(token.NEWLINE, token.SEMICOLON) {
		return nil
	}
	return stmt
}

func (p *parser) parseExpressionStatement() *ast.ExpressionStatement {
	if p.tracer != nil {
		defer p.tracer.Un(p.tracer.Trace(trace.Here()))
	}
	stmt := &ast.ExpressionStatement{}
	stmt.Expression = p.parseExpression(precLowest)
	if p.peekIs(token.SEMICOLON, token.NEWLINE) {
		p.nextToken()
	}
	return stmt
}

func (p *parser) parseExpression(precedence int) ast.Expression {
	if p.tracer != nil {
		defer p.tracer.Un(p.tracer.Trace(trace.Here()))
	}
	prefix := p.prefixParseFns[p.curToken.Type]
	if prefix == nil {
		p.noPrefixParseFnError(p.curToken.Type)
		return nil
	}
	leftExp := prefix()
	for precedence < precedenceForToken(p.peekToken.Type) {
		if leftExp == nil {
			return nil // fail early and stop parsing
		}
		if p.currentIs(token.NEWLINE, token.SEMICOLON) {
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
	if p.tracer != nil {
		defer p.tracer.Un(p.tracer.Trace(trace.Here()))
	}
	comment := &ast.Comment{}
	comment.Value = p.curToken.Literal
	if !p.peekIs(token.NEWLINE, token.EOF) {
		epos := p.file.Position(p.pos)
		p.Error(fmt.Errorf("%s: Expected newline or eof after comment", epos.String()))
		return nil
	}

	return comment
}

// LAMBDA          : "->" "(" CALL_ARGS ")" "{" COMPSTMT "}"
//                 | "->" "{" COMPSTMT "}";

const _LAMBDA_NAME_CHARSET = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

var _LAMBDA_NAME_RAND = rand.New(rand.NewSource(42))

func newAnonymousName(prefix string) string {
	const length = 8
	charset := []byte(_LAMBDA_NAME_CHARSET)
	_LAMBDA_NAME_RAND.Shuffle(len(charset), func(i, j int) {
		charset[i], charset[j] = charset[j], charset[i]
	})
	if !strings.HasPrefix(prefix, "__") {
		prefix = "__" + prefix
	}
	if !strings.HasSuffix(prefix, "_") {
		prefix = prefix + "_"
	}
	return prefix + string(charset[:length])
}

func (p *parser) parseLambdaLiteral() ast.Expression {
	if p.tracer != nil {
		defer p.tracer.Un(p.tracer.Trace(trace.Here()))
	}
	proc := &ast.FunctionLiteral{
		Name: newAnonymousName("lambda"),
	}
	if p.peekIs(token.LPAREN) {
		proc.Parameters = p.parseFunctionParameters(token.LPAREN, token.RPAREN)
	}
	if !p.accept(token.LBRACE) {
		return nil
	}
	proc.Body = p.parseBlockStatement(token.RBRACE)
	if proc.Body == nil {
		return nil
	}
	if !p.accept(token.RBRACE) {
		return nil
	}
	return proc
}

func (p *parser) parseSplat() ast.Expression {
	if p.tracer != nil {
		defer p.tracer.Un(p.tracer.Trace(trace.Here()))
	}
	splat := &ast.Splat{}
	// p.nextToken()
	// if p.curToken.Type != token.IDENT {
	// 	p.Error(p.curToken.Type, "", token.IDENT)
	// 	return nil
	// }
	p.nextToken()
	// TODO: what's supposed t be the prec here?
	// precLowest breaks for `x = [*y, 1]`
	expr := p.parseExpression(precSplat)
	// fmt.Println("Splat expression:", expr)
	// panic("TODO: splat expression")
	if expr == nil {
		return nil
	}
	splat.Value = expr
	return splat
}

func (p *parser) parseExpressions(left ast.Expression) ast.Expression {
	if p.tracer != nil {
		defer p.tracer.Un(p.tracer.Trace(trace.Here()))
	}
	p.nextToken()
	elements := []ast.Expression{left}
	next := p.parseExpression(precAssignment)
	elements = append(elements, next)
	for p.peekIs(token.COMMA) {
		p.consume(token.COMMA)
		next = p.parseExpression(precAssignment)
		elements = append(elements, next)
	}
	return ast.ExpressionList(elements)
}

func (p *parser) parseAssignmentOperator(left ast.Expression) ast.Expression {
	if p.tracer != nil {
		defer p.tracer.Un(p.tracer.Trace(trace.Here()))
	}
	assignIndex := strings.LastIndexByte(p.curToken.Literal, '=')
	if assignIndex < 0 {
		return nil
	}
	op := infix.InfixFromAssignmentOperator(p.curToken)
	if op == infix.ILLEGAL {
		epos := p.file.Position(p.pos)
		p.Error(fmt.Errorf("%s: invalid assignment operator %q", epos.String(), p.curToken.Literal))
		return nil
	}

	newInf := &ast.InfixExpression{
		Left:     left,
		Operator: op,
	}
	assign := &ast.Assignment{
		Left: left,
	}
	p.nextToken()
	newInf.Right = p.parseExpression(precLowest)
	assign.Right = newInf
	return assign
}

func (p *parser) parseAssignment(left ast.Expression) ast.Expression {
	if p.tracer != nil {
		defer p.tracer.Un(p.tracer.Trace(trace.Here()))
	}
	switch left.(type) {
	case *ast.Identifier:
	case *ast.IndexExpression:
	case ast.ExpressionList:
	default:
		p.unexpectedTokenError(p.curToken.Type, "", token.EOF)
		return nil
	}

	assign := &ast.Assignment{
		Left: left,
	}
	p.nextToken()
	expr := p.parseExpression(precLowest)
	right, ok := expr.(*ast.ConditionalExpression)
	if !ok {
		assign.Right = expr
		return assign
	}
	// rewrite to conditional assignment
	con, ok := right.Consequence.Statements[0].(*ast.ExpressionStatement)
	if !ok {
		p.Error(fmt.Errorf("malformed AST in assignment of the consequence"))
		return nil
	}

	var alt *ast.ExpressionStatement
	if right.Alternative != nil {
		alt, ok = right.Alternative.Statements[0].(*ast.ExpressionStatement)
		if !ok {
			p.Error(fmt.Errorf("malformed AST in assignment of the alternative"))
			return nil
		}
	} else {
		// alt = &ast.SymbolLiteral{Value: "nil"}
		alt = &ast.ExpressionStatement{
			Expression: &ast.SymbolLiteral{Value: "nil"},
		}
	}

	cond := &ast.ConditionalExpression{
		Unless:    right.Unless,
		Condition: right.Condition,
		Consequence: &ast.BlockStatement{
			Statements: []ast.Statement{
				&ast.ExpressionStatement{
					Expression: &ast.Assignment{
						Left:  left,
						Right: con.Expression,
					},
				},
			},
		},
		Alternative: &ast.BlockStatement{
			Statements: []ast.Statement{
				&ast.ExpressionStatement{
					Expression: &ast.Assignment{
						Left:  left,
						Right: alt.Expression,
					},
				},
			},
		},
	}

	return cond
}

func (p *parser) parseNilLiteral() ast.Expression {
	if p.tracer != nil {
		defer p.tracer.Un(p.tracer.Trace(trace.Here()))
	}
	return &ast.SymbolLiteral{Value: "nil"}
}

var integerLiteralReplacer = strings.NewReplacer("_", "")

func (p *parser) parseIntegerLiteral() ast.Expression {
	if p.tracer != nil {
		defer p.tracer.Un(p.tracer.Trace(trace.Here()))
	}
	// p.tracer.Message("hello sailor!")
	lit := &ast.IntegerLiteral{}
	value, err := strconv.ParseInt(integerLiteralReplacer.Replace(p.curToken.Literal), 0, 64)
	if err != nil {
		p.Error(fmt.Errorf("could not parse %q as integer", p.curToken.Literal))
		return nil
	}
	lit.Value = value
	return lit
}

func (p *parser) parseFloatLiteral() ast.Expression {
	if p.tracer != nil {
		defer p.tracer.Un(p.tracer.Trace(trace.Here()))
	}
	lit := &ast.FloatLiteral{}
	value, err := strconv.ParseFloat(p.curToken.Literal, 64)
	if err != nil {
		p.Error(fmt.Errorf("could not parse %q as float", p.curToken.Literal))
		return nil
	}
	lit.Value = value
	return lit

}
func (p *parser) parseStringLiteral() ast.Expression {
	if p.tracer != nil {
		defer p.tracer.Un(p.tracer.Trace(trace.Here()))
	}
	return &ast.StringLiteral{
		Value: p.curToken.Literal}
}

func (p *parser) parseSymbolLiteral() ast.Expression {
	if p.tracer != nil {
		defer p.tracer.Un(p.tracer.Trace(trace.Here()))
	}
	return &ast.SymbolLiteral{
		Value: strings.TrimPrefix(p.curToken.Literal, ":"),
	}
}

func (p *parser) parseArrayLiteral() ast.Expression {
	if p.tracer != nil {
		defer p.tracer.Un(p.tracer.Trace(trace.Here()))
	}
	array := &ast.ArrayLiteral{}

	p.nextToken()
	array.Elements = p.parseExpressionList(token.RBRACKET)
	return array
}

func (p *parser) parseBoolean() ast.Expression {
	if p.tracer != nil {
		defer p.tracer.Un(p.tracer.Trace(trace.Here()))
	}
	if p.currentIs(token.FALSE) {
		return &ast.SymbolLiteral{Value: "false"}
	} else if p.currentIs(token.TRUE) {
		return &ast.SymbolLiteral{Value: "true"}
	} else {
		p.unexpectedTokenError(p.curToken.Type, "", token.TRUE, token.FALSE)
		return nil
	}
}

func (p *parser) consumeNewlineOrComment() {
	for {
		if p.currentIs(token.NEWLINE) {
			// consume the newline
			p.nextToken()
		} else if p.currentIs(token.COMMENT) {
			// consume the comment
			p.nextToken()
			if p.currentIs(token.STRING) {
				p.nextToken()
			}
		} else {
			break
		}
	}
}
func (p *parser) parseHash() ast.Expression {
	if p.tracer != nil {
		defer p.tracer.Un(p.tracer.Trace(trace.Here()))
	}
	hash := &ast.HashLiteral{Map: make(map[ast.Expression]ast.Expression)}
	p.nextToken()

	p.consumeNewlineOrComment()

	if p.currentIs(token.RBRACE) {
		return hash
	}

	// parse the first key-value pair
	k, v, ok := p.parseKeyValue()
	if !ok {
		return nil
	}
	hash.Map[k] = v
	p.nextToken() // move past the end of the key-value pair
	p.consumeNewlineOrComment()

	for p.currentIs(token.COMMA) {
		p.nextToken()
		p.consumeNewlineOrComment()
		if p.currentIs(token.RBRACE) {
			break
		}
		k, v, ok := p.parseKeyValue()
		if !ok {
			return nil
		}
		hash.Map[k] = v
		p.nextToken() // move past the end of the key-value pair
		p.consumeNewlineOrComment()
	}
	if p.currentIs(token.RBRACE) {
		// we've landed on a RBRACE
		// this is fine, no need to do anything
	} else if p.peekIs(token.RBRACE) {
		// we've landed on the end of the last key-value pair (hopefully)
		// we need to accept the RBRACE
		if !p.accept(token.RBRACE) {
			return nil
		}
	}
	return hash
}

func (p *parser) parseKeyValue() (ast.Expression, ast.Expression, bool) {
	if p.tracer != nil {
		defer p.tracer.Un(p.tracer.Trace(trace.Here()))
	}
	key := p.parseExpression(precAssignment)
	if !p.consume(token.HASHROCKET) {
		return nil, nil, false
	}
	val := p.parseExpression(precAssignment)
	return key, val, true
}

func (p *parser) parseBlock() *ast.FunctionLiteral {
	if p.tracer != nil {
		defer p.tracer.Un(p.tracer.Trace(trace.Here()))
	}
	name := newAnonymousName("block")
	if p.tracer != nil {
		p.tracer.Message(name)
	}
	block := &ast.FunctionLiteral{
		Name: name,
	}
	if p.peekIs(token.PIPE) {
		block.Parameters = p.parseFunctionParameters(token.PIPE, token.PIPE)
	}

	if p.peekIs(token.NEWLINE, token.SEMICOLON) {
		p.accept(token.NEWLINE, token.SEMICOLON)
	}

	endToken := token.RBRACE

	block.Body = p.parseBlockStatement(endToken)
	p.nextToken()
	return block
}

func (p *parser) parsePrefixExpression() ast.Expression {
	if p.tracer != nil {
		defer p.tracer.Un(p.tracer.Trace(trace.Here()))
	}
	expression := &ast.PrefixExpression{
		Operator: p.curToken.Literal,
	}
	p.nextToken()
	expression.Right = p.parseExpression(precPrefix)
	return expression
}

func (p *parser) parseInfixExpression(left ast.Expression) ast.Expression {
	if p.tracer != nil {
		defer p.tracer.Un(p.tracer.Trace(trace.Here()))
	}

	op := infix.InfixFromToken(p.curToken)
	if op == infix.ILLEGAL {
		p.Error(fmt.Errorf("illegal infix operator %q", p.curToken.Literal))
		return nil
	}

	expression := &ast.InfixExpression{
		Operator: op,
		Left:     left,
	}
	precedence := precedenceForToken(p.curToken.Type)
	p.nextToken()
	expression.Right = p.parseExpression(precedence)
	return expression
}

func (p *parser) parseRangeLiteral(left ast.Expression) ast.Expression {
	if p.tracer != nil {
		defer p.tracer.Un(p.tracer.Trace(trace.Here()))
	}
	expression := &ast.RangeLiteral{
		Left:      left,
		Inclusive: p.curToken.Type == token.DDOT,
	}
	precedence := precedenceForToken(p.curToken.Type)
	p.nextToken()
	expression.Right = p.parseExpression(precedence)
	return expression
}

func (p *parser) parseIndexExpression(left ast.Expression) ast.Expression {
	if p.tracer != nil {
		defer p.tracer.Un(p.tracer.Trace(trace.Here()))
	}
	exp := &ast.IndexExpression{Left: left}

	p.nextToken()
	content := p.parseExpression(precLowest)
	exp.Index = content

	// if elist, ok := exp.Index.(ast.ExpressionList); ok {
	// 	// fmt.Println("Got an ExpressionList for the IndexExpression:", elist)
	// 	exp.Index = elist
	// } else if splat, ok := exp.Index.(*ast.Splat); ok {
	// 	// Indexing with a splat, e.g. array[*a]
	// 	exp.Index = splat
	// }

	if !p.accept(token.RBRACKET) {
		return nil
	}
	// fmt.Println("IndexExpression:", exp)
	return exp
}

func (p *parser) parseGroupedExpression() ast.Expression {
	if p.tracer != nil {
		defer p.tracer.Un(p.tracer.Trace(trace.Here()))
	}
	p.nextToken()
	exp := p.parseExpression(precLowest)
	if !p.accept(token.RPAREN) {
		return nil
	}
	return exp
}

func (p *parser) parseIfExpression() ast.Expression {
	if p.tracer != nil {
		defer p.tracer.Un(p.tracer.Trace(trace.Here()))
	}
	expression := &ast.ConditionalExpression{Unless: p.currentIs(token.UNLESS)}
	p.nextToken()
	expression.Condition = p.parseExpression(precLowest)
	if p.peekIs(token.THEN) {
		p.accept(token.THEN)
	}

	// there may be a comment here. gobble it up
	if p.peekIs(token.COMMENT) {
		p.accept(token.COMMENT)
	}

	if !p.peekIs(token.NEWLINE, token.SEMICOLON) {
		p.unexpectedTokenError(p.peekToken.Type, "could not parse if expression", token.NEWLINE, token.SEMICOLON)
		return nil
	}
	p.accept(token.NEWLINE, token.SEMICOLON)

	expression.Consequence = p.parseBlockStatement(token.ELSE, token.ELSIF)
	parsed_elsif := false
	if p.peekIs(token.ELSE) {
		p.accept(token.ELSE)
		p.accept(token.NEWLINE)
		expression.Alternative = p.parseBlockStatement() // parse until the END
	} else if p.peekIs(token.ELSIF) {
		// start parsing a new if expression from here
		p.accept(token.ELSIF)
		expression.Alternative = &ast.BlockStatement{
			Statements: []ast.Statement{
				&ast.ExpressionStatement{
					Expression: p.parseIfExpression().(*ast.ConditionalExpression),
				},
			},
		}
		// back up one token
		parsed_elsif = true
	}
	if parsed_elsif {
		// don't expect the END, since it has been gobbled up by the innermost
		// if expression. check that it's there, though.
		if !p.currentIs(token.END) {
			p.unexpectedTokenError(p.curToken.Type, "expected END after elsif", token.END)
		}
	} else {
		p.accept(token.END)
	}
	return expression
}

func (p *parser) parseTernaryIfExpression(condition ast.Expression) ast.Expression {
	if p.tracer != nil {
		defer p.tracer.Un(p.tracer.Trace(trace.Here()))
	}
	expression := &ast.ConditionalExpression{Unless: p.currentIs(token.UNLESS)}
	p.nextToken()
	expression.Condition = condition
	expression.Consequence = &ast.BlockStatement{
		Statements: []ast.Statement{
			&ast.ExpressionStatement{
				Expression: p.parseExpression(precTernary),
			},
		},
	}
	p.consume(token.COLON)
	expression.Alternative = &ast.BlockStatement{
		Statements: []ast.Statement{
			&ast.ExpressionStatement{
				Expression: p.parseExpression(precTernary),
			},
		},
	}
	return expression
}

func (p *parser) parseModifierConditionalExpression(left ast.Expression) ast.Expression {
	if p.tracer != nil {
		defer p.tracer.Un(p.tracer.Trace(trace.Here()))
	}
	expression := &ast.ConditionalExpression{Unless: p.currentIs(token.UNLESS)}
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
	if p.tracer != nil {
		defer p.tracer.Un(p.tracer.Trace(trace.Here()))
	}
	loop := &ast.LoopExpression{}
	if p.curToken.Type == token.LOOP {
		if p.peekIs(token.LBRACE) {
			p.accept(token.LBRACE)
			loop.Block = p.parseBlockStatement(token.RBRACE)
			p.nextToken()
		}
	} else {
		p.unexpectedTokenError(p.curToken.Type, "unexpected loop start", token.LOOP)
		return nil
	}

	return loop
}

func (p *parser) parseBreakStatement() *ast.BreakStatement {
	if p.tracer != nil {
		defer p.tracer.Un(p.tracer.Trace(trace.Here()))
	}
	stmt := &ast.BreakStatement{}
	if p.peekIs(token.IF) {
		p.accept(token.IF)
		p.nextToken()
		stmt.Condition = p.parseExpression(precLowest)
	} else if p.peekIs(token.UNLESS) {
		p.accept(token.UNLESS)
		p.nextToken()
		stmt.Condition = p.parseExpression(precLowest)
		stmt.Unless = true
	}
	return stmt
}

func (p *parser) parseFunctionLiteral() ast.Expression {
	if p.tracer != nil {
		defer p.tracer.Un(p.tracer.Trace(trace.Here()))
	}
	fl := &ast.FunctionLiteral{}

	if !p.peekIs(token.IDENT) && !p.peekToken.Type.IsOperator() {
		p.unexpectedTokenError(p.peekToken.Type, "", token.IDENT)
		return nil
	}

	if p.peekIs(token.IDENT) {
		p.accept(token.IDENT)
	} else {
		p.nextToken()
	}
	fl.Name = p.curToken.Literal

	fl.Parameters = p.parseFunctionParameters(token.LPAREN, token.RPAREN)

	if !p.accept(token.NEWLINE, token.SEMICOLON) {
		return nil
	}

	fl.Body = p.parseBlockStatement(token.END)
	if !p.accept(token.END) {
		return nil
	}

	// Check for dynamic constant assignment
	inspect := func(n ast.Node) {
		// if _, ok := n.(*ast.Splat); ok {
		// 	// skip walking the splat since we don't want to evaluate it
		// 	return false
		// }
		x, ok := n.(*ast.Assignment)
		if ok {
			switch left := x.Left.(type) {
			case *ast.Identifier:
				if left.IsConstant() {
					p.Error(fmt.Errorf("dynamic constant assignment"))
				}
			case ast.ExpressionList:
				for _, expr := range left {
					if ident, ok := expr.(*ast.Identifier); ok {
						if ident.IsConstant() {
							p.Error(fmt.Errorf("dynamic constant assignment"))
						}
					}
				}
			}
		}
		// return true
	}
	ast.Inspect(fl.Body, inspect)

	return fl
}

func (p *parser) parseFunctionParameters(startToken, endToken token.Type) []*ast.FunctionParameter {
	if p.tracer != nil {
		defer p.tracer.Un(p.tracer.Trace(trace.Here()))
	}
	hasDelimiters := false
	if p.peekIs(startToken) {
		hasDelimiters = true
		p.accept(startToken)
	}

	identifiers := []*ast.FunctionParameter{}

	if !hasDelimiters && p.peekIs(endToken) {
		p.unexpectedTokenError(p.peekToken.Type, "", token.NEWLINE, token.SEMICOLON)
		return nil
	}

	if hasDelimiters && p.peekIs(endToken) {
		p.accept(endToken)
		return identifiers
	}

	if !hasDelimiters && p.peekIs(token.NEWLINE, token.SEMICOLON) {
		return identifiers
	}

	got_splat := false
	if p.peekIs(token.ASTERISK) {
		got_splat = true
		p.accept(token.ASTERISK)
	}
	p.accept(token.IDENT)

	ident := &ast.FunctionParameter{Name: p.curToken.Literal}
	if got_splat {
		ident.Splat = true
		got_splat = false
	}
	if p.peekIs(token.ASSIGN) {
		p.consume(token.ASSIGN)
		ident.Default = p.parseExpression(precAssignment)
	}
	identifiers = append(identifiers, ident)

	for p.peekIs(token.COMMA) {
		p.accept(token.COMMA)
		if p.peekIs(token.ASTERISK) {
			got_splat = true
			p.accept(token.ASTERISK)
		}
		p.accept(token.IDENT)
		ident := &ast.FunctionParameter{Name: p.curToken.Literal}
		if got_splat {
			ident.Splat = true
			got_splat = false
		}
		if p.peekIs(token.ASSIGN) {
			p.consume(token.ASSIGN)
			ident.Default = p.parseExpression(precPrefix)
		}
		identifiers = append(identifiers, ident)
	}

	if !hasDelimiters && p.peekIs(endToken) {
		p.unexpectedTokenError(p.peekToken.Type, "no delimiters but end delimiter found", endToken)
		return nil
	}

	if hasDelimiters && p.peekIs(endToken) {
		p.accept(endToken)
	}

	return identifiers
}

func (p *parser) parseBlockStatement(t ...token.Type) *ast.BlockStatement {
	if p.tracer != nil {
		defer p.tracer.Un(p.tracer.Trace(trace.Here()))
	}
	terminatorTokens := append(
		[]token.Type{
			token.END,
		},
		t...,
	)
	block := &ast.BlockStatement{}
	block.Statements = []ast.Statement{}

	for !p.peekIs(terminatorTokens...) {
		if p.peekIs(token.EOF) {
			p.unexpectedTokenError(p.peekToken.Type, "", token.EOF)
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
	if p.tracer != nil {
		defer p.tracer.Un(p.tracer.Trace(trace.Here()))
	}
	contextCallExpression := &ast.ContextCallExpression{Context: context}

	p.nextToken()

	if !p.currentIs(token.IDENT) && !p.curToken.Type.IsOperator() {
		p.unexpectedTokenError(p.curToken.Type, "", token.IDENT)
		return nil
	}

	name := p.curToken.Literal
	if p.tracer != nil {
		p.tracer.Message(name)
	}
	contextCallExpression.Function = name

	if p.peekIs(token.SEMICOLON, token.NEWLINE, token.EOF, token.DOT, token.RPAREN, token.QMARK) {
		contextCallExpression.Arguments = []ast.Expression{}
		return contextCallExpression
	}

	if p.peekIs(token.LPAREN) {
		p.accept(token.LPAREN)
		p.nextToken()
		contextCallExpression.Arguments = p.parseExpressionList(token.RPAREN)
		if p.peekIs(token.LBRACE) {
			p.accept(token.LBRACE)
			contextCallExpression.Block = p.parseBlock()
		}
		return contextCallExpression
	}

	if p.peekIs(append(tokensNotPossibleInCallArgs, token.RBRACE)...) {
		return contextCallExpression
	}

	p.nextToken()

	contextCallExpression.Arguments = p.parseCallArguments(token.LBRACE)
	if p.currentIs(token.LBRACE) {
		contextCallExpression.Block = p.parseBlock()
	}
	return contextCallExpression
}

func (p *parser) debugMessageState() {
	if p.tracer != nil {
		p.tracer.Message(fmt.Sprintf("Current token: %v", p.curToken))
		p.tracer.Message(fmt.Sprintf("Peek token: %v", p.peekToken))
	}
}
func (p *parser) parseContextCallExpression(context ast.Expression) ast.Expression {
	if p.tracer != nil {
		defer p.tracer.Un(p.tracer.Trace(trace.Here()))
	}
	contextCallExpression := &ast.ContextCallExpression{Context: context}
	if p.currentIs(token.DOT) {
		p.nextToken()
	}

	if !p.currentIs(token.IDENT) {
		p.unexpectedTokenError(p.curToken.Type, "", token.IDENT)
		return nil
	}

	// ident := p.parseIdentifier().(*ast.Identifier)
	contextCallExpression.Function = p.curToken.Literal

	if p.peekIs(token.SEMICOLON, token.NEWLINE, token.DOT) {
		contextCallExpression.Arguments = []ast.Expression{}
		return contextCallExpression
	}

	if p.peekIs(token.LPAREN) {
		p.accept(token.LPAREN)
		p.nextToken()
		contextCallExpression.Arguments = p.parseExpressionList(token.RPAREN)
		if p.peekIs(token.LBRACE) {
			p.accept(token.LBRACE)
			contextCallExpression.Block = p.parseBlock()
		}
		return contextCallExpression
	}

	if p.peekIs(append(tokensNotPossibleInCallArgs, token.RBRACE)...) {
		return contextCallExpression
	}

	p.nextToken()
	contextCallExpression.Arguments = p.parseCallArguments(token.LBRACE)
	if p.currentIs(token.LBRACE) {
		contextCallExpression.Block = p.parseBlock()
	}
	return contextCallExpression
}

func (p *parser) parseCallArgument(function ast.Expression) ast.Expression {
	if p.tracer != nil {
		defer p.tracer.Un(p.tracer.Trace(trace.Here()))
	}
	ident, ok := function.(*ast.Identifier)
	if !ok {
		// method call on any other object
		return p.parseContextCallExpression(function)
	}
	exp := &ast.ContextCallExpression{Function: ident.Value}
	if p.currentIs(token.LBRACE) {
		exp.Block = p.parseBlock()
		return exp
	}

	exp.Arguments = p.parseExpressionList(token.SEMICOLON, token.NEWLINE)
	if p.peekIs(token.LBRACE) {
		p.accept(token.LBRACE)
		exp.Block = p.parseBlock()
	}
	return exp
}

func (p *parser) parseCallBlock(function ast.Expression) ast.Expression {
	if p.tracer != nil {
		defer p.tracer.Un(p.tracer.Trace(trace.Here()))
	}

	exp := &ast.ContextCallExpression{}
	exp.Block = p.parseBlock()
	switch fn := function.(type) {
	case *ast.Identifier:
		exp.Function = fn.Value
		return exp
	case *ast.InfixExpression:
		ident, ok := fn.Right.(*ast.Identifier)
		if !ok {
			break
		}
		exp.Function = ident.Value
		fn.Right = exp
		return fn
	}
	p.Error(fmt.Errorf("could not parse call expression: expected identifier, got token '%T'", function))
	return nil
}

// func (p *parser) parseCallExpression(function ast.Expression) ast.Expression {

func (p *parser) parseCallExpressionWithParens(function ast.Expression) ast.Expression {
	if p.tracer != nil {
		defer p.tracer.Un(p.tracer.Trace(trace.Here()))
	}
	ident, ok := function.(*ast.Identifier)
	if !ok {
		p.Error(fmt.Errorf("could not parse call expression: expected identifier, got token '%T'", function))
		return nil
	}
	exp := &ast.ContextCallExpression{Function: ident.Value}
	p.nextToken()
	exp.Arguments = p.parseExpressionList(token.RPAREN)
	if p.peekIs(token.LBRACE) {
		p.accept(token.LBRACE)
		exp.Block = p.parseBlock()
	}
	return exp
}

func (p *parser) parseCallArguments(end ...token.Type) []ast.Expression {
	if p.tracer != nil {
		defer p.tracer.Un(p.tracer.Trace(trace.Here()))
	}
	list := []ast.Expression{}
	if p.currentIs(end...) {
		return list
	}

	list = append(list, p.parseExpression(precAssignment))

	for p.peekIs(token.COMMA) {
		p.consume(token.COMMA)
		list = append(list, p.parseExpression(precAssignment))
	}

	if p.peekIs(end...) {
		p.accept(end...)
	}

	return list
}

func (p *parser) parseExpressionList(end ...token.Type) []ast.Expression {
	if p.tracer != nil {
		defer p.tracer.Un(p.tracer.Trace(trace.Here()))
	}
	list := []ast.Expression{}
	if p.currentIs(end...) {
		return list
	}

	next := p.parseExpression(precIfUnless)
	if elist, ok := next.(ast.ExpressionList); ok {
		if p.peekIs(end...) {
			p.accept(end...)
		}
		return elist
	}
	list = append(list, next)

	if p.peekIs(end...) {
		p.accept(end...)
	}

	return list
}

func precedenceForToken(t token.Type) int {
	if prec, ok := precedences[t]; ok {
		return prec
	}
	return precLowest
}

func (p *parser) currentIs(types ...token.Type) bool {
	for _, typ := range types {
		if p.curToken.Type == typ {
			return true
		}
	}
	return false
}

func (p *parser) peekIs(types ...token.Type) bool {
	for _, typ := range types {
		if p.peekToken.Type == typ {
			return true
		}
	}
	return false
}

// accept moves to the next Token
// if it's from the valid set.
func (p *parser) accept(t ...token.Type) bool {
	if p.peekIs(t...) {
		p.nextToken()
		return true
	}

	p.unexpectedTokenError(p.peekToken.Type, "", t...)
	return false
}

// consume consumes the next token
// if it's from the valid set.
func (p *parser) consume(t ...token.Type) bool {
	isRightToken := p.accept(t...)
	if isRightToken {
		p.nextToken()
	}
	return isRightToken
}
