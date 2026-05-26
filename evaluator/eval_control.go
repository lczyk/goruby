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
	// LHS shapes:
	//   for x in iter     -- single Identifier
	//   for k, v in iter  -- ExpressionList of Identifiers; destructure
	//                        each element via auto-splat
	var names []string
	switch lhs := infix.Left.(type) {
	case *ast.Identifier:
		names = []string{lhs.Value}
	case ast.ExpressionList:
		for _, e := range lhs {
			id, ok := e.(*ast.Identifier)
			if !ok {
				return nil, errorf("evaluator: for-loop LHS element must be an identifier, got %T", e)
			}
			names = append(names, id.Value)
		}
	default:
		return nil, errorf("evaluator: for-loop LHS must be an identifier or list (got %T)", infix.Left)
	}
	iter, err := Eval(infix.Right, env)
	if err != nil {
		return nil, err
	}
	step := func(v object.RubyObject) (bool, object.RubyObject, error) {
		if len(names) == 1 {
			env.AssignVisible(names[0], v)
		} else {
			// Auto-splat: destructure Array; missing slots bind nil,
			// extras drop.
			arr, ok := v.(*object.Array)
			if !ok {
				return false, nil, errorf("evaluator: for-loop expected Array for multi-var LHS, got %T", v)
			}
			for i, n := range names {
				if i < len(arr.Elements) {
					env.AssignVisible(n, arr.Elements[i])
				} else {
					env.AssignVisible(n, object.NIL)
				}
			}
		}
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
	case *object.Instance:
		// For-in over a user object: dispatch `each` with a
		// goBlockMarker that runs step on each yielded value.
		cb := func(args []object.RubyObject) (object.RubyObject, error) {
			var v object.RubyObject
			if len(args) == 1 {
				v = args[0]
			} else if len(args) > 1 {
				v = object.NewArray(args...)
			} else {
				v = object.NIL
			}
			stop, brk, err := step(v)
			if err != nil {
				return nil, err
			}
			if stop {
				return brk, &breakSignal{Value: brk}
			}
			return object.NIL, nil
		}
		_, err := callMethodWithProc(env, r, "each", nil, procFromGoBlock(&goBlockMarker{fn: cb}))
		if err != nil {
			if bs, ok := err.(*breakSignal); ok {
				return bs.Value, nil
			}
			return nil, err
		}
	default:
		return nil, errorf("evaluator: for-in over %T not yet supported", iter)
	}
	return iter, nil
}

