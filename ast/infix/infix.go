package infix

import "github.com/lczyk/goruby/token"

type Infix int

const (
	ILLEGAL    Infix = Infix(token.ILLEGAL)
	PLUS             = Infix(token.PLUS)
	MINUS            = Infix(token.MINUS)
	SLASH            = Infix(token.SLASH)
	ASTERISK         = Infix(token.ASTERISK)
	POW              = Infix(token.POW)
	MODULO           = Infix(token.MODULO)
	AND              = Infix(token.AND)
	PIPE             = Infix(token.PIPE)
	EQ               = Infix(token.EQ)
	NOTEQ            = Infix(token.NOTEQ)
	LT               = Infix(token.LT)
	GT               = Infix(token.GT)
	LTE              = Infix(token.LTE)
	GTE              = Infix(token.GTE)
	LOGICALOR        = Infix(token.LOGICALOR)
	LOGICALAND       = Infix(token.LOGICALAND)
	SPACESHIP        = Infix(token.SPACESHIP)
	LSHIFT           = Infix(token.LSHIFT)
)

// var infix_strings = map[Infix]string{
// 	ILLEGAL:    "ILLEGAL",
// 	PLUS:       "PLUS",
// 	MINUS:      "MINUS",
// 	SLASH:      "SLASH",
// 	ASTERISK:   "ASTERISK",
// 	POW:        "POW",
// 	MODULO:     "MODULO",
// 	AND:        "AND",
// 	PIPE:       "PIPE",
// 	EQ:         "EQ",
// 	NOTEQ:      "NOTEQ",
// 	LT:         "LT",
// 	GT:         "GT",
// 	LTE:        "LTE",
// 	GTE:        "GTE",
// 	LOGICALOR:  "LOGICALOR",
// 	LOGICALAND: "LOGICALAND",
// 	SPACESHIP:  "SPACESHIP",
// 	LSHIFT:     "LSHIFT",
// }

var infix_repr = map[Infix]string{
	ILLEGAL:    "ILLEGAL",
	PLUS:       "+",
	MINUS:      "-",
	SLASH:      "/",
	ASTERISK:   "*",
	POW:        "**",
	MODULO:     "%",
	AND:        "&&",
	PIPE:       "||",
	EQ:         "==",
	NOTEQ:      "!=",
	LT:         "<",
	GT:         ">",
	LTE:        "<=",
	GTE:        ">=",
	LOGICALOR:  "||",
	LOGICALAND: "&&",
	SPACESHIP:  "<=>",
	LSHIFT:     "<<",
}

func (i Infix) String() string {
	if s, ok := infix_repr[i]; ok {
		return s
	}
	return infix_repr[ILLEGAL]
}

func InfixFromTokenType(t token.Type) Infix {
	switch t {
	case token.PLUS:
		return PLUS
	case token.MINUS:
		return MINUS
	case token.SLASH:
		return SLASH
	case token.ASTERISK:
		return ASTERISK
	case token.POW:
		return POW
	case token.MODULO:
		return MODULO
	case token.AND:
		return AND
	case token.PIPE:
		return PIPE
	case token.EQ:
		return EQ
	case token.NOTEQ:
		return NOTEQ
	case token.LT:
		return LT
	case token.GT:
		return GT
	case token.LTE:
		return LTE
	case token.GTE:
		return GTE
	case token.LOGICALOR:
		return LOGICALOR
	case token.LOGICALAND:
		return LOGICALAND
	case token.SPACESHIP:
		return SPACESHIP
	case token.LSHIFT:
		return LSHIFT
	default:
		return ILLEGAL
	}
}

func InfixFromToken(t token.Token) Infix {
	return InfixFromTokenType(t.Type)
}

// p.registerInfix(token.ADDASSIGN, p.parseAssignmentOperator)
// p.registerInfix(token.SUBASSIGN, p.parseAssignmentOperator)
// p.registerInfix(token.MULASSIGN, p.parseAssignmentOperator)
// p.registerInfix(token.DIVASSIGN, p.parseAssignmentOperator)
// p.registerInfix(token.MODASSIGN, p.parseAssignmentOperator)

func InfixFromAssignmentOperatorType(t token.Type) Infix {
	switch t {
	case token.ADDASSIGN:
		return PLUS
	case token.SUBASSIGN:
		return MINUS
	case token.MULASSIGN:
		return ASTERISK
	case token.DIVASSIGN:
		return SLASH
	case token.MODASSIGN:
		return MODULO
	default:
		return ILLEGAL
	}
}

func InfixFromAssignmentOperator(t token.Token) Infix {
	return InfixFromAssignmentOperatorType(t.Type)
}
