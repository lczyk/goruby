package evaluator

import (
	"strings"

	"github.com/lczyk/goruby/ast"
	"github.com/lczyk/goruby/object"
)

// Eval walks node and returns its runtime value. Errors are evaluator
// errors (programmer / unimplemented), distinct from ruby exceptions.
func Eval(node ast.Node, env *object.Environment) (object.RubyObject, error) {
	switch n := node.(type) {

	case *ast.Program:
		return evalProgram(env, n)

	case *ast.ExpressionStatement:
		return Eval(n.Expression, env)

	// Literals -------------------------------------------------------

	case *ast.IntegerLiteral:
		return object.NewInteger(n.Value), nil

	case *ast.Boolean:
		return object.BooleanOf(n.Value), nil

	case *ast.Nil:
		return object.NIL, nil

	case *ast.StringLiteral:
		return evalStringLiteral(env, n)

	case *ast.SymbolLiteral:
		return evalSymbolLiteral(env, n)

	case *ast.ArrayLiteral:
		return evalArrayLiteral(env, n)

	case *ast.HashLiteral:
		return evalHashLiteral(env, n)

	// Compound -------------------------------------------------------

	case *ast.PrefixExpression:
		return evalPrefix(env, n)

	case *ast.InfixExpression:
		return evalInfix(env, n)

	case *ast.IndexExpression:
		return evalIndex(env, n)

	case *ast.ContextCallExpression:
		return evalContextCall(env, n)
	}

	return nil, errorf("evaluator: unhandled AST node type %T", node)
}

func evalProgram(env *object.Environment, p *ast.Program) (object.RubyObject, error) {
	var result object.RubyObject = object.NIL
	for _, stmt := range p.Statements {
		v, err := Eval(stmt, env)
		if err != nil {
			return nil, err
		}
		result = v
	}
	return result, nil
}

func evalStringLiteral(env *object.Environment, n *ast.StringLiteral) (object.RubyObject, error) {
	// Interpolation (Parts) deferred to a later step. Literal corpus only
	// uses Value-form strings.
	if len(n.Parts) > 0 {
		return nil, errorf("evaluator: string interpolation not yet supported")
	}
	// Parser keeps escape sequences verbatim in Value (for source-faithful
	// roundtripping). The evaluator decodes them per the quote style.
	decoded := decodeStringEscapes(n.Value, n.Token.SingleQuoted)
	return object.NewString(decoded), nil
}

// decodeStringEscapes interprets backslash escapes in raw source text.
// Double-quoted strings honour the common C-style escapes (\n, \t, \r,
// \\, \", \0, \a, \b, \e, \f, \v, \s); single-quoted strings only honour
// \\ and \'. Unknown escapes in double-quoted strings drop the
// backslash, matching MRI.
func decodeStringEscapes(s string, singleQuoted bool) string {
	if !strings.ContainsRune(s, '\\') {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '\\' || i+1 >= len(s) {
			b.WriteByte(c)
			continue
		}
		next := s[i+1]
		if singleQuoted {
			switch next {
			case '\\', '\'':
				b.WriteByte(next)
				i++
				continue
			}
			b.WriteByte(c)
			continue
		}
		switch next {
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case 'r':
			b.WriteByte('\r')
		case '\\':
			b.WriteByte('\\')
		case '"':
			b.WriteByte('"')
		case '\'':
			b.WriteByte('\'')
		case '0':
			b.WriteByte(0)
		case 'a':
			b.WriteByte(7)
		case 'b':
			b.WriteByte(8)
		case 'e':
			b.WriteByte(27)
		case 'f':
			b.WriteByte(12)
		case 'v':
			b.WriteByte(11)
		case 's':
			b.WriteByte(' ')
		default:
			b.WriteByte(next)
		}
		i++
	}
	return b.String()
}

func evalSymbolLiteral(env *object.Environment, n *ast.SymbolLiteral) (object.RubyObject, error) {
	// Parser stores the symbol's name as either *ast.Identifier (`:foo`)
	// or *ast.StringLiteral (`a:` hash label). Both forms carry the
	// bare name; intern it.
	name, err := symbolName(n.Value)
	if err != nil {
		return nil, err
	}
	return env.Symbols().Intern(name), nil
}

func symbolName(expr ast.Expression) (string, error) {
	switch v := expr.(type) {
	case *ast.Identifier:
		return v.Value, nil
	case *ast.StringLiteral:
		if len(v.Parts) > 0 {
			return "", errorf("evaluator: interpolated symbol not yet supported")
		}
		return v.Value, nil
	}
	return "", errorf("evaluator: malformed SymbolLiteral.Value: %T", expr)
}

