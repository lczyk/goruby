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
	if v, handled, err := numericInfix(env, op, left, right); handled {
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
		if _, found := inst.SingletonMethods[name]; found {
			return true
		}
		if inst.SingletonClass != nil {
			if _, found := inst.SingletonClass.LookupMethod(name); found {
				return true
			}
		}
		if _, found := dispatchClass(env, inst).LookupMethod(name); found {
			return true
		}
	}
	if cls, ok := recv.(*object.Class); ok {
		if _, found := cls.LookupClassMethod(name); found {
			return true
		}
		// Class object: also consult the Class's own class
		// (ClassClass / ModuleClass) for inherited instance methods --
		// catches `new`, `superclass`, `name`, etc. now living there.
		if mc := cls.Class(); mc != nil {
			if _, found := mc.LookupMethod(name); found {
				return true
			}
		}
	}
	// Consult respond_to_missing? if defined -- the canonical way to
	// extend respond_to? for method_missing-driven dispatch. Pass
	// (name, include_private=false) per MRI; rake's mock objects rely
	// on this to gate respond_to? before exercising method_missing.
	if inst, ok := recv.(*object.Instance); ok {
		var rtm object.RubyMethod
		var foundRTM bool
		if inst.SingletonMethods != nil {
			rtm, foundRTM = inst.SingletonMethods["respond_to_missing?"]
		}
		if !foundRTM {
			rtm, foundRTM = dispatchClass(env, inst).LookupMethod("respond_to_missing?")
		}
		if foundRTM {
			args := []object.RubyObject{env.Symbols().Intern(name), object.FALSE}
			var v object.RubyObject
			var err error
			switch mm := rtm.(type) {
			case *object.UserMethod:
				v, err = invokeMethodOn(env, inst, mm, args, nil)
			case *object.BuiltinMethod:
				v, err = mm.Fn(env, inst, args, nil)
			}
			if err == nil {
				return truthy(v)
			}
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
			_, sortErr = raiseBuiltin(env, "ArgumentError",
				"comparison of "+classNameOf(xs[i])+" with "+classNameOf(xs[j])+" failed")
			return false
		}
		return c < 0
	})
	return sortErr
}

// classNameOf returns the ruby-visible class name of obj for error
// messages. Mirrors MRI's "comparison of X with Y" formatting.
func classNameOf(obj object.RubyObject) string {
	if inst, ok := obj.(*object.Instance); ok && inst.C != nil {
		return inst.C.Name
	}
	switch obj.(type) {
	case *object.Integer:
		return "Integer"
	case *object.Float:
		return "Float"
	case *object.String:
		return "String"
	case *object.Symbol:
		return "Symbol"
	case *object.Array:
		return "Array"
	case *object.Hash:
		return "Hash"
	case *object.Nil:
		return "NilClass"
	case *object.Boolean:
		return "TrueClass"
	}
	return "Object"
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
		if x.IsBig() || y.IsBig() {
			return x.ToBig().Cmp(y.ToBig()), true
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
	case *object.Array:
		y, ok := b.(*object.Array)
		if !ok {
			return 0, false
		}
		// Lexicographic comparison: walk element-by-element, return on
		// first non-equal pair; shorter array wins ties on prefix.
		n := len(x.Elements)
		if len(y.Elements) < n {
			n = len(y.Elements)
		}
		for i := 0; i < n; i++ {
			c, ok := compareObjects(x.Elements[i], y.Elements[i])
			if !ok {
				return 0, false
			}
			if c != 0 {
				return c, true
			}
		}
		switch {
		case len(x.Elements) < len(y.Elements):
			return -1, true
		case len(x.Elements) > len(y.Elements):
			return 1, true
		}
		return 0, true
	}
	return 0, false
}

