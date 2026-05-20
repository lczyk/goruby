package evaluator

import (
	"github.com/lczyk/goruby/ast"
	"github.com/lczyk/goruby/object"
)

// evalBlockStatement evaluates a sequence of statements and returns the
// last value. An empty / nil block evaluates to nil.
func evalBlockStatement(env *object.Environment, b *ast.BlockStatement) (object.RubyObject, error) {
	if b == nil || len(b.Statements) == 0 {
		return object.NIL, nil
	}
	var result object.RubyObject = object.NIL
	for _, s := range b.Statements {
		v, err := Eval(s, env)
		if err != nil {
			return nil, err
		}
		result = v
	}
	return result, nil
}

func evalConditional(env *object.Environment, n *ast.ConditionalExpression) (object.RubyObject, error) {
	cond, err := Eval(n.Condition, env)
	if err != nil {
		return nil, err
	}
	t := truthy(cond)
	if n.IsNegated() {
		t = !t
	}
	if t {
		return evalBlockStatement(env, n.Consequence)
	}
	if n.Alternative != nil {
		return evalBlockStatement(env, n.Alternative)
	}
	return object.NIL, nil
}

func evalLoop(env *object.Environment, n *ast.LoopExpression) (object.RubyObject, error) {
	if infix, ok := n.Condition.(*ast.InfixExpression); ok && infix.Operator == "in" {
		return evalForIn(env, infix, n.Block)
	}
	negate := n.Token.Type.Literal() == "until"

	check := func() (bool, error) {
		c, err := Eval(n.Condition, env)
		if err != nil {
			return false, err
		}
		t := truthy(c)
		if negate {
			t = !t
		}
		return t, nil
	}

	bodyStep := func() (stop bool, breakValue object.RubyObject, err error) {
		_, berr := evalBlockStatement(env, n.Block)
		if berr == nil {
			return false, nil, nil
		}
		if _, ok := berr.(*nextSignal); ok {
			return false, nil, nil
		}
		if bs, ok := berr.(*breakSignal); ok {
			return true, bs.Value, nil
		}
		return false, nil, berr
	}

	if n.PostTest {
		for {
			stop, brk, err := bodyStep()
			if err != nil {
				return nil, err
			}
			if stop {
				return brk, nil
			}
			ok, err := check()
			if err != nil {
				return nil, err
			}
			if !ok {
				break
			}
		}
		return object.NIL, nil
	}

	for {
		ok, err := check()
		if err != nil {
			return nil, err
		}
		if !ok {
			break
		}
		stop, brk, err := bodyStep()
		if err != nil {
			return nil, err
		}
		if stop {
			return brk, nil
		}
	}
	return object.NIL, nil
}

// evalForIn handles `for x in iter ... end`. Iter may be an Array,
// Hash, or Range; the loop var is rebound on each iteration in the
// surrounding scope (for-loops don't introduce a new scope in ruby).
func evalForIn(env *object.Environment, infix *ast.InfixExpression, body *ast.BlockStatement) (object.RubyObject, error) {
	id, ok := infix.Left.(*ast.Identifier)
	if !ok {
		return nil, errorf("evaluator: for-loop LHS must be a single identifier (got %T)", infix.Left)
	}
	iter, err := Eval(infix.Right, env)
	if err != nil {
		return nil, err
	}
	// step returns (stop, breakValue, err). breakValue is nil unless
	// the iteration was halted by `break`.
	step := func(v object.RubyObject) (bool, object.RubyObject, error) {
		env.AssignVisible(id.Value, v)
		_, err := evalBlockStatement(env, body)
		if err == nil {
			return false, nil, nil
		}
		if _, ok := err.(*nextSignal); ok {
			return false, nil, nil
		}
		if bs, ok := err.(*breakSignal); ok {
			return true, bs.Value, nil
		}
		return false, nil, err
	}
	switch r := iter.(type) {
	case *object.Array:
		for _, e := range r.Elements {
			stop, brk, err := step(e)
			if err != nil {
				return nil, err
			}
			if stop {
				return brk, nil
			}
		}
	case *object.Range:
		lo, hi, ok := rangeIntegerBounds(r)
		if !ok {
			return nil, errorf("evaluator: for-in over Range needs Integer bounds")
		}
		end := hi
		if !r.Exclusive {
			end = hi + 1
		}
		for i := lo; i < end; i++ {
			stop, brk, err := step(object.NewInteger(i))
			if err != nil {
				return nil, err
			}
			if stop {
				return brk, nil
			}
		}
	case *object.Hash:
		for _, e := range r.Entries {
			stop, brk, err := step(object.NewArray(e.Key, e.Value))
			if err != nil {
				return nil, err
			}
			if stop {
				return brk, nil
			}
		}
	default:
		return nil, errorf("evaluator: for-in over %T not yet supported", iter)
	}
	return iter, nil
}

func evalCase(env *object.Environment, n *ast.CaseExpression) (object.RubyObject, error) {
	if len(n.InClauses) > 0 {
		return nil, errorf("evaluator: case/in pattern matching not yet supported")
	}

	var subject object.RubyObject
	if n.Condition != nil {
		v, err := Eval(n.Condition, env)
		if err != nil {
			return nil, err
		}
		subject = v
	}

	matches := func(pattern object.RubyObject) bool {
		if subject == nil {
			return truthy(pattern)
		}
		return caseEqual(env, pattern, subject)
	}

	for _, w := range n.WhenClauses {
		hit := false
		for _, cond := range w.Conditions {
			v, err := Eval(cond, env)
			if err != nil {
				return nil, err
			}
			// `when A, B, C` arrives as a single ExpressionList condition;
			// match against each member.
			if list, ok := v.(rubyObjects); ok {
				for _, item := range list {
					if matches(item) {
						hit = true
						break
					}
				}
			} else if matches(v) {
				hit = true
			}
			if hit {
				break
			}
		}
		if hit {
			return evalBlockStatement(env, w.Body)
		}
	}
	if n.ElseBody != nil {
		return evalBlockStatement(env, n.ElseBody)
	}
	return object.NIL, nil
}

// caseEqual implements the `===` operator. For Class patterns it
// behaves like `subject.is_a?(pattern)`; for Range it tests
// containment; otherwise it falls back to `==`.
func caseEqual(env *object.Environment, pattern, subject object.RubyObject) bool {
	if cls, ok := pattern.(*object.Class); ok {
		c := classOfRaw(env, subject)
		if c == nil {
			return false
		}
		return c.IsAncestor(cls)
	}
	if re, ok := pattern.(*object.Regex); ok {
		s, ok := stringText(env, subject)
		if !ok {
			return false
		}
		return re.RE.MatchString(s)
	}
	if rng, ok := pattern.(*object.Range); ok {
		i, ok := subject.(*object.Integer)
		if !ok {
			return false
		}
		lo, hi, ok := rangeIntegerBounds(rng)
		if !ok {
			return false
		}
		if i.Value < lo {
			return false
		}
		if rng.Exclusive {
			return i.Value < hi
		}
		return i.Value <= hi
	}
	return rubyEqual(pattern, subject)
}