func evalCase(env *object.Environment, n *ast.CaseExpression) (object.RubyObject, error) {
	if len(n.InClauses) > 0 {
		return evalCaseIn(env, n)
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

// evalCaseIn implements case/in pattern matching (ruby 3.0+). Each
// `in <pattern>` clause is tried in order; on a successful match the
// pattern's named bindings are installed in env and the body runs.
// No matching clause + no else body returns nil.
func evalCaseIn(env *object.Environment, n *ast.CaseExpression) (object.RubyObject, error) {
	if n.Condition == nil {
		return nil, errorf("evaluator: case/in requires a subject expression")
	}
	subject, err := Eval(n.Condition, env)
	if err != nil {
		return nil, err
	}
	for _, in := range n.InClauses {
		if len(in.Conditions) == 0 {
			continue
		}
		// in clauses always carry exactly one pattern; the slice shape
		// is shared w/ WhenClause.
		ok, err := matchPattern(env, in.Conditions[0], subject)
		if err != nil {
			return nil, err
		}
		if ok {
			return evalBlockStatement(env, in.Body)
		}
	}
	if n.ElseBody != nil {
		return evalBlockStatement(env, n.ElseBody)
	}
	return object.NIL, nil
}

// matchPattern checks subject against an ast pattern node. On success
// any named bindings are installed in env via AssignVisible and the
// function returns true. Supported pattern shapes (the subset the
// corpus exercises today):
//
//   - literal patterns: IntegerLiteral, StringLiteral (no interp),
//     SymbolLiteral, true/false/nil. Match by rubyEqual.
//   - class pattern: an Identifier resolving to a Class. Match by
//     subject.is_a?(class).
//   - identifier binding: lowercase Identifier. Always matches; binds
//     subject to the name.
//   - capture: `<pat> => name` (InfixExpression Operator "=>").
//     Match pat, then bind subject to name.
//   - array pattern: ArrayLiteral. Subject must be Array; element
//     count matches (with a single `*name` splat allowed, consuming
//     the unmatched middle / tail).
//   - hash pattern: HashLiteral. Subject must be Hash; every pattern
//     key must be present and its value match the sub-pattern.
//
// Anything else is rejected as unsupported so callers see the gap.
func matchPattern(env *object.Environment, pat ast.Expression, subject object.RubyObject) (bool, error) {
	switch p := pat.(type) {
	case *ast.IntegerLiteral:
		v, err := Eval(p, env)
		if err != nil {
			return false, err
		}
		return rubyEqual(v, subject), nil
	case *ast.SymbolLiteral:
		v, err := Eval(p, env)
		if err != nil {
			return false, err
		}
		return rubyEqual(v, subject), nil
	case *ast.StringLiteral:
		if len(p.Parts) > 0 {
			return false, errorf("evaluator: case/in: interpolated string pattern not supported")
		}
		v, err := Eval(p, env)
		if err != nil {
			return false, err
		}
		return rubyEqual(v, subject), nil
	case *ast.Nil:
		_, isNil := subject.(*object.Nil)
		return isNil, nil
	case *ast.Boolean:
		v, err := Eval(p, env)
		if err != nil {
			return false, err
		}
		return rubyEqual(v, subject), nil
	case *ast.Identifier:
		if isConstantName(p.Value) {
			// Class / constant pattern: subject.is_a?(constant).
			v, err := Eval(p, env)
			if err != nil {
				return false, err
			}
			if cls, ok := v.(*object.Class); ok {
				c := classOfRaw(env, subject)
				return c != nil && c.IsAncestor(cls), nil
			}
			return rubyEqual(v, subject), nil
		}
		// Lowercase identifier: always matches, binds subject.
		env.AssignVisible(p.Value, subject)
		return true, nil
	case *ast.InfixExpression:
		if p.Operator == "=>" {
			ok, err := matchPattern(env, p.Left, subject)
			if err != nil || !ok {
				return false, err
			}
			id, ok := p.Right.(*ast.Identifier)
			if !ok {
				return false, errorf("evaluator: case/in: capture RHS must be an identifier, got %T", p.Right)
			}
			env.AssignVisible(id.Value, subject)
			return true, nil
		}
		return false, errorf("evaluator: case/in: unsupported infix pattern %q", p.Operator)
	case *ast.ArrayLiteral:
		return matchArrayPattern(env, p, subject)
	case *ast.HashLiteral:
		return matchHashPattern(env, p, subject)
	}
	return false, errorf("evaluator: case/in: unsupported pattern %T", pat)
}

// matchArrayPattern handles [p1, p2, *rest, pn] forms. At most one
// splat is permitted; it absorbs the middle / tail. Element count must
// match exactly when there's no splat.
func matchArrayPattern(env *object.Environment, p *ast.ArrayLiteral, subject object.RubyObject) (bool, error) {
	arr, ok := subject.(*object.Array)
	if !ok {
		return false, nil
	}
	// Find splat position, if any.
	splatIdx := -1
	for i, e := range p.Elements {
		if pe, ok := e.(*ast.PrefixExpression); ok && pe.Operator == "*" {
			if splatIdx >= 0 {
				return false, errorf("evaluator: case/in: at most one splat per array pattern")
			}
			splatIdx = i
		}
	}
	if splatIdx < 0 {
		if len(p.Elements) != len(arr.Elements) {
			return false, nil
		}
		for i, e := range p.Elements {
			ok, err := matchPattern(env, e, arr.Elements[i])
			if err != nil || !ok {
				return ok, err
			}
		}
		return true, nil
	}
	// With splat: head elements 0..splatIdx-1 match first N of subject;
	// tail elements splatIdx+1..end match last M of subject; splat
	// consumes the middle.
	head := p.Elements[:splatIdx]
	tail := p.Elements[splatIdx+1:]
	if len(arr.Elements) < len(head)+len(tail) {
		return false, nil
	}
	for i, e := range head {
		ok, err := matchPattern(env, e, arr.Elements[i])
		if err != nil || !ok {
			return ok, err
		}
	}
	for i, e := range tail {
		ok, err := matchPattern(env, e, arr.Elements[len(arr.Elements)-len(tail)+i])
		if err != nil || !ok {
			return ok, err
		}
	}
	// Bind splat name (if any) to the middle slice.
	splat := p.Elements[splatIdx].(*ast.PrefixExpression)
	if id, ok := splat.Right.(*ast.Identifier); ok {
		mid := arr.Elements[len(head) : len(arr.Elements)-len(tail)]
		copied := make([]object.RubyObject, len(mid))
		copy(copied, mid)
		env.AssignVisible(id.Value, object.NewArray(copied...))
	}
	return true, nil
}

// matchHashPattern handles {key: pat, ...} subset matching. Every
// pattern key must exist in the subject hash and its value must match
// the sub-pattern. Pattern keys are symbol literals (the Map already
// canonicalises `name:` shorthand to a SymbolLiteral).
func matchHashPattern(env *object.Environment, p *ast.HashLiteral, subject object.RubyObject) (bool, error) {
	h, ok := subject.(*object.Hash)
	if !ok {
		return false, nil
	}
	for _, kv := range p.Map.Entries() {
		k, err := Eval(kv.Key, env)
		if err != nil {
			return false, err
		}
		// Find matching entry in subject. Hash entries are linear; the
		// corpus uses small hashes so a scan is fine.
		var found object.RubyObject
		hit := false
		for _, e := range h.Entries {
			if rubyEqual(k, e.Key) {
				found = e.Value
				hit = true
				break
			}
		}
		if !hit {
			return false, nil
		}
		// Shorthand `{name:}` binds the value to a local of the same
		// name; the parser leaves Value nil in that case. Treat as if
		// the pattern were `name: name`.
		if kv.Value == nil {
			if sym, ok := kv.Key.(*ast.SymbolLiteral); ok {
				if id, ok := sym.Value.(*ast.Identifier); ok {
					env.AssignVisible(id.Value, found)
					continue
				}
			}
			return false, errorf("evaluator: case/in: malformed hash-pattern shorthand")
		}
		ok, err := matchPattern(env, kv.Value, found)
		if err != nil || !ok {
			return ok, err
		}
	}
	return true, nil
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
		idx := re.RE.FindStringSubmatchIndex(s)
		if idx == nil {
			setMatchGlobals(env, nil)
			return false
		}
		setMatchGlobals(env, submatchesFromIndices(s, idx))
		return true
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

