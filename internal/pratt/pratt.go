// Package pratt provides a generic Pratt expression-parsing core.
//
// It is intentionally minimal: a precedence-climbing loop over caller-owned
// prefix/infix dispatch tables, parameterised on the caller's parser type P
// and expression node type N. All token access (peek, advance, precedence) is
// delegated to caller-supplied functions on P.
//
// The package imports token.Type to size the dispatch tables as fixed arrays
// (one entry per token type) for zero-overhead dispatch. It is internal to
// goruby and not intended for external use.
package pratt

import "github.com/lczyk/goruby/token"

// TableSize is the length of the prefix/infix dispatch arrays.
const TableSize = token.TypeMax + 1

// PrefixFn parses an expression starting at the current token.
type PrefixFn[P any, N any] func(*P) N

// InfixFn parses an expression that extends left, starting at the current
// token (the operator). The caller has already advanced curToken to the
// operator before InfixFn is called.
type InfixFn[P any, N any] func(*P, N) N

// HookResult is what an iteration Hook returns to direct the climb loop.
type HookResult[N any] struct {
	// Replace, if Handled is true, becomes the new left expression. The hook
	// is responsible for any token advancement it performed.
	Replace N
	// Handled means the hook consumed this iteration: skip default infix
	// dispatch and continue the loop with Replace as the new left.
	Handled bool
	// Stop ends the climb loop immediately, returning the current left.
	Stop bool
}

// Hook runs each loop iteration after the precedence check passes, before
// default infix dispatch. Use it to express grammar quirks that interleave
// with precedence climbing (e.g. context-sensitive terminators, ambiguous
// juxtaposition rules) without bloating the core.
//
// A nil Hook is treated as a no-op.
type Hook[P any, N any] func(p *P, left N) HookResult[N]

// Config bundles dispatch tables and parser-access callbacks for Climb.
// One Config is built per parser (usually at package init) and reused for
// every Climb call.
type Config[P any, N any] struct {
	Prefix [TableSize]PrefixFn[P, N]
	Infix  [TableSize]InfixFn[P, N]

	// CurType returns the type of the current token.
	CurType func(*P) token.Type
	// Advance moves curToken to peekToken (and refills peekToken).
	Advance func(*P)
	// PeekPrec returns the precedence of the peek token.
	PeekPrec func(*P) int

	// OnNoPrefix is called when no prefix fn is registered for the current
	// token. The caller typically records a parse error.
	OnNoPrefix func(p *P, t token.Type)

	// PeekType returns the type of the peek token. Used to look up the next
	// infix fn after the precedence check.
	PeekType func(*P) token.Type

	// Hook is an optional per-iteration callback. Nil means no hook.
	Hook Hook[P, N]
}

// Climb runs the precedence-climbing loop with minimum binding power
// minPrec. It returns the parsed expression, or the zero value of N if no
// prefix fn is registered for the current token.
//
// The loop body:
//  1. dispatch prefix fn for current token -> left
//  2. while minPrec < peek precedence:
//     a. run Hook (if set); honour Stop / Handled
//     b. look up infix fn for peek; if none, return left
//     c. advance, dispatch infix(left), update left
//  3. return left
func Climb[P any, N any](p *P, c *Config[P, N], minPrec int) N {
	var zero N
	prefix := c.Prefix[c.CurType(p)]
	if prefix == nil {
		if c.OnNoPrefix != nil {
			c.OnNoPrefix(p, c.CurType(p))
		}
		return zero
	}
	left := prefix(p)
	for minPrec < c.PeekPrec(p) {
		if c.Hook != nil {
			r := c.Hook(p, left)
			if r.Stop {
				return left
			}
			if r.Handled {
				left = r.Replace
				continue
			}
		}
		infix := c.Infix[c.PeekType(p)]
		if infix == nil {
			return left
		}
		c.Advance(p)
		left = infix(p, left)
	}
	return left
}
