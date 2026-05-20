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

// callMethodLegacy is the residual dispatch fallback after every
// builtin's method set has migrated onto its class. What remains here
// is the Class-receiver bridge (callOnClass) and the Instance
// Comparable / Enumerable derivations -- both branches Send can't
// express until those derivations themselves move onto Instance's
// class chain. Returns NoMethodError on miss.
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

	return nil, errorf("evaluator: NoMethodError: undefined method `%s' for %T", name, recv)
}
