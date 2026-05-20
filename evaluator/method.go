package evaluator

import (
	"github.com/lczyk/goruby/ast"
	"github.com/lczyk/goruby/object"
)

// evalContextCall handles both implicit-self calls (e.g. `puts 1`,
// where Context is nil) and receiver calls (e.g. `[1,2,3].length`).
func evalContextCall(env *object.Environment, n *ast.ContextCallExpression) (object.RubyObject, error) {
	if n.Block != nil {
		return nil, errorf("evaluator: blocks not yet supported")
	}

	args, err := evalExpressions(env, n.Arguments)
	if err != nil {
		return nil, err
	}

	if n.Context == nil {
		return callKernel(env, n.Function.Value, args)
	}

	recv, err := Eval(n.Context, env)
	if err != nil {
		return nil, err
	}
	return callMethod(env, recv, n.Function.Value, args)
}

func evalExpressions(env *object.Environment, exprs []ast.Expression) ([]object.RubyObject, error) {
	out := make([]object.RubyObject, 0, len(exprs))
	for _, e := range exprs {
		v, err := Eval(e, env)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

// callMethod is the minimal hand-rolled dispatcher used until the class
// machinery lands. Hardcoded for the methods the literals corpus needs.
func callMethod(env *object.Environment, recv object.RubyObject, name string, args []object.RubyObject) (object.RubyObject, error) {
	switch r := recv.(type) {
	case *object.Array:
		switch name {
		case "length", "size":
			return object.NewInteger(int64(len(r.Elements))), nil
		}
	case *object.Hash:
		switch name {
		case "length", "size":
			return object.NewInteger(int64(len(r.Entries))), nil
		}
	}
	return nil, errorf("evaluator: NoMethodError: undefined method `%s' for %T", name, recv)
}
