package evaluator

import (
	"sort"
	"strings"

	"github.com/lczyk/goruby/ast"
	"github.com/lczyk/goruby/object"
	"github.com/lczyk/goruby/token"
)

// flattenArrayDepth flattens at most `depth` levels. -1 means flatten
// until no nested Arrays remain.
func flattenArrayDepth(a *object.Array, depth int) []object.RubyObject {
	out := make([]object.RubyObject, 0, len(a.Elements))
	for _, e := range a.Elements {
		if inner, ok := e.(*object.Array); ok && depth != 0 {
			out = append(out, flattenArrayDepth(inner, depth-1)...)
			continue
		}
		out = append(out, e)
	}
	return out
}

// sortStable is a thin wrapper over sort.SliceStable that accepts a
// swap fn so callers can sort parallel arrays without exposing the
// underlying slice type.
func sortStable(n int, less func(i, j int) bool, swap func(i, j int)) {
	sort.Stable(sortIface{n: n, less: less, swap: swap})
}

type sortIface struct {
	n    int
	less func(i, j int) bool
	swap func(i, j int)
}

func (s sortIface) Len() int           { return s.n }
func (s sortIface) Less(i, j int) bool { return s.less(i, j) }
func (s sortIface) Swap(i, j int)      { s.swap(i, j) }

// floorFloat / ceilFloat / roundFloat are tiny self-contained
// implementations to avoid pulling in math at this scale. Accurate for
// finite values in the corpus's range.
func floorFloat(v float64) float64 {
	i := int64(v)
	if v < 0 && v != float64(i) {
		i--
	}
	return float64(i)
}

func ceilFloat(v float64) float64 {
	i := int64(v)
	if v > 0 && v != float64(i) {
		i++
	}
	return float64(i)
}

func roundFloat(v float64) float64 {
	// Half-up away from zero, matching MRI's default Float#round.
	if v < 0 {
		return -floorFloat(-v + 0.5)
	}
	return floorFloat(v + 0.5)
}

// evalBinaryOp dispatches a binary op (the kind named by a symbol in
// `reduce(:+)`) between two values. Tries the numeric / string
// fast-paths first, then falls back to method-style dispatch so user
// classes that define the operator participate.
func evalBinaryOp(env *object.Environment, op string, left, right object.RubyObject) (object.RubyObject, error) {
	if v, handled, err := numericInfix(op, left, right); handled {
		if _, isZD := err.(zeroDivErr); isZD {
			return raiseBuiltin(env, "ZeroDivisionError", "divided by 0")
		}
		return v, err
	}
	if v, handled, err := stringInfix(env, op, left, right); handled {
		return v, err
	}
	return callMethod(env, left, op, []object.RubyObject{right})
}

// receiverResponds is a best-effort `respond_to?` answer for the
// receiver / name pair. Returns true if a built-in dispatch or user
// method would accept the call.
func receiverResponds(env *object.Environment, recv object.RubyObject, name string) bool {
	if inst, ok := recv.(*object.Instance); ok {
		_, found := dispatchClass(env, inst).LookupMethod(name)
		if found {
			return true
		}
	}
	if cls, ok := recv.(*object.Class); ok {
		if _, found := cls.LookupClassMethod(name); found {
			return true
		}
		if name == "new" || name == "name" || name == "superclass" {
			return true
		}
	}
	// Probe by dispatching; ignore the result. Reports false on any
	// NoMethodError-style failure. Cheap enough for the corpus.
	_, err := callMethod(env, recv, name, nil)
	if err == nil {
		return true
	}
	if strings.Contains(err.Error(), "NoMethodError") {
		return false
	}
	// Other errors (e.g. ArgumentError from a 0-arg probe of a
	// method that needs args) mean the method exists.
	return true
}

// sortArray sorts elements in place using ruby `<=>` semantics. For
// Instance elements, dispatches to the class's user-defined `<=>`.
func sortArray(env *object.Environment, xs []object.RubyObject) error {
	var sortErr error
	sort.SliceStable(xs, func(i, j int) bool {
		if sortErr != nil {
			return false
		}
		c, ok := compareObjectsEnv(env, xs[i], xs[j])
		if !ok {
			sortErr = errorf("evaluator: ArgumentError: comparison of %T with %T failed", xs[i], xs[j])
			return false
		}
		return c < 0
	})
	return sortErr
}

// compareObjects is the env-less form used in tests; production callers
// should prefer compareObjectsEnv so user-defined `<=>` dispatches and
// Symbols compare by name (Symbol#<=>).
func compareObjects(a, b object.RubyObject) (int, bool) {
	switch x := a.(type) {
	case *object.Integer:
		y, ok := b.(*object.Integer)
		if !ok {
			return 0, false
		}
		switch {
		case x.Value < y.Value:
			return -1, true
		case x.Value > y.Value:
			return 1, true
		}
		return 0, true
	case *object.Float:
		y, ok := b.(*object.Float)
		if !ok {
			return 0, false
		}
		switch {
		case x.Value < y.Value:
			return -1, true
		case x.Value > y.Value:
			return 1, true
		}
		return 0, true
	case *object.String:
		y, ok := b.(*object.String)
		if !ok {
			return 0, false
		}
		return strCompare(string(x.Buf), string(y.Buf)), true
	}
	return 0, false
}

