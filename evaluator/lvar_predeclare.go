package evaluator

import (
	"github.com/lczyk/goruby/ast"
	"github.com/lczyk/goruby/object"
)

// predeclareTopLevelLocals walks the program's top-level statements and
// pre-binds every identifier that appears on the left of an assignment
// to nil, unless it's already bound. This mirrors ruby's parser-time
// "lvar introduction" semantics, where a local variable is considered
// defined for the rest of its scope as soon as the parser sees an
// assignment to it -- even if the assignment sits inside a branch that
// never runs. Without this pre-pass, code like
//
//	if cond
//	    x = 1
//	end
//	puts x
//
// raises NameError when cond is false. Real ruby prints "nil".
//
// Scope boundaries match ruby's:
//   - if / unless / case / while / until / begin..end / parens share scope
//     with the enclosing program / method, so descend into them.
//   - def / class / module / lambda / proc / do..end-block bodies are
//     their own scope -- don't descend (their locals shouldn't bleed up,
//     and the introduction semantics apply within each scope on its own
//     entry; for blocks the rule is more nuanced but the conservative
//     skip is safe and avoids accidental binding leaks).
func predeclareTopLevelLocals(env *object.Environment, stmts []ast.Statement) {
	for _, s := range stmts {
		predeclareNode(env, s)
	}
}

func predeclareNode(env *object.Environment, n ast.Node) {
	switch v := n.(type) {
	case nil:
		return
	case *ast.Assignment:
		introduce(env, v.Left)
		predeclareNode(env, v.Right)
	case *ast.MultiAssignment:
		for _, t := range v.Variables {
			introduce(env, t)
		}
		for _, e := range v.Values {
			predeclareNode(env, e)
		}
	case *ast.ExpressionStatement:
		predeclareNode(env, v.Expression)
	case *ast.ConditionalExpression:
		predeclareNode(env, v.Condition)
		if v.Consequence != nil {
			for _, s := range v.Consequence.Statements {
				predeclareNode(env, s)
			}
		}
		if v.Alternative != nil {
			for _, s := range v.Alternative.Statements {
				predeclareNode(env, s)
			}
		}
	case *ast.CaseExpression:
		predeclareNode(env, v.Condition)
		for _, w := range v.WhenClauses {
			if w == nil || w.Body == nil {
				continue
			}
			for _, b := range w.Body.Statements {
				predeclareNode(env, b)
			}
		}
		if v.ElseBody != nil {
			for _, s := range v.ElseBody.Statements {
				predeclareNode(env, s)
			}
		}
	case *ast.LoopExpression:
		predeclareNode(env, v.Condition)
		if v.Block != nil {
			for _, s := range v.Block.Statements {
				predeclareNode(env, s)
			}
		}
	case *ast.ExceptionHandlingBlock:
		if v.TryBody != nil {
			for _, s := range v.TryBody.Statements {
				predeclareNode(env, s)
			}
		}
		for _, r := range v.Rescues {
			if r == nil || r.Body == nil {
				continue
			}
			for _, s := range r.Body.Statements {
				predeclareNode(env, s)
			}
		}
		if v.EnsureBody != nil {
			for _, s := range v.EnsureBody.Statements {
				predeclareNode(env, s)
			}
		}
	case *ast.ParenExpression:
		predeclareNode(env, v.Expr)
		for _, s := range v.Stmts {
			predeclareNode(env, s)
		}
	case *ast.BlockStatement:
		for _, s := range v.Statements {
			predeclareNode(env, s)
		}
	case *ast.InfixExpression:
		predeclareNode(env, v.Left)
		predeclareNode(env, v.Right)
	case *ast.PrefixExpression:
		predeclareNode(env, v.Right)
	case ast.ExpressionList:
		for _, e := range v {
			predeclareNode(env, e)
		}
		// def / class / module / function literals are their own scope --
		// don't descend.
	}
}

// introduce binds name=nil in the local scope if no binding (in this or
// any outer scope) yet exists. Existing bindings stay untouched so we
// don't clobber a value the caller already produced.
func introduce(env *object.Environment, target ast.Expression) {
	id, ok := target.(*ast.Identifier)
	if !ok {
		return
	}
	if isConstantName(id.Value) {
		return
	}
	if _, ok := env.Get(id.Value); ok {
		return
	}
	env.Set(id.Value, object.NIL)
}
