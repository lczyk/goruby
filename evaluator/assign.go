package evaluator

import (
	"github.com/lczyk/goruby/ast"
	"github.com/lczyk/goruby/object"
)

// rubyObjects packs an ExpressionList's element values. Distinct from
// object.Array so the evaluator can tell "naked comma-expression" from
// "explicit [1, 2, 3]" -- multi-assignment unpacks the former without
// the array-wrapping it would impose on the latter.
type rubyObjects []object.RubyObject

func (r rubyObjects) Inspect() string         { return "" } // never user-visible
func (r rubyObjects) Type() object.Type       { return 0 }
func (r rubyObjects) Class() object.RubyClass { return nil }

func evalExpressionList(env *object.Environment, list ast.ExpressionList) (object.RubyObject, error) {
	vals := make(rubyObjects, 0, len(list))
	for _, e := range list {
		v, err := Eval(e, env)
		if err != nil {
			return nil, err
		}
		vals = append(vals, v)
	}
	return vals, nil
}

func evalIdentifier(env *object.Environment, n *ast.Identifier) (object.RubyObject, error) {
	if v, ok := env.Get(n.Value); ok {
		return v, nil
	}
	return nil, errorf("evaluator: NameError: undefined local variable or method `%s'", n.Value)
}

func evalGlobal(env *object.Environment, n *ast.Global) (object.RubyObject, error) {
	if v, ok := env.Get(n.Value); ok {
		return v, nil
	}
	// MRI returns nil (with a warning) for unset globals. Mirror the
	// value behaviour; the warning belongs to a later observability pass.
	return object.NIL, nil
}

func evalAssignment(env *object.Environment, n *ast.Assignment) (object.RubyObject, error) {
	right, err := Eval(n.Right, env)
	if err != nil {
		return nil, err
	}

	switch lhs := n.Left.(type) {
	case *ast.Identifier:
		env.Set(lhs.Value, expandSingle(right))
		return right, nil
	case *ast.Global:
		env.SetGlobal(lhs.Value, expandSingle(right))
		return right, nil
	case ast.ExpressionList:
		return evalMultiAssign(env, lhs, right)
	}
	return nil, errorf("evaluator: unsupported assignment lhs %T", n.Left)
}

// expandSingle collapses a single-element rubyObjects back into the
// scalar value. Multi-rhs (a, b = 1, 2) keeps the list shape; single-rhs
// must not bind `a = (1)` to a 1-tuple.
func expandSingle(o object.RubyObject) object.RubyObject {
	if r, ok := o.(rubyObjects); ok && len(r) == 1 {
		return r[0]
	}
	return o
}

func evalMultiAssign(env *object.Environment, lhs ast.ExpressionList, right object.RubyObject) (object.RubyObject, error) {
	values := unpackMultiRHS(right)

	for i, target := range lhs {
		var v object.RubyObject
		if i < len(values) {
			v = values[i]
		} else {
			v = object.NIL
		}
		if err := assignTarget(env, target, v); err != nil {
			return nil, err
		}
	}
	return right, nil
}

// unpackMultiRHS turns the right side of a multi-assignment into the
// sequence of bind values. Three shapes:
//   - rubyObjects (ExpressionList on the rhs): pass through.
//   - *object.Array (e.g. `x, y, z = [1, 2, 3]`): unpack the elements.
//   - any other single value: wrap as a one-element slice, leaving
//     surplus targets to bind to nil.
func unpackMultiRHS(o object.RubyObject) []object.RubyObject {
	switch r := o.(type) {
	case rubyObjects:
		return []object.RubyObject(r)
	case *object.Array:
		return r.Elements
	}
	return []object.RubyObject{o}
}

func assignTarget(env *object.Environment, target ast.Expression, value object.RubyObject) error {
	switch t := target.(type) {
	case *ast.Identifier:
		env.Set(t.Value, value)
		return nil
	case *ast.Global:
		env.SetGlobal(t.Value, value)
		return nil
	}
	return errorf("evaluator: unsupported multi-assignment target %T", target)
}