// compareObjectsEnv extends compareObjects with Symbol-by-name and
// user `<=>` dispatch for Instance receivers.
func compareObjectsEnv(env *object.Environment, a, b object.RubyObject) (int, bool) {
	if c, ok := compareObjects(a, b); ok {
		return c, true
	}
	if sa, ok := a.(*object.Symbol); ok {
		if sb, ok := b.(*object.Symbol); ok {
			return strCompare(env.Symbols().Name(sa.ID), env.Symbols().Name(sb.ID)), true
		}
	}
	if inst, ok := a.(*object.Instance); ok {
		if _, found := dispatchClass(env, inst).LookupMethod("<=>"); found {
			v, err := callMethod(env, a, "<=>", []object.RubyObject{b})
			if err != nil {
				return 0, false
			}
			i, ok := v.(*object.Integer)
			if !ok {
				return 0, false
			}
			return int(i.Value), true
		}
	}
	return 0, false
}

func strCompare(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// evalContextCall handles both implicit-self calls (e.g. `puts 1`,
// where Context is nil) and receiver calls (e.g. `[1,2,3].length`).
func evalContextCall(env *object.Environment, n *ast.ContextCallExpression) (object.RubyObject, error) {
	args, kwargs, blockProc, err := evalCallArguments(env, n.Arguments)
	if err != nil {
		return nil, err
	}
	// Stash pending kwargs on the env so the dispatch path (which
	// builds a fresh callEnv) can copy them onto the new frame's
	// CurrentKwargs without us threading another parameter through
	// every helper. Cleared after this call returns.
	prevKwargs := env.CurrentKwargs
	env.CurrentKwargs = kwargs
	defer func() { env.CurrentKwargs = prevKwargs }()

	if n.Context == nil {
		if cls := env.EnclosingClass(); cls != nil {
			if v, handled, err := classBodyDSL(env, cls, n.Function.Value, args); handled {
				return v, err
			}
		}
		// Implicit-self: bare-name calls inside a method body should
		// dispatch to a matching instance method on the current self,
		// or to derivations on the receiver (Comparable / Enumerable).
		// Top-level method definitions (registered via env.SetMethod)
		// take precedence so a recursive bare call inside a method
		// body finds itself even when self happens to be a user
		// Instance -- only fall back to instance dispatch when no top-
		// level method matches.
		if self := env.EnclosingSelf(); self != nil {
			if _, isTop := env.GetMethod(n.Function.Value); !isTop {
				// Implicit-self inside a class method: dispatch via
				// the receiver's Class table (covers `new` inside
				// `def self.foo`).
				if cls, ok := self.(*object.Class); ok {
					if n.Block != nil {
						return callMethodWithBlock(env, cls, n.Function.Value, args, n.Block)
					}
					if blockProc != nil {
						return callMethodWithProc(env, cls, n.Function.Value, args, blockProc)
					}
					if v, handled, err := callOnClass(env, cls, n.Function.Value, args); handled {
						return v, err
					}
				}
				if inst, ok := self.(*object.Instance); ok {
					if m, found := dispatchClass(env, inst).LookupMethod(n.Function.Value); found {
						if um, ok := m.(*object.UserMethod); ok {
							if n.Block != nil {
								return invokeMethodOnWithBlock(env, inst, um, args, n.Block)
							}
							if blockProc != nil {
								return callMethodWithProc(env, inst, n.Function.Value, args, blockProc)
							}
							return invokeMethodOn(env, inst, um, args, nil)
						}
					}
					// Fall through to receiver-style dispatch so
					// Comparable / Enumerable derivations are visible
					// from inside the class's own methods.
					if n.Block != nil {
						return callMethodWithBlock(env, inst, n.Function.Value, args, n.Block)
					}
					if blockProc != nil {
						return callMethodWithProc(env, inst, n.Function.Value, args, blockProc)
					}
					if v, handled, err := callEnumerable(env, inst, n.Function.Value, args); handled {
						return v, err
					}
					// Last-ditch implicit-self: route through
					// callMethod so universal methods (send,
					// instance_variable_set, etc.) work. Suppress
					// NoMethodError so we still fall back to kernel.
					if v, err := callMethod(env, inst, n.Function.Value, args); err == nil {
						return v, nil
					}
				}
			}
		}
		if n.Block != nil {
			if m, ok := env.GetMethod(n.Function.Value); ok {
				if um, ok := m.(*object.UserMethod); ok {
					return callUserMethodWithBlock(env, um, args, n.Block)
				}
			}
			if n.Function.Value == "loop" {
				return kernelLoop(env, n.Block)
			}
			return nil, errorf("evaluator: blocks on kernel calls not yet supported")
		}
		if blockProc != nil {
			if m, ok := env.GetMethod(n.Function.Value); ok {
				if um, ok := m.(*object.UserMethod); ok {
					// User method called with &block-capture: wrap the
					// Proc into a synthetic block payload via the
					// shared callUserMethodWithBlock path.
					return callUserMethodWithProc(env, um, args, blockProc)
				}
			}
		}
		return callKernel(env, n.Function.Value, args)
	}

	recv, err := Eval(n.Context, env)
	if err != nil {
		return nil, err
	}
	// Safe-navigation `&.`: skip the call when the receiver is nil,
	// returning nil straight through.
	if n.OpType == token.LONELY {
		if _, isNil := recv.(*object.Nil); isNil {
			return object.NIL, nil
		}
	}
	if n.Block != nil {
		return callMethodWithBlock(env, recv, n.Function.Value, args, n.Block)
	}
	if blockProc != nil {
		return callMethodWithProc(env, recv, n.Function.Value, args, blockProc)
	}
	return callMethod(env, recv, n.Function.Value, args)
}

// evalCallArguments evaluates a call's argument list, splitting out
// trailing keyword args and a `&block` capture. Arguments shaped like
// `name: value` are emitted by the parser as `InfixExpression{Operator: ":"}`;
// `&expr` arrives as a `BlockCapture` node.
func evalCallArguments(env *object.Environment, exprs []ast.Expression) ([]object.RubyObject, map[string]object.RubyObject, *object.Proc, error) {
	var args []object.RubyObject
	var kwargs map[string]object.RubyObject
	var blockProc *object.Proc
	for _, e := range exprs {
		if bc, ok := e.(*ast.BlockCapture); ok {
			p, err := blockCaptureToProc(env, bc)
			if err != nil {
				return nil, nil, nil, err
			}
			blockProc = p
			continue
		}
		if sp, ok := e.(*ast.SplatExpression); ok {
			v, err := Eval(sp.Right, env)
			if err != nil {
				return nil, nil, nil, err
			}
			switch sp.Operator {
			case "*":
				arr, ok := v.(*object.Array)
				if !ok {
					args = append(args, v)
					continue
				}
				args = append(args, arr.Elements...)
				continue
			case "**":
				h, ok := v.(*object.Hash)
				if !ok {
					return nil, nil, nil, errorf("evaluator: ** splat needs Hash, got %T", v)
				}
				if kwargs == nil {
					kwargs = make(map[string]object.RubyObject)
				}
				for _, ent := range h.Entries {
					name, ok := symbolOrString(env, ent.Key)
					if !ok {
						return nil, nil, nil, errorf("evaluator: ** splat key must be Symbol/String, got %T", ent.Key)
					}
					kwargs[name] = ent.Value
				}
				continue
			}
		}
		if key, val, ok := splitKwargInfix(e); ok {
			v, err := Eval(val, env)
			if err != nil {
				return nil, nil, nil, err
			}
			if kwargs == nil {
				kwargs = make(map[string]object.RubyObject)
			}
			kwargs[key] = v
			continue
		}
		v, err := Eval(e, env)
		if err != nil {
			return nil, nil, nil, err
		}
		args = append(args, v)
	}
	return args, kwargs, blockProc, nil
}

// blockCaptureToProc resolves `&x` for the shapes the evaluator
// supports today: an Identifier already bound to a Proc, an inline
// Symbol literal (`&:upcase`), or any expression that evaluates to a
// Proc.
func blockCaptureToProc(env *object.Environment, bc *ast.BlockCapture) (*object.Proc, error) {
	if bc.Name != nil {
		v, ok := env.Get(bc.Name.Value)
		if !ok {
			return nil, errorf("evaluator: NameError: undefined local variable `%s' for &-capture", bc.Name.Value)
		}
		// `&nil` is the legal "no block" form -- forwarded as "no
		// block" rather than erroring.
		if _, isNil := v.(*object.Nil); isNil {
			return nil, nil
		}
		p, ok := v.(*object.Proc)
		if !ok {
			return nil, errorf("evaluator: &-capture: %T is not a Proc", v)
		}
		return p, nil
	}
	if bc.Expr == nil {
		return nil, errorf("evaluator: empty &-capture")
	}
	if sym, ok := bc.Expr.(*ast.SymbolLiteral); ok {
		name, err := symbolName(sym.Value)
		if err != nil {
			return nil, err
		}
		return procFromSymbol(name), nil
	}
	v, err := Eval(bc.Expr, env)
	if err != nil {
		return nil, err
	}
	p, ok := v.(*object.Proc)
	if !ok {
		return nil, errorf("evaluator: &-capture: %T is not a Proc", v)
	}
	return p, nil
}

// splitKwargInfix recognises `key:value` call-site syntax (parsed as
// `InfixExpression{Operator: ":"}` with left = SymbolLiteral).
func splitKwargInfix(e ast.Expression) (string, ast.Expression, bool) {
	infix, ok := e.(*ast.InfixExpression)
	if !ok || infix.Operator != ":" {
		return "", nil, false
	}
	sym, ok := infix.Left.(*ast.SymbolLiteral)
	if !ok {
		return "", nil, false
	}
	switch v := sym.Value.(type) {
	case *ast.Identifier:
		return v.Value, infix.Right, true
	case *ast.StringLiteral:
		return v.Value, infix.Right, true
	}
	return "", nil, false
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

// callMethod dispatches `name` on `recv`. Send is consulted first; if
// no class-owned method matches, fall back to the legacy hand-rolled
// switch in callMethodLegacy. The legacy path shrinks as each builtin
// type's methods migrate onto its class.
func callMethod(env *object.Environment, recv object.RubyObject, name string, args []object.RubyObject) (object.RubyObject, error) {
	if v, ok, err := object.Send(env, recv, name, args, nil); ok {
		return v, err
	}
	return callMethodLegacy(env, recv, name, args)
}

// callMethodLegacy is the hand-rolled fallback dispatch. BuiltinMethod
// adapters call it directly to reuse existing per-name implementations
// without re-entering Send (which would loop on the adapter itself).
// Universal Object methods have moved to object_methods.go (registered
// on object.ObjectClass); fully-migrated builtin types (Integer, Symbol,
// Nil, Boolean, Proc) likewise live in their own *_methods.go files.
// What remains here are Array / Hash / Range / String per-name bodies,
// plus the Class / Instance branches.
func callMethodLegacy(env *object.Environment, recv object.RubyObject, name string, args []object.RubyObject) (object.RubyObject, error) {

	// Class-receiver dispatch: `Foo.new`, `Foo.kind`, etc.
	if cls, ok := recv.(*object.Class); ok {
		if v, handled, err := callOnClass(env, cls, name, args); handled {
			return v, err
		}
	}

	// Instance-receiver dispatch: direct user methods and method_missing
	// already fired via Send at the top of callMethod. What's left here
	// is the Comparable / Enumerable derivations, then the final
	// NoMethodError.
	if inst, ok := recv.(*object.Instance); ok {
		if v, handled, err := comparableFromSpaceship(env, inst, name, args); handled {
			return v, err
		}
		if v, handled, err := callEnumerable(env, inst, name, args); handled {
			return v, err
		}
		return nil, errorf("evaluator: NoMethodError: undefined method `%s' for instance of %s", name, inst.C.Name)
	}

	switch r := recv.(type) {
	case *object.Array:
		switch name {
		case "length", "size", "count":
			if name == "count" && len(args) == 1 {
				n := 0
				for _, e := range r.Elements {
					if rubyEqual(e, args[0]) {
						n++
					}
				}
				return object.NewInteger(int64(n)), nil
			}
			return object.NewInteger(int64(len(r.Elements))), nil
		case "first":
			if len(args) == 1 {
				n, ok := args[0].(*object.Integer)
				if !ok {
					return nil, errorf("evaluator: Array#first(n) needs Integer")
				}
				cnt := int(n.Value)
				if cnt < 0 {
					return nil, errorf("evaluator: ArgumentError: negative array size")
				}
				if cnt > len(r.Elements) {
					cnt = len(r.Elements)
				}
				out := make([]object.RubyObject, cnt)
				copy(out, r.Elements[:cnt])
				return object.NewArray(out...), nil
			}
			if len(r.Elements) == 0 {
				return object.NIL, nil
			}
			return r.Elements[0], nil
		case "last":
			if len(args) == 1 {
				n, ok := args[0].(*object.Integer)
				if !ok {
					return nil, errorf("evaluator: Array#last(n) needs Integer")
				}
				cnt := int(n.Value)
				if cnt < 0 {
					return nil, errorf("evaluator: ArgumentError: negative array size")
				}
				if cnt > len(r.Elements) {
					cnt = len(r.Elements)
				}
				out := make([]object.RubyObject, cnt)
				copy(out, r.Elements[len(r.Elements)-cnt:])
				return object.NewArray(out...), nil
			}
			if len(r.Elements) == 0 {
				return object.NIL, nil
			}
			return r.Elements[len(r.Elements)-1], nil
		case "push", "append":
			r.Elements = append(r.Elements, args...)
			return r, nil
		case "pop":
			if len(r.Elements) == 0 {
				return object.NIL, nil
			}
			v := r.Elements[len(r.Elements)-1]
			r.Elements = r.Elements[:len(r.Elements)-1]
			return v, nil
		case "shift":
			if len(r.Elements) == 0 {
				return object.NIL, nil
			}
			v := r.Elements[0]
			r.Elements = r.Elements[1:]
			return v, nil
		case "unshift", "prepend":
			r.Elements = append(args, r.Elements...)
			return r, nil
		case "delete":
			if len(args) != 1 {
				return nil, errorf("evaluator: Array#delete expects 1 arg, got %d", len(args))
			}
			out := make([]object.RubyObject, 0, len(r.Elements))
			var removed object.RubyObject = object.NIL
			for _, e := range r.Elements {
				if rubyEqual(e, args[0]) {
					removed = e
					continue
				}
				out = append(out, e)
			}
			r.Elements = out
			return removed, nil
		case "delete_at":
			if len(args) != 1 {
				return nil, errorf("evaluator: Array#delete_at expects 1 arg, got %d", len(args))
			}
			idx, ok := args[0].(*object.Integer)
			if !ok {
				return nil, errorf("evaluator: Array#delete_at needs Integer")
			}
			i := int(idx.Value)
			if i < 0 {
				i += len(r.Elements)
			}
			if i < 0 || i >= len(r.Elements) {
				return object.NIL, nil
			}
			v := r.Elements[i]
			r.Elements = append(r.Elements[:i], r.Elements[i+1:]...)
			return v, nil
		case "concat":
			for _, a := range args {
				other, ok := a.(*object.Array)
				if !ok {
					return nil, errorf("evaluator: Array#concat needs Array args")
				}
				r.Elements = append(r.Elements, other.Elements...)
			}
			return r, nil
		case "reverse":
			out := make([]object.RubyObject, len(r.Elements))
			for i, v := range r.Elements {
				out[len(r.Elements)-1-i] = v
			}
			return object.NewArray(out...), nil
		case "sort":
			out := make([]object.RubyObject, len(r.Elements))
			copy(out, r.Elements)
			if err := sortArray(env, out); err != nil {
				return nil, err
			}
			return object.NewArray(out...), nil
		case "include?":
			if len(args) != 1 {
				return nil, errorf("evaluator: wrong number of arguments to Array#include? (given %d, expected 1)", len(args))
			}
			for _, v := range r.Elements {
				if rubyEqual(v, args[0]) {
					return object.TRUE, nil
				}
			}
			return object.FALSE, nil
		case "empty?":
			return object.BooleanOf(len(r.Elements) == 0), nil
		case "any?":
			for _, e := range r.Elements {
				if truthy(e) {
					return object.TRUE, nil
				}
			}
			return object.FALSE, nil
		case "all?":
			for _, e := range r.Elements {
				if !truthy(e) {
					return object.FALSE, nil
				}
			}
			return object.TRUE, nil
		case "none?":
			for _, e := range r.Elements {
				if truthy(e) {
					return object.FALSE, nil
				}
			}
			return object.TRUE, nil
		case "one?":
			n := 0
			for _, e := range r.Elements {
				if truthy(e) {
					n++
					if n > 1 {
						return object.FALSE, nil
					}
				}
			}
			return object.BooleanOf(n == 1), nil
		case "to_h":
			entries := make([]object.HashEntry, 0, len(r.Elements))
			for _, e := range r.Elements {
				pair, ok := e.(*object.Array)
				if !ok || len(pair.Elements) != 2 {
					return nil, errorf("evaluator: TypeError: wrong element type for to_h (expected 2-element Array)")
				}
				entries = append(entries, object.HashEntry{Key: pair.Elements[0], Value: pair.Elements[1]})
			}
			return object.NewHash(entries...), nil
		case "to_a":
			return r, nil
		case "dig":
			var cur object.RubyObject = r
			for _, k := range args {
				switch x := cur.(type) {
				case *object.Array:
					ki, ok := k.(*object.Integer)
					if !ok {
						return nil, errorf("evaluator: Array#dig needs Integer key")
					}
					idx := int(ki.Value)
					if idx < 0 {
						idx += len(x.Elements)
					}
					if idx < 0 || idx >= len(x.Elements) {
						return object.NIL, nil
					}
					cur = x.Elements[idx]
				case *object.Hash:
					var found object.RubyObject = object.NIL
					for _, e := range x.Entries {
						if rubyEqual(e.Key, k) {
							found = e.Value
							break
						}
					}
					cur = found
				default:
					return object.NIL, nil
				}
				if _, isNil := cur.(*object.Nil); isNil {
					return object.NIL, nil
				}
			}
			return cur, nil
		case "each_with_index":
			// No-block form: return an Array of [elem, index] pairs.
			// Block form is in callMethodWithBlockImpl.
			out := make([]object.RubyObject, len(r.Elements))
			for i, e := range r.Elements {
				out[i] = object.NewArray(e, object.NewInteger(int64(i)))
			}
			return object.NewArray(out...), nil
		case "each_slice":
			if len(args) != 1 {
				return nil, errorf("evaluator: Array#each_slice expects 1 arg")
			}
			n, ok := args[0].(*object.Integer)
			if !ok || n.Value <= 0 {
				return nil, errorf("evaluator: Array#each_slice needs positive Integer")
			}
			step := int(n.Value)
			out := []object.RubyObject{}
			for i := 0; i < len(r.Elements); i += step {
				end := i + step
				if end > len(r.Elements) {
					end = len(r.Elements)
				}
				slice := make([]object.RubyObject, end-i)
				copy(slice, r.Elements[i:end])
				out = append(out, object.NewArray(slice...))
			}
			return object.NewArray(out...), nil
		case "each_cons":
			if len(args) != 1 {
				return nil, errorf("evaluator: Array#each_cons expects 1 arg")
			}
			n, ok := args[0].(*object.Integer)
			if !ok || n.Value <= 0 {
				return nil, errorf("evaluator: Array#each_cons needs positive Integer")
			}
			w := int(n.Value)
			if w > len(r.Elements) {
				return object.NewArray(), nil
			}
			out := make([]object.RubyObject, 0, len(r.Elements)-w+1)
			for i := 0; i+w <= len(r.Elements); i++ {
				slice := make([]object.RubyObject, w)
				copy(slice, r.Elements[i:i+w])
				out = append(out, object.NewArray(slice...))
			}
			return object.NewArray(out...), nil
		case "zip":
			out := make([]object.RubyObject, len(r.Elements))
			others := make([]*object.Array, 0, len(args))
			for _, a := range args {
				oa, ok := a.(*object.Array)
				if !ok {
					return nil, errorf("evaluator: Array#zip needs Array args, got %T", a)
				}
				others = append(others, oa)
			}
			for i, e := range r.Elements {
				tuple := make([]object.RubyObject, 1+len(others))
				tuple[0] = e
				for j, o := range others {
					if i < len(o.Elements) {
						tuple[j+1] = o.Elements[i]
					} else {
						tuple[j+1] = object.NIL
					}
				}
				out[i] = object.NewArray(tuple...)
			}
			return object.NewArray(out...), nil
		case "take":
			if len(args) != 1 {
				return nil, errorf("evaluator: Array#take expects 1 arg")
			}
			n, ok := args[0].(*object.Integer)
			if !ok {
				return nil, errorf("evaluator: Array#take needs Integer")
			}
			cnt := int(n.Value)
			if cnt < 0 {
				return nil, errorf("evaluator: ArgumentError: negative array size")
			}
			if cnt > len(r.Elements) {
				cnt = len(r.Elements)
			}
			out := make([]object.RubyObject, cnt)
			copy(out, r.Elements[:cnt])
			return object.NewArray(out...), nil
		case "drop":
			if len(args) != 1 {
				return nil, errorf("evaluator: Array#drop expects 1 arg")
			}
			n, ok := args[0].(*object.Integer)
			if !ok {
				return nil, errorf("evaluator: Array#drop needs Integer")
			}
			cnt := int(n.Value)
			if cnt < 0 {
				return nil, errorf("evaluator: ArgumentError: negative array size")
			}
			if cnt > len(r.Elements) {
				cnt = len(r.Elements)
			}
			out := make([]object.RubyObject, len(r.Elements)-cnt)
			copy(out, r.Elements[cnt:])
			return object.NewArray(out...), nil
		case "join":
			sep := ""
			if len(args) == 1 {
				if t, ok := stringText(env, args[0]); ok {
					sep = t
				}
			}
			parts := make([]string, len(r.Elements))
			for i, e := range r.Elements {
				parts[i] = toStringValue(env, e)
			}
			return object.NewString(strings.Join(parts, sep)), nil
		case "min":
			if len(r.Elements) == 0 {
				return object.NIL, nil
			}
			best := r.Elements[0]
			for _, e := range r.Elements[1:] {
				c, ok := compareObjectsEnv(env, e, best)
				if !ok {
					return nil, errorf("evaluator: comparison failed in Array#min")
				}
				if c < 0 {
					best = e
				}
			}
			return best, nil
		case "minmax":
			if len(r.Elements) == 0 {
				return object.NewArray(object.NIL, object.NIL), nil
			}
			lo := r.Elements[0]
			hi := r.Elements[0]
			for _, e := range r.Elements[1:] {
				if c, ok := compareObjectsEnv(env, e, lo); ok && c < 0 {
					lo = e
				}
				if c, ok := compareObjectsEnv(env, e, hi); ok && c > 0 {
					hi = e
				}
			}
			return object.NewArray(lo, hi), nil
		case "max":
			if len(r.Elements) == 0 {
				return object.NIL, nil
			}
			best := r.Elements[0]
			for _, e := range r.Elements[1:] {
				c, ok := compareObjectsEnv(env, e, best)
				if !ok {
					return nil, errorf("evaluator: comparison failed in Array#max")
				}
				if c > 0 {
					best = e
				}
			}
			return best, nil
		case "sum":
			var intSum int64
			var floatSum float64
			anyFloat := false
			for _, e := range r.Elements {
				switch v := e.(type) {
				case *object.Integer:
					if anyFloat {
						floatSum += float64(v.Value)
					} else {
						intSum += v.Value
					}
				case *object.Float:
					if !anyFloat {
						floatSum = float64(intSum)
						anyFloat = true
					}
					floatSum += v.Value
				default:
					return nil, errorf("evaluator: Array#sum: non-numeric element %T not supported", e)
				}
			}
			if anyFloat {
				return object.NewFloat(floatSum), nil
			}
			return object.NewInteger(intSum), nil
		case "grep":
			if len(args) != 1 {
				return nil, errorf("evaluator: Array#grep expects 1 arg, got %d", len(args))
			}
			pattern := args[0]
			out := []object.RubyObject{}
			for _, e := range r.Elements {
				// Use `===` on the pattern for primitives we know, or
				// dispatch the pattern's `===` method for instances /
				// modules. `caseEqual` already does this for built-in
				// patterns; for user-defined `===` we call directly.
				if inst, ok := pattern.(*object.Instance); ok {
					if _, found := dispatchClass(env, inst).LookupMethod("==="); found {
						v, err := callMethod(env, pattern, "===", []object.RubyObject{e})
						if err != nil {
							return nil, err
						}
						if truthy(v) {
							out = append(out, e)
						}
						continue
					}
				}
				if caseEqual(env, pattern, e) {
					out = append(out, e)
				}
			}
			return object.NewArray(out...), nil
		case "uniq":
			out := []object.RubyObject{}
		dedup:
			for _, e := range r.Elements {
				for _, k := range out {
					if rubyEqual(e, k) {
						continue dedup
					}
				}
				out = append(out, e)
			}
			return object.NewArray(out...), nil
		case "compact":
			out := []object.RubyObject{}
			for _, e := range r.Elements {
				if _, isNil := e.(*object.Nil); !isNil {
					out = append(out, e)
				}
			}
			return object.NewArray(out...), nil
		case "flatten":
			depth := -1
			if len(args) == 1 {
				d, ok := args[0].(*object.Integer)
				if !ok {
					return nil, errorf("evaluator: Array#flatten depth must be Integer")
				}
				depth = int(d.Value)
			}
			return object.NewArray(flattenArrayDepth(r, depth)...), nil
		case "reduce", "inject":
			// No-block forms: `reduce(:+)` and `reduce(init, :+)`.
			// Block forms are handled in callMethodWithBlock.
			var op string
			var acc object.RubyObject
			start := 0
			switch {
			case len(args) == 1:
				sym, ok := args[0].(*object.Symbol)
				if !ok {
					return nil, errorf("evaluator: Array#reduce(sym) needs Symbol")
				}
				op = env.Symbols().Name(sym.ID)
				if len(r.Elements) == 0 {
					return object.NIL, nil
				}
				acc = r.Elements[0]
				start = 1
			case len(args) == 2:
				sym, ok := args[1].(*object.Symbol)
				if !ok {
					return nil, errorf("evaluator: Array#reduce(init, sym) needs Symbol")
				}
				op = env.Symbols().Name(sym.ID)
				acc = args[0]
			default:
				return nil, errorf("evaluator: Array#reduce: wrong number of arguments (%d)", len(args))
			}
			for i := start; i < len(r.Elements); i++ {
				v, err := evalBinaryOp(env, op, acc, r.Elements[i])
				if err != nil {
					return nil, err
				}
				acc = v
			}
			return acc, nil
		case "tally":
			entries := []object.HashEntry{}
			for _, e := range r.Elements {
				found := false
				for i := range entries {
					if rubyEqual(entries[i].Key, e) {
						v := entries[i].Value.(*object.Integer)
						entries[i].Value = object.NewInteger(v.Value + 1)
						found = true
						break
					}
				}
				if !found {
					entries = append(entries, object.HashEntry{Key: e, Value: object.NewInteger(1)})
				}
			}
			return object.NewHash(entries...), nil
		}
	case *object.Hash:
		switch name {
		case "length", "size":
			return object.NewInteger(int64(len(r.Entries))), nil
		case "keys":
			ks := make([]object.RubyObject, 0, len(r.Entries))
			for _, e := range r.Entries {
				ks = append(ks, e.Key)
			}
			return object.NewArray(ks...), nil
		case "values":
			vs := make([]object.RubyObject, 0, len(r.Entries))
			for _, e := range r.Entries {
				vs = append(vs, e.Value)
			}
			return object.NewArray(vs...), nil
		case "merge":
			out := make([]object.HashEntry, len(r.Entries))
			copy(out, r.Entries)
			for _, a := range args {
				other, ok := a.(*object.Hash)
				if !ok {
					return nil, errorf("evaluator: Hash#merge needs Hash args")
				}
				for _, e := range other.Entries {
					replaced := false
					for i := range out {
						if rubyEqual(out[i].Key, e.Key) {
							out[i].Value = e.Value
							replaced = true
							break
						}
					}
					if !replaced {
						out = append(out, e)
					}
				}
			}
			return object.NewHash(out...), nil
		case "delete":
			if len(args) != 1 {
				return nil, errorf("evaluator: Hash#delete expects 1 arg, got %d", len(args))
			}
			for i, e := range r.Entries {
				if rubyEqual(e.Key, args[0]) {
					r.Entries = append(r.Entries[:i], r.Entries[i+1:]...)
					return e.Value, nil
				}
			}
			return object.NIL, nil
		case "store":
			if len(args) != 2 {
				return nil, errorf("evaluator: Hash#store expects 2 args, got %d", len(args))
			}
			for i := range r.Entries {
				if rubyEqual(r.Entries[i].Key, args[0]) {
					r.Entries[i].Value = args[1]
					return args[1], nil
				}
			}
			r.Entries = append(r.Entries, object.HashEntry{Key: args[0], Value: args[1]})
			return args[1], nil
		case "to_a":
			out := make([]object.RubyObject, len(r.Entries))
			for i, e := range r.Entries {
				out[i] = object.NewArray(e.Key, e.Value)
			}
			return object.NewArray(out...), nil
		case "empty?":
			return object.BooleanOf(len(r.Entries) == 0), nil
		case "any?":
			return object.BooleanOf(len(r.Entries) > 0), nil
		case "fetch":
			if len(args) < 1 || len(args) > 2 {
				return nil, errorf("evaluator: Hash#fetch expects 1..2 args, got %d", len(args))
			}
			for _, e := range r.Entries {
				if rubyEqual(e.Key, args[0]) {
					return e.Value, nil
				}
			}
			if len(args) == 2 {
				return args[1], nil
			}
			return raiseBuiltin(env, "KeyError", "key not found")
		case "dig":
			var cur object.RubyObject = r
			for _, k := range args {
				switch x := cur.(type) {
				case *object.Hash:
					var found object.RubyObject = object.NIL
					for _, e := range x.Entries {
						if rubyEqual(e.Key, k) {
							found = e.Value
							break
						}
					}
					cur = found
				case *object.Array:
					ki, ok := k.(*object.Integer)
					if !ok {
						return nil, errorf("evaluator: Array#dig needs Integer key")
					}
					idx := int(ki.Value)
					if idx < 0 {
						idx += len(x.Elements)
					}
					if idx < 0 || idx >= len(x.Elements) {
						return object.NIL, nil
					}
					cur = x.Elements[idx]
				default:
					return object.NIL, nil
				}
				if _, isNil := cur.(*object.Nil); isNil {
					return object.NIL, nil
				}
			}
			return cur, nil
		case "sort":
			// Sort by key ascending; returns Array of [k, v] pairs.
			out := make([]object.HashEntry, len(r.Entries))
			copy(out, r.Entries)
			sortStable(len(out), func(i, j int) bool {
				c, _ := compareObjectsEnv(env, out[i].Key, out[j].Key)
				return c < 0
			}, func(i, j int) {
				out[i], out[j] = out[j], out[i]
			})
			pairs := make([]object.RubyObject, len(out))
			for i, e := range out {
				pairs[i] = object.NewArray(e.Key, e.Value)
			}
			return object.NewArray(pairs...), nil
		case "min", "max":
			// Hash#min / #max return [k, v] of min/max key.
			if len(r.Entries) == 0 {
				return object.NIL, nil
			}
			best := r.Entries[0]
			for _, e := range r.Entries[1:] {
				c, ok := compareObjectsEnv(env, e.Key, best.Key)
				if !ok {
					return nil, errorf("evaluator: Hash#%s comparison failed", name)
				}
				if (name == "min" && c < 0) || (name == "max" && c > 0) {
					best = e
				}
			}
			return object.NewArray(best.Key, best.Value), nil
		case "invert":
			out := make([]object.HashEntry, len(r.Entries))
			for i, e := range r.Entries {
				out[i] = object.HashEntry{Key: e.Value, Value: e.Key}
			}
			return object.NewHash(out...), nil
		case "except":
			out := []object.HashEntry{}
		nextEntry:
			for _, e := range r.Entries {
				for _, k := range args {
					if rubyEqual(e.Key, k) {
						continue nextEntry
					}
				}
				out = append(out, e)
			}
			return object.NewHash(out...), nil
		case "has_key?", "key?", "include?", "member?":
			if len(args) != 1 {
				return nil, errorf("evaluator: wrong number of arguments to Hash#%s (given %d, expected 1)", name, len(args))
			}
			for _, e := range r.Entries {
				if rubyEqual(e.Key, args[0]) {
					return object.TRUE, nil
				}
			}
			return object.FALSE, nil
		}
	}
	return nil, errorf("evaluator: NoMethodError: undefined method `%s' for %T", name, recv)
}
