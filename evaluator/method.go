package evaluator

import (
	"sort"
	"strconv"
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
		_, found := inst.C.LookupMethod(name)
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
		if _, found := inst.C.LookupMethod("<=>"); found {
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
					if m, found := inst.C.LookupMethod(n.Function.Value); found {
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

// callMethod is the minimal hand-rolled dispatcher used until the class
// machinery lands. Hardcoded for the methods the literals corpus needs.
func callMethod(env *object.Environment, recv object.RubyObject, name string, args []object.RubyObject) (object.RubyObject, error) {

	// Universal Object methods first.
	switch name {
	case "nil?":
		_, isNil := recv.(*object.Nil)
		return object.BooleanOf(isNil), nil
	case "class":
		return classOf(env, recv), nil
	case "inspect":
		return object.NewString(env.Inspect(recv)), nil
	case "itself":
		return recv, nil
	case "instance_variable_get":
		if len(args) != 1 {
			return nil, errorf("evaluator: instance_variable_get expects 1 arg")
		}
		name, ok := symbolOrString(env, args[0])
		if !ok {
			return nil, errorf("evaluator: instance_variable_get: name must be Symbol or String")
		}
		inst, ok := recv.(*object.Instance)
		if !ok {
			return object.NIL, nil
		}
		key := name
		if len(key) == 0 || key[0] != '@' {
			key = "@" + key
		}
		if v, ok := inst.Ivars[key]; ok {
			return v, nil
		}
		return object.NIL, nil
	case "instance_variable_set":
		if len(args) != 2 {
			return nil, errorf("evaluator: instance_variable_set expects 2 args")
		}
		name, ok := symbolOrString(env, args[0])
		if !ok {
			return nil, errorf("evaluator: instance_variable_set: name must be Symbol or String")
		}
		inst, ok := recv.(*object.Instance)
		if !ok {
			return nil, errorf("evaluator: instance_variable_set: receiver must be an instance, got %T", recv)
		}
		key := name
		if len(key) == 0 || key[0] != '@' {
			key = "@" + key
		}
		inst.Ivars[key] = args[1]
		return args[1], nil
	case "instance_variables":
		inst, ok := recv.(*object.Instance)
		if !ok {
			return object.NewArray(), nil
		}
		out := make([]object.RubyObject, 0, len(inst.Ivars))
		for k := range inst.Ivars {
			out = append(out, env.Symbols().Intern(k))
		}
		return object.NewArray(out...), nil
	case "frozen?":
		// We don't track freezing yet; symbols / integers / nil / bool
		// are conceptually frozen and others report false.
		switch recv.(type) {
		case *object.Symbol, *object.Integer, *object.Float, *object.Nil, *object.Boolean:
			return object.TRUE, nil
		}
		return object.FALSE, nil
	case "dup", "clone":
		return recv, nil
	case "tap":
		// without block: identity
		return recv, nil
	case "equal?":
		if len(args) != 1 {
			return nil, errorf("evaluator: equal? expects 1 arg")
		}
		// Identity comparison: same Go pointer.
		return object.BooleanOf(recv == args[0]), nil
	case "eql?":
		// MRI Object#eql? defaults to identity; numerics override to
		// type-strict equality. We approximate with strict typed
		// equality which is what every test case in scope needs.
		if len(args) != 1 {
			return nil, errorf("evaluator: eql? expects 1 arg")
		}
		return object.BooleanOf(rubyEqual(recv, args[0])), nil
	case "send", "__send__", "public_send":
		if len(args) < 1 {
			return nil, errorf("evaluator: send needs a method name")
		}
		mname, ok := symbolOrString(env, args[0])
		if !ok {
			return nil, errorf("evaluator: send: method name must be Symbol or String")
		}
		return callMethod(env, recv, mname, args[1:])
	case "method":
		// Returns a callable bound to (recv, sym). We approximate with
		// a Proc that calls back.
		if len(args) != 1 {
			return nil, errorf("evaluator: Object#method expects 1 arg")
		}
		mname, ok := symbolOrString(env, args[0])
		if !ok {
			return nil, errorf("evaluator: Object#method: name must be Symbol or String")
		}
		return procFromBound(env, recv, mname), nil
	case "respond_to?":
		if len(args) != 1 {
			return nil, errorf("evaluator: wrong number of arguments to respond_to? (given %d, expected 1)", len(args))
		}
		mname, ok := symbolOrString(env, args[0])
		if !ok {
			return nil, errorf("evaluator: respond_to? needs Symbol or String, got %T", args[0])
		}
		return object.BooleanOf(receiverResponds(env, recv, mname)), nil
	case "is_a?", "kind_of?", "instance_of?":
		if len(args) != 1 {
			return nil, errorf("evaluator: wrong number of arguments to Object#%s (given %d, expected 1)", name, len(args))
		}
		target, ok := args[0].(*object.Class)
		if !ok {
			return nil, errorf("evaluator: TypeError: class or module required")
		}
		c := classOfRaw(env, recv)
		if c == nil {
			return object.FALSE, nil
		}
		if name == "instance_of?" {
			return object.BooleanOf(c == target), nil
		}
		return object.BooleanOf(c.IsAncestor(target)), nil
	}

	// Proc-receiver dispatch: `.call` / `.()` (parser rewrites `.()` to
	// `.call`) / `.yield`.
	if p, ok := recv.(*object.Proc); ok {
		switch name {
		case "call", "yield", "()", "[]":
			return invokeProc(env, p, args)
		case "lambda?":
			return object.BooleanOf(p.IsLambda), nil
		case "arity":
			return procArity(p), nil
		}
	}

	// Class-receiver dispatch: `Foo.new`, `Foo.kind`, etc.
	if cls, ok := recv.(*object.Class); ok {
		if v, handled, err := callOnClass(env, cls, name, args); handled {
			return v, err
		}
	}

	// Instance-receiver dispatch: walk the inheritance chain for an
	// instance method.
	if inst, ok := recv.(*object.Instance); ok {
		if m, found := inst.C.LookupMethod(name); found {
			if um, ok := m.(*object.UserMethod); ok {
				return invokeMethodOn(env, inst, um, args, nil)
			}
		}
		// Comparable-style ops: derive from <=> when defined.
		if v, handled, err := comparableFromSpaceship(env, inst, name, args); handled {
			return v, err
		}
		// Enumerable: derive map/select/etc. from `each` when the class
		// includes Enumerable.
		if v, handled, err := callEnumerable(env, inst, name, args); handled {
			return v, err
		}
		// method_missing fallback: dispatches a synthetic call with
		// the original method name prepended as a Symbol.
		if mm, found := inst.C.LookupMethod("method_missing"); found {
			if um, ok := mm.(*object.UserMethod); ok {
				mmArgs := make([]object.RubyObject, 0, 1+len(args))
				mmArgs = append(mmArgs, env.Symbols().Intern(name))
				mmArgs = append(mmArgs, args...)
				return invokeMethodOn(env, inst, um, mmArgs, nil)
			}
		}
		return nil, errorf("evaluator: NoMethodError: undefined method `%s' for instance of %s", name, inst.C.Name)
	}

	if v, ok, err := callStringMethod(env, recv, name, args); ok {
		return v, err
	}

	if r, ok := recv.(*object.Range); ok {
		switch name {
		case "to_a":
			elems, err := rangeToSlice(env, r)
			if err != nil {
				return nil, err
			}
			return object.NewArray(elems...), nil
		case "size", "count", "length":
			lo, hi, ok := rangeIntegerBounds(r)
			if !ok {
				return nil, errorf("evaluator: Range#%s needs Integer bounds", name)
			}
			n := hi - lo
			if !r.Exclusive {
				n++
			}
			if n < 0 {
				n = 0
			}
			return object.NewInteger(n), nil
		case "first":
			return r.Begin, nil
		case "last":
			return r.End, nil
		case "min":
			return r.Begin, nil
		case "max":
			i, ok := r.End.(*object.Integer)
			if !ok {
				return r.End, nil
			}
			if r.Exclusive {
				return object.NewInteger(i.Value - 1), nil
			}
			return r.End, nil
		case "sum":
			lo, hi, ok := rangeIntegerBounds(r)
			if !ok {
				return nil, errorf("evaluator: Range#sum needs Integer bounds")
			}
			end := hi
			if !r.Exclusive {
				end = hi + 1
			}
			n := end - lo
			if n <= 0 {
				return object.NewInteger(0), nil
			}
			// arithmetic series: n*(lo+last)/2 where last = end-1
			s := n * (lo + (end - 1)) / 2
			return object.NewInteger(s), nil
		case "reduce", "inject", "min_by", "max_by", "sort", "sort_by", "tally", "uniq", "each_slice", "each_cons", "each_with_index", "zip", "take", "drop", "join", "flatten":
			elems, err := rangeToSlice(env, r)
			if err != nil {
				return nil, err
			}
			return callMethod(env, object.NewArray(elems...), name, args)
		case "include?", "cover?":
			if len(args) != 1 {
				return nil, errorf("evaluator: wrong number of arguments to Range#%s", name)
			}
			lo, hi, ok := rangeIntegerBounds(r)
			if !ok {
				return nil, errorf("evaluator: Range#%s needs Integer bounds", name)
			}
			vi, ok := args[0].(*object.Integer)
			if !ok {
				return object.FALSE, nil
			}
			if vi.Value < lo {
				return object.FALSE, nil
			}
			if r.Exclusive {
				return object.BooleanOf(vi.Value < hi), nil
			}
			return object.BooleanOf(vi.Value <= hi), nil
		}
	}

	switch r := recv.(type) {
	case *object.Integer:
		switch name {
		case "even?":
			return object.BooleanOf(r.Value%2 == 0), nil
		case "odd?":
			return object.BooleanOf(r.Value%2 != 0), nil
		case "zero?":
			return object.BooleanOf(r.Value == 0), nil
		case "to_s":
			base := 10
			if len(args) == 1 {
				b, ok := args[0].(*object.Integer)
				if !ok {
					return nil, errorf("evaluator: Integer#to_s base must be Integer")
				}
				base = int(b.Value)
				if base < 2 || base > 36 {
					return nil, errorf("evaluator: ArgumentError: invalid radix %d", base)
				}
			}
			return object.NewString(strconv.FormatInt(r.Value, base)), nil
		case "to_i", "to_int":
			return r, nil
		case "to_f":
			return object.NewFloat(float64(r.Value)), nil
		case "to_r":
			// Rational not modelled; return Float as the next-best
			// approximation that won't crash typical code paths.
			return object.NewFloat(float64(r.Value)), nil
		case "abs":
			if r.Value < 0 {
				return object.NewInteger(-r.Value), nil
			}
			return r, nil
		case "chr":
			return object.NewString(string(rune(r.Value))), nil
		case "clamp":
			// Accept either two bounds (`5.clamp(1, 10)`) or a Range
			// (`5.clamp(1..10)`, ruby 2.7+).
			var lo, hi int64
			switch len(args) {
			case 1:
				rng, ok := args[0].(*object.Range)
				if !ok {
					return nil, errorf("evaluator: Integer#clamp expects Range or 2 ints")
				}
				blo, bhi, ok := rangeIntegerBounds(rng)
				if !ok {
					return nil, errorf("evaluator: Integer#clamp Range needs Integer bounds")
				}
				lo = blo
				hi = bhi
				if rng.Exclusive {
					hi--
				}
			case 2:
				lov, ok1 := args[0].(*object.Integer)
				hiv, ok2 := args[1].(*object.Integer)
				if !ok1 || !ok2 {
					return nil, errorf("evaluator: Integer#clamp needs Integer bounds")
				}
				lo = lov.Value
				hi = hiv.Value
			default:
				return nil, errorf("evaluator: Integer#clamp expects 1..2 args, got %d", len(args))
			}
			v := r.Value
			switch {
			case v < lo:
				v = lo
			case v > hi:
				v = hi
			}
			return object.NewInteger(v), nil
		case "between?":
			if len(args) != 2 {
				return nil, errorf("evaluator: Integer#between? expects 2 args, got %d", len(args))
			}
			lo, ok1 := args[0].(*object.Integer)
			hi, ok2 := args[1].(*object.Integer)
			if !ok1 || !ok2 {
				return nil, errorf("evaluator: Integer#between? needs Integer bounds")
			}
			return object.BooleanOf(r.Value >= lo.Value && r.Value <= hi.Value), nil
		case "divmod":
			if len(args) != 1 {
				return nil, errorf("evaluator: Integer#divmod expects 1 arg")
			}
			d, ok := args[0].(*object.Integer)
			if !ok {
				return nil, errorf("evaluator: Integer#divmod needs Integer")
			}
			if d.Value == 0 {
				return raiseBuiltin(env, "ZeroDivisionError", "divided by 0")
			}
			q := r.Value / d.Value
			if r.Value%d.Value != 0 && (r.Value < 0) != (d.Value < 0) {
				q--
			}
			m := r.Value - q*d.Value
			return object.NewArray(object.NewInteger(q), object.NewInteger(m)), nil
		case "modulo", "%":
			if len(args) != 1 {
				return nil, errorf("evaluator: Integer#modulo expects 1 arg")
			}
			d, ok := args[0].(*object.Integer)
			if !ok {
				return nil, errorf("evaluator: Integer#modulo needs Integer")
			}
			if d.Value == 0 {
				return raiseBuiltin(env, "ZeroDivisionError", "divided by 0")
			}
			m := r.Value % d.Value
			if m != 0 && ((m < 0) != (d.Value < 0)) {
				m += d.Value
			}
			return object.NewInteger(m), nil
		case "fdiv":
			if len(args) != 1 {
				return nil, errorf("evaluator: Integer#fdiv expects 1 arg")
			}
			df, err := toFloatValue(args[0])
			if err != nil {
				return nil, err
			}
			return object.NewFloat(float64(r.Value) / df), nil
		case "remainder":
			if len(args) != 1 {
				return nil, errorf("evaluator: Integer#remainder expects 1 arg")
			}
			d, ok := args[0].(*object.Integer)
			if !ok {
				return nil, errorf("evaluator: Integer#remainder needs Integer")
			}
			if d.Value == 0 {
				return raiseBuiltin(env, "ZeroDivisionError", "divided by 0")
			}
			return object.NewInteger(r.Value % d.Value), nil
		case "gcd":
			if len(args) != 1 {
				return nil, errorf("evaluator: Integer#gcd expects 1 arg")
			}
			d, ok := args[0].(*object.Integer)
			if !ok {
				return nil, errorf("evaluator: Integer#gcd needs Integer")
			}
			a, b := r.Value, d.Value
			if a < 0 {
				a = -a
			}
			if b < 0 {
				b = -b
			}
			for b != 0 {
				a, b = b, a%b
			}
			return object.NewInteger(a), nil
		case "succ", "next":
			return object.NewInteger(r.Value + 1), nil
		case "pred":
			return object.NewInteger(r.Value - 1), nil
		case "negative?":
			return object.BooleanOf(r.Value < 0), nil
		case "positive?":
			return object.BooleanOf(r.Value > 0), nil
		case "bit_length":
			v := r.Value
			if v < 0 {
				v = ^v
			}
			n := int64(0)
			for v > 0 {
				n++
				v >>= 1
			}
			return object.NewInteger(n), nil
		case "digits":
			base := int64(10)
			if len(args) == 1 {
				b, ok := args[0].(*object.Integer)
				if !ok {
					return nil, errorf("evaluator: TypeError: Integer#digits base must be Integer")
				}
				base = b.Value
			}
			if base < 2 {
				return nil, errorf("evaluator: ArgumentError: invalid digits base %d", base)
			}
			if r.Value < 0 {
				return nil, errorf("evaluator: Math::DomainError: out of domain")
			}
			if r.Value == 0 {
				return object.NewArray(object.NewInteger(0)), nil
			}
			out := []object.RubyObject{}
			n := r.Value
			for n > 0 {
				out = append(out, object.NewInteger(n%base))
				n /= base
			}
			return object.NewArray(out...), nil
		}
	case *object.Boolean:
		if name == "to_s" || name == "inspect" {
			if r.Value {
				return object.NewString("true"), nil
			}
			return object.NewString("false"), nil
		}
		if name == "&" {
			if len(args) != 1 {
				return nil, errorf("evaluator: Boolean#& expects 1 arg")
			}
			return object.BooleanOf(r.Value && truthy(args[0])), nil
		}
		if name == "|" {
			if len(args) != 1 {
				return nil, errorf("evaluator: Boolean#| expects 1 arg")
			}
			return object.BooleanOf(r.Value || truthy(args[0])), nil
		}
		if name == "^" {
			if len(args) != 1 {
				return nil, errorf("evaluator: Boolean#^ expects 1 arg")
			}
			return object.BooleanOf(r.Value != truthy(args[0])), nil
		}
	case *object.Nil:
		if name == "to_s" {
			return object.NewString(""), nil
		}
		if name == "to_a" {
			return object.NewArray(), nil
		}
		if name == "to_h" {
			return object.NewHash(), nil
		}
		if name == "inspect" {
			return object.NewString("nil"), nil
		}
		_ = r
	case *object.Symbol:
		switch name {
		case "to_s", "id2name":
			return object.NewString(env.Symbols().Name(r.ID)), nil
		case "to_sym":
			return r, nil
		case "to_proc":
			return procFromSymbol(env.Symbols().Name(r.ID)), nil
		case "length", "size":
			return object.NewInteger(int64(len(env.Symbols().Name(r.ID)))), nil
		case "upcase":
			n := env.Symbols().Name(r.ID)
			return env.Symbols().Intern(strings.ToUpper(n)), nil
		case "downcase":
			n := env.Symbols().Name(r.ID)
			return env.Symbols().Intern(strings.ToLower(n)), nil
		}
	case *object.Float:
		switch name {
		case "abs":
			if r.Value < 0 {
				return object.NewFloat(-r.Value), nil
			}
			return r, nil
		case "to_i", "to_int", "truncate":
			return object.NewInteger(int64(r.Value)), nil
		case "to_f":
			return r, nil
		case "to_s":
			return object.NewString(env.Inspect(r)), nil
		case "negative?":
			return object.BooleanOf(r.Value < 0), nil
		case "positive?":
			return object.BooleanOf(r.Value > 0), nil
		case "zero?":
			return object.BooleanOf(r.Value == 0), nil
		case "nan?":
			return object.BooleanOf(r.Value != r.Value), nil
		case "infinite?":
			switch {
			case r.Value > 1e308:
				return object.NewInteger(1), nil
			case r.Value < -1e308:
				return object.NewInteger(-1), nil
			}
			return object.NIL, nil
		case "floor":
			return object.NewInteger(int64(floorFloat(r.Value))), nil
		case "ceil":
			return object.NewInteger(int64(ceilFloat(r.Value))), nil
		case "round":
			if len(args) == 1 {
				digits, ok := args[0].(*object.Integer)
				if !ok {
					return nil, errorf("evaluator: Float#round digits must be Integer")
				}
				p := 1.0
				for i := int64(0); i < digits.Value; i++ {
					p *= 10
				}
				return object.NewFloat(roundFloat(r.Value*p) / p), nil
			}
			return object.NewInteger(int64(roundFloat(r.Value))), nil
		case "clamp":
			if len(args) == 1 {
				rng, ok := args[0].(*object.Range)
				if !ok {
					return nil, errorf("evaluator: Float#clamp needs Range or 2 args")
				}
				lo, _ := toFloatValue(rng.Begin)
				hi, _ := toFloatValue(rng.End)
				if rng.Exclusive {
					hi -= 1e-12
				}
				v := r.Value
				if v < lo {
					v = lo
				}
				if v > hi {
					v = hi
				}
				return object.NewFloat(v), nil
			}
			if len(args) != 2 {
				return nil, errorf("evaluator: Float#clamp expects 1..2 args")
			}
			lo, err := toFloatValue(args[0])
			if err != nil {
				return nil, err
			}
			hi, err := toFloatValue(args[1])
			if err != nil {
				return nil, err
			}
			v := r.Value
			if v < lo {
				v = lo
			}
			if v > hi {
				v = hi
			}
			return object.NewFloat(v), nil
		}
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
					if _, found := inst.C.LookupMethod("==="); found {
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