// compareObjectsEnv extends compareObjects with Symbol-by-name and
// user `<=>` dispatch for Instance receivers.
func compareObjectsEnv(env *object.Environment, a, b object.RubyObject) (int, bool) {
	// Array case handled here (not in compareObjects) so element
	// recursion can see env-aware extensions (Symbol-by-name, user
	// <=>). The env-less compareObjects's Array branch only works
	// for value types it natively understands.
	if ax, ok := a.(*object.Array); ok {
		bx, ok := b.(*object.Array)
		if !ok {
			return 0, false
		}
		n := len(ax.Elements)
		if len(bx.Elements) < n {
			n = len(bx.Elements)
		}
		for i := 0; i < n; i++ {
			c, ok := compareObjectsEnv(env, ax.Elements[i], bx.Elements[i])
			if !ok {
				return 0, false
			}
			if c != 0 {
				return c, true
			}
		}
		switch {
		case len(ax.Elements) < len(bx.Elements):
			return -1, true
		case len(ax.Elements) > len(bx.Elements):
			return 1, true
		}
		return 0, true
	}
	if c, ok := compareObjects(a, b); ok {
		return c, true
	}
	if sa, ok := a.(*object.Symbol); ok {
		if sb, ok := b.(*object.Symbol); ok {
			return strCompare(env.Symbols().Name(sa.ID), env.Symbols().Name(sb.ID)), true
		}
	}
	if inst, ok := a.(*object.Instance); ok {
		if _, found := dispatchClass(env, inst).LookupSpaceship(); found {
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
	args, kwargs, kwargOrder, blockProc, err := evalCallArguments(env, n.Arguments)
	if err != nil {
		return nil, err
	}
	// Stash pending kwargs on the env so the dispatch path (which
	// builds a fresh callEnv) can copy them onto the new frame's
	// CurrentKwargs without us threading another parameter through
	// every helper. Cleared after this call returns. CurrentKwargOrder
	// preserves source-text order for places that need to build a
	// Hash from the kwargs (Go maps don't keep insertion order).
	prevKwargs := env.CurrentKwargs
	prevKwargOrder := env.CurrentKwargOrder
	env.CurrentKwargs = kwargs
	env.CurrentKwargOrder = kwargOrder
	defer func() {
		env.CurrentKwargs = prevKwargs
		env.CurrentKwargOrder = prevKwargOrder
	}()

	if n.Context == nil {
		// SingletonHost takes precedence over EnclosingClass: inside
		// `class << host`, attr_reader / attr_writer / attr_accessor
		// must install on host's singleton table rather than the
		// enclosing class's instance methods. Mirrors the `def` routing
		// in eval_def.go's EnclosingSingletonHost branch (checked
		// before EnclosingClass for the same reason).
		if host := env.EnclosingSingletonHost(); host != nil {
			if v, handled, err := singletonBodyDSL(env, host, n.Function.Value, args); handled {
				return v, err
			}
		}
		if cls := env.EnclosingClass(); cls != nil {
			// classBodyDSL (attr_accessor / include / private / ...)
			// is only valid in the class body itself. Inside an
			// instance method body self is the instance; a bare-name
			// call like `include(pattern)` must dispatch to the
			// instance method, not the class-body helper. Skip the
			// DSL when EnclosingSelf is a concrete *Instance (i.e. we
			// are inside an instance-method frame). Class/Module body
			// scope leaves Self == cls; eigenclass body and top-level
			// nesting both keep Self as either the class or nil, so
			// neither flips into Instance.
			if _, isInst := env.EnclosingSelf().(*object.Instance); !isInst {
				// When self is a Class that differs from the
				// lexically enclosing class, prefer self as the DSL
				// target. This is the helper-method pattern: a method
				// defined on module CM and extended onto C, then
				// called inside C's body as `helper :foo` -- the
				// expected target is C (the receiver), not CM (the
				// lexical home of helper). MRI routes class-body DSL
				// calls through self for exactly this reason.
				target := cls
				if selfCls, ok := env.EnclosingSelf().(*object.Class); ok && selfCls != cls {
					target = selfCls
				}
				if v, handled, err := classBodyDSL(env, target, n.Function.Value, args); handled {
					return v, err
				}
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
		self := env.EnclosingSelf()
		// At toplevel, EnclosingSelf returns nil. If main has a
		// singleton method by this name (installed e.g. via
		// `self.extend Rake::DSL` -- the rake/dsl_definition.rb shape),
		// pin main as the receiver so the singleton dispatch is
		// reachable. Skip the synthesis when the name is a top-level
		// method or a kernel-builtin keyword (loop / proc / lambda),
		// since those go through their dedicated branches further down.
		if self == nil {
			if main, ok := mainObject(env).(*object.Instance); ok && main.SingletonMethods != nil {
				if _, ok := main.SingletonMethods[n.Function.Value]; ok {
					self = main
				}
			}
		}
		if self != nil {
			if _, isTop := env.GetMethod(n.Function.Value); !isTop {
				// Implicit-self inside a class method: dispatch via
				// the receiver's Class table (covers `new` inside
				// `def self.foo`).
				if cls, ok := self.(*object.Class); ok {
					// Kernel-builtin block forms (proc / lambda / loop)
					// must short-circuit before the class-receiver
					// dispatch -- they're not Class instance methods,
					// they're Kernel module methods callable from any
					// scope. without this, `proc do ... end` inside a
					// class body would error with NoMethodError on the
					// class.
					if n.Block != nil {
						switch n.Function.Value {
						case "proc", "lambda":
							return procFromBlock(env, n.Block), nil
						case "loop":
							return kernelLoop(env, n.Block)
						}
						return callMethodWithBlock(env, cls, n.Function.Value, args, n.Block)
					}
					if blockProc != nil {
						return callMethodWithProc(env, cls, n.Function.Value, args, blockProc)
					}
					// Try the class-receiver dispatch. If the method exists
					// on the class (or its ancestry up through
					// ClassClass / ModuleClass), use that result -- even
					// when it errors -- so a real error from inside the
					// method body propagates instead of silently falling
					// through to the kernel-builtin lookup.
					// Look the name up via the same chain callMethod will
					// walk: own ClassMethods first, then recv.Class()'s
					// instance-method ancestry. If it resolves, route the
					// call there so any error from the method body
					// propagates out rather than being swallowed by the
					// kernel-builtin fallback.
					_, hasOwn := cls.LookupClassMethod(n.Function.Value)
					if !hasOwn {
						if rc := cls.Class(); rc != nil {
							if _, hasInherited := rc.LookupMethod(n.Function.Value); hasInherited {
								hasOwn = true
							}
						}
					}
					if hasOwn {
						return callMethod(env, cls, n.Function.Value, args)
					}
				}
				// Implicit-self inside a method on a builtin-typed value
				// (Integer, String, Array, etc. -- anything whose
				// .Class() returns a *Class but the receiver itself
				// isn't an Instance). Dispatch through the receiver's
				// class to pick up reopened-method definitions
				// (e.g. `class Integer; def chr_utf_8; chr(...); end`
				// -- the bare `chr` call must resolve on Integer).
				if self != nil {
					if _, isInst := self.(*object.Instance); !isInst {
						if _, isCls := self.(*object.Class); !isCls {
							if cls := classOfRaw(env, self); cls != nil {
								if _, found := cls.LookupMethod(n.Function.Value); found {
									return callMethod(env, self, n.Function.Value, args)
								}
							}
						}
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
					// Bare-name block builtins (loop, proc, lambda) take
					// precedence over instance-receiver dispatch when the
					// instance class doesn't define the name. Without
					// this, `lambda do ... end` inside an instance method
					// would route to the inst's class and raise
					// NoMethodError on a name MRI resolves to Kernel.
					if n.Block != nil {
						switch n.Function.Value {
						case "loop":
							return kernelLoop(env, n.Block)
						case "proc", "lambda":
							p := procFromBlock(env, n.Block)
							p.IsLambda = n.Function.Value == "lambda"
							return p, nil
						}
					}
					// Fall through to receiver-style dispatch so
					// Comparable / Enumerable derivations (now living
					// on ObjectClass) are visible from inside the
					// class's own methods.
					if n.Block != nil {
						return callMethodWithBlock(env, inst, n.Function.Value, args, n.Block)
					}
					if blockProc != nil {
						return callMethodWithProc(env, inst, n.Function.Value, args, blockProc)
					}
					// Last-ditch implicit-self: route through
					// callMethod so universal methods (send,
					// instance_variable_set, etc.) work. Suppress
					// NoMethodError so we still fall back to kernel,
					// but propagate raised exceptions from inside the
					// method body -- a method that ran and raised
					// (Assertion / RuntimeError / ...) must NOT silently
					// fall through to the Kernel lookup.
					if v, err := callMethod(env, inst, n.Function.Value, args); err == nil {
						return v, nil
					} else if _, isRaise := err.(*raiseSignal); isRaise {
						return nil, err
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
			if n.Function.Value == "proc" || n.Function.Value == "lambda" {
				// Kernel#proc { ... } / Kernel#lambda { ... } -- both
				// wrap the literal block into a Proc. We treat the two
				// identically for now; mri distinguishes proc vs
				// lambda on return + arity behaviour, which the corpus
				// doesn't currently hinge on.
				return procFromBlock(env, n.Block), nil
			}
			// Fall back to implicit-self dispatch with block. Picks up
			// kernel-module instance methods reached via the main
			// object's class chain (e.g. Kernel#describe from
			// minitest/spec) and singleton methods on main.
			if main := mainObject(env); main != nil {
				if _, found := dispatchClass(env, main).LookupMethod(n.Function.Value); found {
					return callMethodWithBlock(env, main, n.Function.Value, args, n.Block)
				}
				if inst, ok := main.(*object.Instance); ok && inst.SingletonMethods != nil {
					if _, ok := inst.SingletonMethods[n.Function.Value]; ok {
						return callMethodWithBlock(env, main, n.Function.Value, args, n.Block)
					}
				}
			}
			return nil, errorf("evaluator: blocks on kernel calls not yet supported")
		}
		if blockProc != nil {
			if n.Function.Value == "lambda" || n.Function.Value == "proc" {
				// Kernel#lambda(&blk) / Kernel#proc(&blk) -- return a
				// fresh Proc wrapping the captured block. `lambda` flips
				// the IsLambda flag so Proc#lambda? reflects MRI's
				// strict-arity / return-semantics distinction.
				cp := *blockProc
				cp.IsLambda = n.Function.Value == "lambda"
				return &cp, nil
			}
			if m, ok := env.GetMethod(n.Function.Value); ok {
				if um, ok := m.(*object.UserMethod); ok {
					// User method called with &block-capture: wrap the
					// Proc into a synthetic block payload via the
					// shared callUserMethodWithBlock path.
					return callUserMethodWithProc(env, um, args, blockProc)
				}
			}
		}
		// Before falling to callKernel, check if main has the method
		// via its class chain (Object includes Kernel; gems may have
		// added methods directly to Kernel). Lets throw / describe /
		// other Kernel-module methods route correctly without a
		// dedicated callKernel switch case.
		if main := mainObject(env); main != nil {
			if _, found := dispatchClass(env, main).LookupMethod(n.Function.Value); found {
				return callMethod(env, main, n.Function.Value, args)
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
	// Explicit-receiver call: enforce visibility. Private methods
	// reject any explicit receiver (including `self.foo`); protected
	// methods reject unless the caller's self is an instance of the
	// same class (or a subclass).
	if inst, ok := recv.(*object.Instance); ok {
		mname := n.Function.Value
		if isPrivateMethod(inst.C, mname) {
			return raiseBuiltin(env, "NoMethodError", "private method `"+mname+"' called for instance of "+inst.C.Name)
		}
		if isProtectedMethod(inst.C, mname) && !callerIsKin(env, inst.C) {
			return raiseBuiltin(env, "NoMethodError", "protected method `"+mname+"' called for instance of "+inst.C.Name)
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

// isPrivateMethod walks c and its super chain looking for name in any
// Private set. Mirrors the LookupMethod walk so an inherited private
// method stays private in subclasses.
func isPrivateMethod(c *object.Class, name string) bool {
	for cur := c; cur != nil; cur = cur.Super {
		if cur.Private != nil && cur.Private[name] {
			return true
		}
	}
	return false
}

// isProtectedMethod is the Protected counterpart of isPrivateMethod.
func isProtectedMethod(c *object.Class, name string) bool {
	for cur := c; cur != nil; cur = cur.Super {
		if cur.Protected != nil && cur.Protected[name] {
			return true
		}
	}
	return false
}

// callerIsKin reports whether the caller's class shares lineage with
// target. Used to gate protected dispatch: MRI lets a protected method
// be called with an explicit receiver as long as both classes sit on
// the same inheritance chain in either direction. So either
// callerClass descends from target or target descends from callerClass.
func callerIsKin(env *object.Environment, target *object.Class) bool {
	self := env.EnclosingSelf()
	inst, ok := self.(*object.Instance)
	if !ok {
		return false
	}
	return inst.C == target ||
		inst.C.IsAncestor(target) ||
		target.IsAncestor(inst.C)
}

// evalCallArguments evaluates a call's argument list, splitting out
// trailing keyword args and a `&block` capture. Arguments shaped like
// `name: value` are emitted by the parser as `InfixExpression{Operator: ":"}`;
// `&expr` arrives as a `BlockCapture` node.
func evalCallArguments(env *object.Environment, exprs []ast.Expression) ([]object.RubyObject, map[string]object.RubyObject, []string, *object.Proc, error) {
	var args []object.RubyObject
	var kwargs map[string]object.RubyObject
	var kwargOrder []string
	var blockProc *object.Proc
	addKwarg := func(name string, v object.RubyObject) {
		if kwargs == nil {
			kwargs = make(map[string]object.RubyObject)
		}
		if _, seen := kwargs[name]; !seen {
			kwargOrder = append(kwargOrder, name)
		}
		kwargs[name] = v
	}
	for _, e := range exprs {
		if bc, ok := e.(*ast.BlockCapture); ok {
			p, err := blockCaptureToProc(env, bc)
			if err != nil {
				return nil, nil, nil, nil, err
			}
			blockProc = p
			continue
		}
		if sp, ok := e.(*ast.SplatExpression); ok {
			v, err := Eval(sp.Right, env)
			if err != nil {
				return nil, nil, nil, nil, err
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
					return nil, nil, nil, nil, errorf("evaluator: ** splat needs Hash, got %T", v)
				}
				for _, ent := range h.Entries {
					name, ok := symbolOrString(env, ent.Key)
					if !ok {
						return nil, nil, nil, nil, errorf("evaluator: ** splat key must be Symbol/String, got %T", ent.Key)
					}
					addKwarg(name, ent.Value)
				}
				continue
			}
		}
		if key, val, ok := splitKwargInfix(e); ok {
			v, err := Eval(val, env)
			if err != nil {
				return nil, nil, nil, nil, err
			}
			addKwarg(key, v)
			continue
		}
		v, err := Eval(e, env)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		args = append(args, v)
	}
	return args, kwargs, kwargOrder, blockProc, nil
}

// blockCaptureToProc resolves `&x` for the shapes the evaluator
// supports today: an Identifier already bound to a Proc, an inline
// Symbol literal (`&:upcase`), or any expression that evaluates to a
// Proc.
func blockCaptureToProc(env *object.Environment, bc *ast.BlockCapture) (*object.Proc, error) {
	if bc.Name != nil {
		v, ok := env.Get(bc.Name.Value)
		if !ok {
			// Not a local -- try calling it as a method. Covers the
			// idiom `&class_method` where the named method returns a
			// Proc (e.g. `Command.new('$', &noop)` calling self.noop
			// which `proc {}`s and returns the Proc to capture).
			if cv, cerr := evalIdentifier(env, &ast.Identifier{Value: bc.Name.Value}); cerr == nil {
				v = cv
			} else {
				return nil, errorf("evaluator: NameError: undefined local variable or method `%s' for &-capture", bc.Name.Value)
			}
		}
		// `&nil` is the legal "no block" form -- forwarded as "no
		// block" rather than erroring.
		if _, isNil := v.(*object.Nil); isNil {
			return nil, nil
		}
		if p, ok := v.(*object.Proc); ok {
			return p, nil
		}
		// MRI: &sym implicitly invokes Symbol#to_proc, building a Proc
		// that calls the symbol's name on each yielded value. Matches
		// `args.map(&op)` where op = :method_name.
		if sym, ok := v.(*object.Symbol); ok {
			return procFromSymbol(env.Symbols().Name(sym.ID)), nil
		}
		return nil, errorf("evaluator: &-capture: %T is not a Proc", v)
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
	if p, ok := v.(*object.Proc); ok {
		return p, nil
	}
	// MRI: &sym implicitly invokes Symbol#to_proc, so passing a
	// Symbol-typed local via &-capture builds a Proc that calls the
	// named method on each yielded value. Matches the `&:to_s`
	// shape via a local variable holding the symbol.
	if sym, ok := v.(*object.Symbol); ok {
		return procFromSymbol(env.Symbols().Name(sym.ID)), nil
	}
	return nil, errorf("evaluator: &-capture: %T is not a Proc", v)
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
		// `f(*args)`: expand a splat argument inline. Arrays splice in
		// element-wise; non-arrays land as a single arg (matching MRI's
		// implicit to_a for non-Array splats -- approximated here as
		// "just pass through").
		if sp, ok := e.(*ast.SplatExpression); ok && sp.Operator == "*" {
			v, err := Eval(sp.Right, env)
			if err != nil {
				return nil, err
			}
			if arr, ok := v.(*object.Array); ok {
				out = append(out, arr.Elements...)
				continue
			}
			out = append(out, v)
			continue
		}
		v, err := Eval(e, env)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

// callMethod dispatches `name` on `recv`. For a Class receiver, the
// class's own ClassMethods table (populated by `def self.foo`) is
// consulted first so user-defined class methods win over the
// ClassClass / ModuleClass-level fallbacks. Then Send walks
// recv.Class()'s ancestry. NoMethodError on miss.
func callMethod(env *object.Environment, recv object.RubyObject, name string, args []object.RubyObject) (object.RubyObject, error) {
	args = bundleKwargsForBuiltin(env, recv, name, args)
	if cls, ok := recv.(*object.Class); ok {
		if m, found := cls.LookupClassMethod(name); found {
			switch mm := m.(type) {
			case *object.UserMethod:
				return invokeMethodOn(env, cls, mm, args, nil)
			case *object.BuiltinMethod:
				return mm.Fn(env, cls, args, nil)
			}
		}
	}
	if v, ok, err := object.Send(env, recv, name, args, nil); ok {
		return v, err
	}
	// Try method_missing on Instance receivers before raising.
	if inst, ok := recv.(*object.Instance); ok {
		var m object.RubyMethod
		var found bool
		if inst.SingletonMethods != nil {
			m, found = inst.SingletonMethods["method_missing"]
		}
		if !found {
			m, found = dispatchClass(env, inst).LookupMethod("method_missing")
		}
		if found {
			mmArgs := append([]object.RubyObject{env.Symbols().Intern(name)}, args...)
			switch mm := m.(type) {
			case *object.UserMethod:
				return invokeMethodOn(env, inst, mm, mmArgs, nil)
			case *object.BuiltinMethod:
				return mm.Fn(env, inst, mmArgs, nil)
			}
		}
		return raiseBuiltin(env, "NoMethodError", "undefined method `"+name+"' for instance of "+inst.C.Name)
	}
	return raiseBuiltin(env, "NoMethodError", "undefined method `"+name+"'")
}

// bundleKwargsForBuiltin bridges the kwargs/Hash gap for builtin
// targets. When the caller passed trailing `key: val` syntax (which
// evalCallArguments stashed in env.CurrentKwargs) and the resolved
// method is a builtin (no kwargs consumption), pack the kwargs into a
// trailing Hash positional arg and clear them so the existing
// UserMethod kwargs flow doesn't double-handle. UserMethod targets
// keep their own kwargs binding path (eval_def.go), so they're left
// alone here.
//
// Lookup mirrors object.Send's chain: singleton methods, then the
// class ancestry. We resolve once just to classify the method shape;
// the actual dispatch re-resolves through Send for consistency.
func bundleKwargsForBuiltin(env *object.Environment, recv object.RubyObject, name string, args []object.RubyObject) []object.RubyObject {
	if len(env.CurrentKwargs) == 0 {
		return args
	}
	m := resolveMethod(env, recv, name)
	if m == nil {
		return args
	}
	if _, isUser := m.(*object.UserMethod); isUser {
		return args
	}
	// `Class#new` is a builtin but forwards to user-defined
	// `initialize`, which may declare kwargs. Leave the kwargs alone
	// so initialize's bindParams can consume them. classNew handles
	// the threading.
	if _, ok := recv.(*object.Class); ok && name == "new" {
		return args
	}
	entries := make([]object.HashEntry, 0, len(env.CurrentKwargs))
	// Prefer ordered iteration so the resulting Hash preserves source
	// order (matches MRI's `{c: 3, a: 1}` literal-order inspect).
	keys := env.CurrentKwargOrder
	if len(keys) != len(env.CurrentKwargs) {
		keys = keys[:0]
		for k := range env.CurrentKwargs {
			keys = append(keys, k)
		}
	}
	for _, k := range keys {
		entries = append(entries, object.HashEntry{
			Key:   env.Symbols().Intern(k),
			Value: env.CurrentKwargs[k],
		})
	}
	env.CurrentKwargs = nil
	env.CurrentKwargOrder = nil
	return append(args, object.NewHash(entries...))
}

// resolveMethod replays object.Send's lookup order and returns the
// matched RubyMethod (without invoking it). Returns nil when nothing
// matches -- callers treat that as "let Send raise NoMethodError".
func resolveMethod(env *object.Environment, recv object.RubyObject, name string) object.RubyMethod {
	if cls, ok := recv.(*object.Class); ok {
		if m, found := cls.LookupClassMethod(name); found {
			return m
		}
	}
	if inst, ok := recv.(*object.Instance); ok {
		if m, found := inst.SingletonMethods[name]; found {
			return m
		}
		if inst.SingletonClass != nil {
			if m, found := inst.SingletonClass.LookupMethod(name); found {
				return m
			}
		}
	}
	cls := classOfRaw(env, recv)
	if cls == nil {
		return nil
	}
	if m, found := cls.LookupMethod(name); found {
		return m
	}
	return nil
}