func evalArrayLiteral(env *object.Environment, n *ast.ArrayLiteral) (object.RubyObject, error) {
	elems := make([]object.RubyObject, 0, len(n.Elements))
	for _, e := range n.Elements {
		v, err := Eval(e, env)
		if err != nil {
			return nil, err
		}
		elems = append(elems, v)
	}
	return object.NewArray(elems...), nil
}

func evalHashLiteral(env *object.Environment, n *ast.HashLiteral) (object.RubyObject, error) {
	if len(n.Splats) > 0 {
		return nil, errorf("evaluator: hash splat (**) not yet supported")
	}
	entries := make([]object.HashEntry, 0, n.Map.Len())
	for _, kv := range n.Map.Entries() {
		k, err := Eval(kv.Key, env)
		if err != nil {
			return nil, err
		}
		v, err := Eval(kv.Value, env)
		if err != nil {
			return nil, err
		}
		entries = append(entries, object.HashEntry{Key: k, Value: v})
	}
	return object.NewHash(entries...), nil
}

func evalPrefix(env *object.Environment, n *ast.PrefixExpression) (object.RubyObject, error) {
	right, err := Eval(n.Right, env)
	if err != nil {
		return nil, err
	}
	switch n.Operator {
	case "-":
		if i, ok := right.(*object.Integer); ok {
			return object.NewInteger(-i.Value), nil
		}
		if f, ok := right.(*object.Float); ok {
			return object.NewFloat(-f.Value), nil
		}
	case "!":
		return object.BooleanOf(!truthy(right)), nil
	case "+":
		return right, nil
	}
	return nil, errorf("evaluator: unsupported prefix %q on %T", n.Operator, right)
}

func evalInfix(env *object.Environment, n *ast.InfixExpression) (object.RubyObject, error) {
	left, err := Eval(n.Left, env)
	if err != nil {
		return nil, err
	}
	right, err := Eval(n.Right, env)
	if err != nil {
		return nil, err
	}
	switch n.Operator {
	case "==":
		return object.BooleanOf(rubyEqual(left, right)), nil
	case "!=":
		return object.BooleanOf(!rubyEqual(left, right)), nil
	}
	return nil, errorf("evaluator: unsupported infix %q for %T / %T", n.Operator, left, right)
}

func evalIndex(env *object.Environment, n *ast.IndexExpression) (object.RubyObject, error) {
	recv, err := Eval(n.Left, env)
	if err != nil {
		return nil, err
	}
	if len(n.Arguments) != 1 {
		return nil, errorf("evaluator: [] with %d args not yet supported", len(n.Arguments))
	}
	key, err := Eval(n.Arguments[0], env)
	if err != nil {
		return nil, err
	}
	switch r := recv.(type) {
	case *object.Hash:
		for _, e := range r.Entries {
			if rubyEqual(e.Key, key) {
				return e.Value, nil
			}
		}
		return object.NIL, nil
	case *object.Array:
		i, ok := key.(*object.Integer)
		if !ok {
			return nil, errorf("evaluator: Array#[] needs Integer index, got %T", key)
		}
		idx := int(i.Value)
		if idx < 0 {
			idx += len(r.Elements)
		}
		if idx < 0 || idx >= len(r.Elements) {
			return object.NIL, nil
		}
		return r.Elements[idx], nil
	}
	return nil, errorf("evaluator: [] not yet supported on %T", recv)
}

// truthy follows ruby semantics: only nil and false are falsy.
func truthy(o object.RubyObject) bool {
	switch v := o.(type) {
	case *object.Nil:
		return false
	case *object.Boolean:
		return v.Value
	}
	return true
}

// rubyEqual implements ruby `==` between common runtime types. Returns
// false for incomparable type pairs (matching MRI's default Object#==).
func rubyEqual(a, b object.RubyObject) bool {
	switch x := a.(type) {
	case *object.Integer:
		y, ok := b.(*object.Integer)
		return ok && x.Value == y.Value
	case *object.Float:
		y, ok := b.(*object.Float)
		return ok && x.Value == y.Value
	case *object.Symbol:
		y, ok := b.(*object.Symbol)
		return ok && x.ID == y.ID
	case *object.String:
		y, ok := b.(*object.String)
		return ok && string(x.Buf) == string(y.Buf)
	case *object.Boolean:
		y, ok := b.(*object.Boolean)
		return ok && x.Value == y.Value
	case *object.Nil:
		_, ok := b.(*object.Nil)
		return ok
	}
	return false
}
