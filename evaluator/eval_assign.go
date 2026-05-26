package evaluator

import (
	"github.com/lczyk/goruby/ast"
	"github.com/lczyk/goruby/object"
	"github.com/lczyk/goruby/token"
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

// evalShortCircuitAssign implements `x ||= rhs` / `x &&= rhs` for the
// common Identifier LHS shape, honouring ruby's "undefined LHS reads as
// nil for op-assign" rule. Returns handled=false for shapes we don't
// special-case (e.g. attr setters); the caller falls back to the
// generic Assignment path.
func evalShortCircuitAssign(env *object.Environment, n *ast.Assignment) (object.RubyObject, bool, error) {
	id, ok := n.Left.(*ast.Identifier)
	if !ok {
		return nil, false, nil
	}
	cur, defined := env.Get(id.Value)
	if !defined {
		cur = object.NIL
	}

	if n.Token.Type == token.ORASSIGN {
		if truthy(cur) {
			return cur, true, nil
		}
		// Parser-desugared RHS is `lhs || rhs`; we already know lhs
		// is falsy, so the InfixExpression evaluates to its right
		// operand. Pull that directly.
		rhs, err := evalAssignRHS(env, n.Right)
		if err != nil {
			return nil, true, err
		}
		env.AssignVisible(id.Value, expandSingle(rhs))
		return rhs, true, nil
	}
	// ANDASSIGN: only assign when current is truthy.
	if !truthy(cur) {
		return cur, true, nil
	}
	rhs, err := evalAssignRHS(env, n.Right)
	if err != nil {
		return nil, true, err
	}
	env.AssignVisible(id.Value, expandSingle(rhs))
	return rhs, true, nil
}

// evalAssignRHS pulls the rhs operand of the parser-desugared
// `lhs OP rhs` shape produced for `||=` / `&&=`. Falls back to the
// whole expression when the shape doesn't match.
func evalAssignRHS(env *object.Environment, e ast.Expression) (object.RubyObject, error) {
	if infix, ok := e.(*ast.InfixExpression); ok {
		return Eval(infix.Right, env)
	}
	return Eval(e, env)
}

func evalIdentifier(env *object.Environment, n *ast.Identifier) (object.RubyObject, error) {
	// `block_given?` reads the current method frame's block slot.
	if n.Value == "block_given?" {
		return object.BooleanOf(env.EnclosingBlock() != nil), nil
	}
	// Visibility keywords used bare inside a class body flip the
	// class's CurrentVisibility so subsequent defs are tagged
	// accordingly. Argument-form ("private :foo, :bar") goes through
	// the regular method-call path elsewhere.
	if cls := env.EnclosingClass(); cls != nil {
		switch n.Value {
		case "private":
			cls.CurrentVisibility = "private"
			return object.NIL, nil
		case "public":
			cls.CurrentVisibility = "public"
			return object.NIL, nil
		case "protected":
			cls.CurrentVisibility = "protected"
			return object.NIL, nil
		case "module_function":
			// Subsequent defs install as both private instance methods
			// and class methods on the module. MRI semantics; rake's
			// Cleaner module relies on it (Rake::Cleaner.cleanup).
			cls.CurrentVisibility = "module_function"
			return object.NIL, nil
		}
	}
	if isConstantName(n.Value) {
		// Walk the lexical chain: every enclosing class/module body
		// gets a shot, then their Super chains. Matches MRI's
		// `Module.nesting`-driven constant lookup -- a constant
		// declared in an outer module is visible from any nested
		// module's body without an explicit ::.
		for cur := env; cur != nil; cur = cur.Outer() {
			if cls := cur.CurrentClass; cls != nil {
				if v, ok := lookupConstant(cls, n.Value); ok {
					return v, nil
				}
			}
		}
		if v, ok := env.Get(n.Value); ok {
			return v, nil
		}
		return nil, errorf("evaluator: NameError: uninitialized constant %s", n.Value)
	}
	if v, ok := env.Get(n.Value); ok {
		return v, nil
	}
	if m, ok := env.GetMethod(n.Value); ok {
		if um, ok := m.(*object.UserMethod); ok {
			return callUserMethod(env, um, nil)
		}
	}
	if self := env.EnclosingSelf(); self != nil {
		if inst, ok := self.(*object.Instance); ok {
			if m, found := dispatchClass(env, inst).LookupMethod(n.Value); found {
				switch mm := m.(type) {
				case *object.UserMethod:
					return invokeMethodOn(env, inst, mm, nil, nil)
				case *object.BuiltinMethod:
					return mm.Fn(env, inst, nil, nil)
				}
			}
		}
		if cls, ok := self.(*object.Class); ok {
			// Bare-identifier read inside a class / module body where
			// the name matches a class method on self. Lets idioms
			// like `&noop` (capturing the result of self.noop) work
			// without the explicit `self.` prefix.
			if m, found := cls.LookupClassMethod(n.Value); found {
				if um, ok := m.(*object.UserMethod); ok {
					return invokeMethodOn(env, cls, um, nil, nil)
				}
			}
			// Bare-identifier inside a class method where the name
			// matches a class-level method on the class's class
			// (typically `new`, inherited from ClassClass/ModuleClass).
			if rc := cls.Class(); rc != nil {
				if _, found := rc.LookupMethod(n.Value); found {
					return callMethod(env, cls, n.Value, nil)
				}
			}
		}
	}
	// Top-level fallback: if main has the name via its class chain
	// (typically a method added by reopening Kernel), dispatch
	// against main as the implicit receiver.
	if env.EnclosingSelf() == nil {
		if main, ok := mainObject(env).(*object.Instance); ok {
			if m, found := dispatchClass(env, main).LookupMethod(n.Value); found {
				switch mm := m.(type) {
				case *object.UserMethod:
					return invokeMethodOn(env, main, mm, nil, nil)
				case *object.BuiltinMethod:
					return mm.Fn(env, main, nil, nil)
				}
			}
		}
	}
	// Final fallback: try Kernel builtins for bare-name identifiers
	// (`puts` / `print` / `p` etc. used as statements without parens
	// or arguments). Suppresses the NameError when the builtin exists.
	if v, kerr := callKernel(env, n.Value, nil); kerr == nil {
		return v, nil
	}
	// NameError is a raisable Ruby exception so `rescue NameError`
	// (and `rescue StandardError`, its ancestor) can recover -- the
	// shape rake/cpu_counter.rb's count_with_default relies on.
	return raiseBuiltin(env, "NameError", "undefined local variable or method `"+n.Value+"'")
}

func isConstantName(s string) bool {
	if s == "" {
		return false
	}
	c := s[0]
	return c >= 'A' && c <= 'Z'
}

// lookupConstant walks the class chain looking for a named constant.
func lookupConstant(cls *object.Class, name string) (object.RubyObject, bool) {
	for cur := cls; cur != nil; cur = cur.Super {
		if v, ok := cur.Constants[name]; ok {
			return v, true
		}
		for _, inc := range cur.Includes {
			if v, ok := lookupConstantInIncludes(inc, name); ok {
				return v, true
			}
		}
	}
	return nil, false
}

// lookupConstantInIncludes walks an included module's own constants and
// its (recursive) includes, but does NOT follow Super -- mirrors MRI's
// constant resolution which scans the module-ancestor chain.
func lookupConstantInIncludes(m *object.Class, name string) (object.RubyObject, bool) {
	if m == nil {
		return nil, false
	}
	if v, ok := m.Constants[name]; ok {
		return v, true
	}
	for _, inc := range m.Includes {
		if v, ok := lookupConstantInIncludes(inc, name); ok {
			return v, true
		}
	}
	return nil, false
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
	// `||=` / `&&=`: parser desugars to `lhs = lhs || rhs` / `lhs && rhs`,
	// but in ruby an undefined LHS does NOT raise -- it's treated as nil
	// (||=) or skipped entirely (&&=). Detect by token type so the
	// short-circuit evaluation matches MRI for previously-unbound names.
	if n.Token.Type == token.ORASSIGN || n.Token.Type == token.ANDASSIGN {
		if v, handled, err := evalShortCircuitAssign(env, n); handled {
			return v, err
		}
	}

	right, err := Eval(n.Right, env)
	if err != nil {
		return nil, err
	}

	switch lhs := n.Left.(type) {
	case *ast.Identifier:
		v := expandSingle(right)
		if isConstantName(lhs.Value) {
			// Anonymous-class naming: when an unnamed (or differently
			// named) class is assigned to a Constant, take on that
			// Constant's name -- mirrors MRI's `Foo = Class.new`.
			if c, ok := v.(*object.Class); ok && (c.Name == "" || c.Name == "StructClass" || c.Name == "Data") {
				c.Name = lhs.Value
			}
			if cls := env.EnclosingClass(); cls != nil {
				cls.Constants[lhs.Value] = v
			} else {
				env.SetGlobal(lhs.Value, v)
			}
			return right, nil
		}
		env.AssignVisible(lhs.Value, v)
		return right, nil
	case *ast.Global:
		env.SetGlobal(lhs.Value, expandSingle(right))
		return right, nil
	case ast.ExpressionList:
		return evalMultiAssign(env, lhs, right)
	case *ast.IndexExpression:
		if err := evalIndexAssign(env, lhs, expandSingle(right)); err != nil {
			return nil, err
		}
		return right, nil
	case *ast.InstanceVariable:
		key := "@" + lhs.Name.Value
		v := expandSingle(right)
		switch self := env.EnclosingSelf().(type) {
		case *object.Instance:
			self.Ivars[key] = v
		case *object.Class:
			if self.Ivars == nil {
				self.Ivars = map[string]object.RubyObject{}
			}
			self.Ivars[key] = v
		default:
			return nil, errorf("evaluator: @%s set outside an instance context", lhs.Name.Value)
		}
		return right, nil
	case *ast.ClassVariable:
		cls := classForCVar(env)
		if cls == nil {
			return nil, errorf("evaluator: @@%s set outside a class", lhs.Name.Value)
		}
		key := "@@" + lhs.Name.Value
		// If an ancestor already owns the cvar, update there (MRI's
		// cvar-sharing semantics); otherwise create on the current
		// class.
		_, owner := cls.LookupClassVar(key)
		if owner == nil {
			owner = cls
		}
		owner.ClassVars[key] = expandSingle(right)
		return right, nil
	case *ast.ContextCallExpression:
		// Op-assign on a setter target: `obj.attr += val` parses as
		// Assignment{Left: ContextCallExpression{attr}, Right: Infix{
		// obj.attr, +, val}}. The plain-assignment form `obj.attr = val`
		// does NOT reach here -- parser emits a ContextCallExpression
		// w/ "attr=" function and val as the sole argument, dispatched
		// via evalContextCall directly. Only op-assign retains the
		// dot-call shape on the LHS, so this case exists to handle that
		// path and the parallel multi-assign-setter case in assignTarget.
		if lhs.Context == nil {
			return nil, errorf("evaluator: op-assign setter target has no receiver")
		}
		recv, err := Eval(lhs.Context, env)
		if err != nil {
			return nil, err
		}
		v := expandSingle(right)
		if _, err := callMethod(env, recv, lhs.Function.Value+"=", []object.RubyObject{v}); err != nil {
			return nil, err
		}
		return v, nil
	}
	if lhs, ok := n.Left.(*ast.ScopedIdentifier); ok {
		// Outer::Inner = value -- assign a constant under the named
		// enclosing class/module. Resolves Outer through the
		// usual lexical lookup, then sets Constants[Inner] = value.
		// Mirrors the same path lookupOrCreateNestedClass uses when
		// installing nested defs.
		parent, err := Eval(lhs.Outer, env)
		if err != nil {
			return nil, err
		}
		cls, ok := parent.(*object.Class)
		if !ok {
			return nil, errorf("evaluator: scoped assign: %s is not a class/module", lhs.Outer)
		}
		inner, ok := lhs.Inner.(*ast.Identifier)
		if !ok {
			return nil, errorf("evaluator: scoped assign: inner must be an Identifier, got %T", lhs.Inner)
		}
		v := expandSingle(right)
		if cls.Constants == nil {
			cls.Constants = map[string]object.RubyObject{}
		}
		cls.Constants[inner.Value] = v
		// If the value is itself a class with no Parent yet, stamp it
		// so subsequent QualifiedName lookups render Outer::Inner.
		if vcls, ok := v.(*object.Class); ok && vcls.Parent == nil {
			vcls.Parent = cls
			// Rename when the class is anonymous or carries the
			// generic Struct/Data placeholder names (Struct.new and
			// Data.define both produce classes pending a real name).
			if vcls.Name == "" || vcls.Name == "StructClass" || vcls.Name == "Data" {
				vcls.Name = inner.Value
			}
		}
		return right, nil
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

	splatIdx := -1
	for i, target := range lhs {
		if _, ok := target.(*ast.SplatExpression); ok {
			splatIdx = i
			break
		}
	}

	if splatIdx == -1 {
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

	preCount := splatIdx
	postCount := len(lhs) - splatIdx - 1

	for i := 0; i < preCount; i++ {
		var v object.RubyObject = object.NIL
		if i < len(values) {
			v = values[i]
		}
		if err := assignTarget(env, lhs[i], v); err != nil {
			return nil, err
		}
	}

	splatLen := len(values) - preCount - postCount
	if splatLen < 0 {
		splatLen = 0
	}
	splatVals := make([]object.RubyObject, splatLen)
	if splatLen > 0 {
		copy(splatVals, values[preCount:preCount+splatLen])
	}
	sp, _ := lhs[splatIdx].(*ast.SplatExpression)
	if sp.Right != nil {
		if err := assignTarget(env, sp.Right, object.NewArray(splatVals...)); err != nil {
			return nil, err
		}
	}

	for i := 0; i < postCount; i++ {
		var v object.RubyObject = object.NIL
		srcIdx := preCount + splatLen + i
		if srcIdx < len(values) {
			v = values[srcIdx]
		}
		if err := assignTarget(env, lhs[splatIdx+1+i], v); err != nil {
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

func evalIndexAssign(env *object.Environment, n *ast.IndexExpression, value object.RubyObject) error {
	recv, err := Eval(n.Left, env)
	if err != nil {
		return err
	}
	keys := make([]object.RubyObject, 0, len(n.Arguments))
	for _, a := range n.Arguments {
		v, err := Eval(a, env)
		if err != nil {
			return err
		}
		keys = append(keys, v)
	}
	// Multi-arg []= on user-defined classes: forward all keys + value
	// to the class's []= method. Hash/Array keep single-arg semantics
	// (the only forms MRI defines for them).
	if len(keys) != 1 {
		if inst, ok := recv.(*object.Instance); ok {
			if m, found := dispatchClass(env, inst).LookupMethod("[]="); found {
				if um, ok := m.(*object.UserMethod); ok {
					_, err := invokeMethodOn(env, inst, um, append(append([]object.RubyObject{}, keys...), value), nil)
					return err
				}
			}
			_, err := raiseBuiltin(env, "NoMethodError", "undefined method `[]=' for instance of "+inst.C.Name)
			return err
		}
		return errorf("evaluator: []= with %d args not yet supported on %T", len(keys), recv)
	}
	key := keys[0]
	switch r := recv.(type) {
	case *object.Hash:
		for i, e := range r.Entries {
			if rubyEqualDispatch(env, e.Key, key) {
				r.Entries[i].Value = value
				return nil
			}
		}
		r.Entries = append(r.Entries, object.HashEntry{Key: key, Value: value})
		return nil
	case *object.Array:
		idx, ok := key.(*object.Integer)
		if !ok {
			return errorf("evaluator: Array#[]= needs Integer index, got %T", key)
		}
		i := int(idx.Value)
		if i < 0 {
			i += len(r.Elements)
			if i < 0 {
				return errorf("evaluator: IndexError: index too small")
			}
		}
		for i >= len(r.Elements) {
			r.Elements = append(r.Elements, object.NIL)
		}
		r.Elements[i] = value
		return nil
	case *object.Instance:
		if m, found := dispatchClass(env, r).LookupMethod("[]="); found {
			switch m := m.(type) {
			case *object.UserMethod:
				_, err := invokeMethodOn(env, r, m, []object.RubyObject{key, value}, nil)
				return err
			case *object.BuiltinMethod:
				_, err := m.Fn(env, r, []object.RubyObject{key, value}, nil)
				return err
			}
		}
		_, err := raiseBuiltin(env, "NoMethodError", "undefined method `[]=' for instance of "+r.C.Name)
		return err
	case *object.Class:
		// Class-receiver []= (e.g. Warning[:x] = v) -- dispatch to
		// the class's []= class method if defined.
		if m, found := r.LookupClassMethod("[]="); found {
			switch m := m.(type) {
			case *object.UserMethod:
				_, err := invokeMethodOn(env, r, m, []object.RubyObject{key, value}, nil)
				return err
			case *object.BuiltinMethod:
				_, err := m.Fn(env, r, []object.RubyObject{key, value}, nil)
				return err
			}
		}
		_, err := raiseBuiltin(env, "NoMethodError", "undefined method `[]=' for "+r.Name+":Class")
		return err
	}
	return errorf("evaluator: []= not yet supported on %T", recv)
}

func assignTarget(env *object.Environment, target ast.Expression, value object.RubyObject) error {
	switch t := target.(type) {
	case *ast.Identifier:
		if isConstantName(t.Value) {
			// Mirror evalAssignment's constant-binding rules so that
			// `MAJOR, MINOR, BUILD = ...` inside a module body lands the
			// names on the enclosing class's Constants map, and at
			// top-level lands them in the global env. Without this,
			// AssignVisible buries the name as a local in the current
			// frame and `M::NAME` lookup fails.
			if c, ok := value.(*object.Class); ok && (c.Name == "" || c.Name == "StructClass" || c.Name == "Data") {
				c.Name = t.Value
			}
			if cls := env.EnclosingClass(); cls != nil {
				cls.Constants[t.Value] = value
			} else {
				env.SetGlobal(t.Value, value)
			}
			return nil
		}
		env.AssignVisible(t.Value, value)
		return nil
	case *ast.Global:
		env.SetGlobal(t.Value, value)
		return nil
	case *ast.InstanceVariable:
		key := "@" + t.Name.Value
		switch self := env.EnclosingSelf().(type) {
		case *object.Instance:
			self.Ivars[key] = value
		case *object.Class:
			if self.Ivars == nil {
				self.Ivars = map[string]object.RubyObject{}
			}
			self.Ivars[key] = value
		default:
			return errorf("evaluator: @%s= outside instance context (self=%T)", t.Name.Value, self)
		}
		return nil
	case *ast.ClassVariable:
		cls := classForCVar(env)
		if cls == nil {
			return errorf("evaluator: @@%s= outside class context", t.Name.Value)
		}
		if cls.ClassVars == nil {
			cls.ClassVars = map[string]object.RubyObject{}
		}
		cls.ClassVars["@@"+t.Name.Value] = value
		return nil
	case *ast.IndexExpression:
		return evalIndexAssign(env, t, value)
	case *ast.ContextCallExpression:
		// Multi-assignment with setter targets: `a.x, a.y = 1, 2`.
		// Parser keeps each LHS as a ContextCallExpression w/ no
		// trailing `=` -- we synthesise the setter name here and
		// dispatch attr= on the receiver.
		if t.Context == nil {
			return errorf("evaluator: multi-assign setter target has no receiver")
		}
		recv, err := Eval(t.Context, env)
		if err != nil {
			return err
		}
		_, err = callMethod(env, recv, t.Function.Value+"=", []object.RubyObject{value})
		return err
	}
	return errorf("evaluator: unsupported multi-assignment target %T", target)
}
