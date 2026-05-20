package evaluator

import (
	"github.com/lczyk/goruby/ast"
	"github.com/lczyk/goruby/object"
)

// evalClassExpression opens (or reopens) the class named by n and runs
// its body in a class-body env. Reopening lets `class Foo ... end`
// repeat in a file or test.
func evalClassExpression(env *object.Environment, n *ast.ClassExpression) (object.RubyObject, error) {
	name := n.Name.Value
	var super *object.Class
	if n.SuperClass != nil {
		sup, err := Eval(n.SuperClass, env)
		if err != nil {
			return nil, err
		}
		c, ok := sup.(*object.Class)
		if !ok {
			return nil, errorf("evaluator: TypeError: superclass must be a Class (%T given)", sup)
		}
		super = c
	}

	cls := lookupOrCreateClass(env, name, super)

	bodyEnv := object.NewEnclosedEnvironment(env)
	bodyEnv.CurrentClass = cls
	bodyEnv.Self = cls

	if n.Body != nil {
		if _, err := evalBlockStatement(bodyEnv, n.Body); err != nil {
			return nil, err
		}
	}
	return cls, nil
}

// evalModuleExpression treats a module as a class with IsModule=true
// and no superclass. Method lookup walks Includes so module methods
// reach instances of including classes.
func evalModuleExpression(env *object.Environment, n *ast.ModuleExpression) (object.RubyObject, error) {
	mod := lookupOrCreateClass(env, n.Name.Value, nil)
	mod.IsModule = true

	bodyEnv := object.NewEnclosedEnvironment(env)
	bodyEnv.CurrentClass = mod
	bodyEnv.Self = mod

	if n.Body != nil {
		if _, err := evalBlockStatement(bodyEnv, n.Body); err != nil {
			return nil, err
		}
	}
	return mod, nil
}

// evalScopedIdentifier resolves `Outer::Inner` -- only the simple
// `<Class>::<Const>` form for now (no nested scope chains beyond two
// levels).
func evalScopedIdentifier(env *object.Environment, n *ast.ScopedIdentifier) (object.RubyObject, error) {
	if n.Outer == nil {
		return nil, errorf("evaluator: top-level ::Const lookup not yet supported")
	}
	outerVal, err := Eval(n.Outer, env)
	if err != nil {
		return nil, err
	}
	cls, ok := outerVal.(*object.Class)
	if !ok {
		return nil, errorf("evaluator: TypeError: %T is not a class/module", outerVal)
	}
	innerID, ok := n.Inner.(*ast.Identifier)
	if !ok {
		return nil, errorf("evaluator: unsupported ::Inner %T", n.Inner)
	}
	if v, ok := cls.Constants[innerID.Value]; ok {
		return v, nil
	}
	return nil, errorf("evaluator: NameError: uninitialized constant %s::%s", cls.Name, innerID.Value)
}

// lookupOrCreateClass returns the existing class bound to name on the
// root env, or creates and stores a fresh one.
func lookupOrCreateClass(env *object.Environment, name string, super *object.Class) *object.Class {
	if existing, ok := env.Get(name); ok {
		if c, ok := existing.(*object.Class); ok {
			return c
		}
	}
	c := object.NewClass(name, super)
	env.SetGlobal(name, c)
	return c
}

// evalClassVariable reads a @@cvar from the enclosing class chain.
// Inside an instance method, walks self's class chain; inside a class
// method or class body, uses the current class directly.
func evalClassVariable(env *object.Environment, n *ast.ClassVariable) (object.RubyObject, error) {
	cls := classForCVar(env)
	if cls == nil {
		return nil, errorf("evaluator: @@%s referenced outside a class", n.Name.Value)
	}
	v, _ := cls.LookupClassVar("@@" + n.Name.Value)
	if v == nil {
		return object.NIL, nil
	}
	return v, nil
}

// classForCVar returns the class @@cvar references resolve against.
func classForCVar(env *object.Environment) *object.Class {
	if cls := env.EnclosingClass(); cls != nil {
		return cls
	}
	if self := env.EnclosingSelf(); self != nil {
		if inst, ok := self.(*object.Instance); ok {
			return inst.C
		}
		if cls, ok := self.(*object.Class); ok {
			return cls
		}
	}
	return nil
}

// evalInstanceVariable reads an @ivar from the current self instance.
// Returns nil on miss (MRI: undefined ivar reads return nil with a
// warning).
func evalInstanceVariable(env *object.Environment, n *ast.InstanceVariable) (object.RubyObject, error) {
	self := env.EnclosingSelf()
	inst, ok := self.(*object.Instance)
	if !ok {
		return object.NIL, nil
	}
	if v, ok := inst.Ivars["@"+n.Name.Value]; ok {
		return v, nil
	}
	return object.NIL, nil
}

func evalSelf(env *object.Environment) (object.RubyObject, error) {
	if self := env.EnclosingSelf(); self != nil {
		return self, nil
	}
	return mainObject(env), nil
}

// mainObject returns the singleton representing top-level `self`. Its
// class is the predefined `Object` class.
func mainObject(env *object.Environment) object.RubyObject {
	obj := bootstrapObjectClass(env)
	if existing, ok := env.Get("__main__"); ok {
		return existing
	}
	inst := object.NewInstance(obj)
	env.SetGlobal("__main__", inst)
	return inst
}

// bootstrapObjectClass returns the predefined `Object` class, creating
// it on first use. All user classes implicitly inherit from it when no
// superclass is named, and top-level `self` is an instance of it.
func bootstrapObjectClass(env *object.Environment) *object.Class {
	if existing, ok := env.Get("Object"); ok {
		if c, ok := existing.(*object.Class); ok {
			return c
		}
	}
	c := object.NewClass("Object", nil)
	env.SetGlobal("Object", c)
	return c
}

// evalSuper invokes the next implementation up the inheritance chain
// for the current method. Implicit-args form (no parentheses) reuses
// the original call's args; explicit-args form evaluates the supplied
// list.
func evalSuper(env *object.Environment, n *ast.SuperExpression) (object.RubyObject, error) {
	name := findMethodName(env)
	if name == "" {
		return nil, errorf("evaluator: super called outside of method")
	}
	self := env.EnclosingSelf()
	inst, ok := self.(*object.Instance)
	if !ok {
		return nil, errorf("evaluator: super outside instance context not yet supported")
	}
	cls := findCallClass(env)
	if cls == nil || cls.Super == nil {
		return nil, errorf("evaluator: NoMethodError: super: no superclass method `%s'", name)
	}
	m, found := cls.Super.LookupMethod(name)
	if !found {
		return nil, errorf("evaluator: NoMethodError: super: no superclass method `%s'", name)
	}
	um, ok := m.(*object.UserMethod)
	if !ok {
		return nil, errorf("evaluator: super on non-user method %T", m)
	}

	var args []object.RubyObject
	if n.Arguments == nil {
		args = findMethodArgs(env)
	} else {
		got, err := evalExpressions(env, n.Arguments)
		if err != nil {
			return nil, err
		}
		args = got
	}
	return invokeMethodOn(env, inst, um, args, nil)
}

func findMethodName(env *object.Environment) string {
	for cur := env; cur != nil; cur = cur.Outer() {
		if cur.MethodFrame && cur.CurrentMethodName != "" {
			return cur.CurrentMethodName
		}
	}
	return ""
}

func findMethodArgs(env *object.Environment) []object.RubyObject {
	for cur := env; cur != nil; cur = cur.Outer() {
		if cur.MethodFrame {
			return cur.CurrentMethodArgs
		}
	}
	return nil
}

func findCallClass(env *object.Environment) *object.Class {
	for cur := env; cur != nil; cur = cur.Outer() {
		if cur.MethodFrame && cur.CurrentClass != nil {
			return cur.CurrentClass
		}
	}
	return nil
}

// arrayClassNew implements `Array.new(size = 0, default = nil)` -- the
// no-block forms used by the corpus. Block form (`Array.new(n) { |i|
// ... }`) lives on the block-aware dispatch path.
func arrayClassNew(args []object.RubyObject) (object.RubyObject, error) {
	switch len(args) {
	case 0:
		return object.NewArray(), nil
	case 1, 2:
		n, ok := args[0].(*object.Integer)
		if !ok {
			return nil, errorf("evaluator: Array.new: size must be Integer")
		}
		if n.Value < 0 {
			return nil, errorf("evaluator: ArgumentError: negative array size")
		}
		var fill object.RubyObject = object.NIL
		if len(args) == 2 {
			fill = args[1]
		}
		els := make([]object.RubyObject, n.Value)
		for i := range els {
			els[i] = fill
		}
		return object.NewArray(els...), nil
	}
	return nil, errorf("evaluator: Array.new: wrong number of arguments (%d)", len(args))
}

// classBodyDSL handles the class-level helpers used inside a class
// body: attr_accessor / attr_reader / attr_writer (which synthesise
// instance methods) and include (which mixes a module's methods into
// the class). Returns handled=false for anything else, letting the
// regular call path take over.
func classBodyDSL(env *object.Environment, cls *object.Class, name string, args []object.RubyObject) (object.RubyObject, bool, error) {
	switch name {
	case "attr_reader":
		for _, a := range args {
			n, ok := symbolOrString(env, a)
			if !ok {
				return nil, true, errorf("evaluator: attr_reader: expected Symbol or String, got %T", a)
			}
			cls.Methods[n] = makeAttrReader(n)
		}
		return object.NIL, true, nil
	case "attr_writer":
		for _, a := range args {
			n, ok := symbolOrString(env, a)
			if !ok {
				return nil, true, errorf("evaluator: attr_writer: expected Symbol or String, got %T", a)
			}
			cls.Methods[n+"="] = makeAttrWriter(n)
		}
		return object.NIL, true, nil
	case "attr_accessor":
		for _, a := range args {
			n, ok := symbolOrString(env, a)
			if !ok {
				return nil, true, errorf("evaluator: attr_accessor: expected Symbol or String, got %T", a)
			}
			cls.Methods[n] = makeAttrReader(n)
			cls.Methods[n+"="] = makeAttrWriter(n)
		}
		return object.NIL, true, nil
	case "include":
		for _, a := range args {
			mod, ok := a.(*object.Class)
			if !ok {
				return nil, true, errorf("evaluator: include: expected Module, got %T", a)
			}
			cls.Includes = append(cls.Includes, mod)
		}
		return object.NIL, true, nil
	case "private", "public", "protected":
		// Visibility modifiers: silently accept; we don't enforce
		// visibility yet.
		return object.NIL, true, nil
	}
	return nil, false, nil
}

func symbolOrString(env *object.Environment, o object.RubyObject) (string, bool) {
	switch v := o.(type) {
	case *object.Symbol:
		return env.Symbols().Name(v.ID), true
	case *object.String:
		return string(v.Buf), true
	case *object.FrozenString:
		return env.Strings().Get(v.ID), true
	}
	return "", false
}

// makeAttrReader / makeAttrWriter are stored on a Class's Methods map
// and recognised by callMethod via a marker type so dispatch can
// invoke the synthesised behaviour without parsing a body.
func makeAttrReader(name string) *object.UserMethod {
	return &object.UserMethod{Name: name, Body: attrReaderMarker(name)}
}

func makeAttrWriter(name string) *object.UserMethod {
	return &object.UserMethod{Name: name + "=", Body: attrWriterMarker(name)}
}

type attrReaderMarker string
type attrWriterMarker string

// instanceEqual resolves `==` on an Instance receiver: dispatches to a
// user-defined `==` or `<=>` if present, falls back to Go pointer
// identity (matching MRI's Object#==).
func instanceEqual(env *object.Environment, inst *object.Instance, left, right object.RubyObject) (object.RubyObject, error) {
	if _, found := inst.C.LookupMethod("=="); found {
		return callMethod(env, left, "==", []object.RubyObject{right})
	}
	if _, found := inst.C.LookupMethod("<=>"); found {
		return callMethod(env, left, "==", []object.RubyObject{right})
	}
	return object.BooleanOf(left == right), nil
}

// comparableFromSpaceship derives the standard Comparable methods
// (`<`, `<=`, `>`, `>=`, `==`, `between?`) from a `<=>` defined on
// the receiver's class. Mirrors what `include Comparable` does in MRI
// without requiring users to explicitly mix it in.
func comparableFromSpaceship(env *object.Environment, inst *object.Instance, name string, args []object.RubyObject) (object.RubyObject, bool, error) {
	switch name {
	case "<", "<=", ">", ">=", "==":
		if len(args) != 1 {
			return nil, false, nil
		}
	case "between?", "clamp":
		if len(args) != 2 {
			return nil, false, nil
		}
	default:
		return nil, false, nil
	}

	m, ok := inst.C.LookupMethod("<=>")
	if !ok {
		return nil, false, nil
	}
	um, ok := m.(*object.UserMethod)
	if !ok {
		return nil, false, nil
	}

	spaceship := func(other object.RubyObject) (int64, error) {
		v, err := invokeMethodOn(env, inst, um, []object.RubyObject{other}, nil)
		if err != nil {
			return 0, err
		}
		i, ok := v.(*object.Integer)
		if !ok {
			return 0, errorf("evaluator: <=> returned %T, expected Integer", v)
		}
		return i.Value, nil
	}

	if name == "between?" {
		lo, err := spaceship(args[0])
		if err != nil {
			return nil, true, err
		}
		hi, err := spaceship(args[1])
		if err != nil {
			return nil, true, err
		}
		return object.BooleanOf(lo >= 0 && hi <= 0), true, nil
	}
	if name == "clamp" {
		loCmp, err := spaceship(args[0])
		if err != nil {
			return nil, true, err
		}
		if loCmp < 0 {
			return args[0], true, nil
		}
		hiCmp, err := spaceship(args[1])
		if err != nil {
			return nil, true, err
		}
		if hiCmp > 0 {
			return args[1], true, nil
		}
		return inst, true, nil
	}

	c, err := spaceship(args[0])
	if err != nil {
		return nil, true, err
	}
	switch name {
	case "<":
		return object.BooleanOf(c < 0), true, nil
	case "<=":
		return object.BooleanOf(c <= 0), true, nil
	case ">":
		return object.BooleanOf(c > 0), true, nil
	case ">=":
		return object.BooleanOf(c >= 0), true, nil
	case "==":
		return object.BooleanOf(c == 0), true, nil
	}
	return nil, false, nil
}

// classOf returns the receiver's ruby class as a RubyObject suitable
// for `Object#class`. Falls back to the predefined Object class for
// receivers we don't yet model with their own Class.
func classOf(env *object.Environment, recv object.RubyObject) object.RubyObject {
	if c := classOfRaw(env, recv); c != nil {
		return c
	}
	return bootstrapObjectClass(env)
}

// classOfRaw is the typed form used by is_a? walking. Returns nil if
// the value has no associated Class yet (e.g. literal Integer).
func classOfRaw(env *object.Environment, recv object.RubyObject) *object.Class {
	switch r := recv.(type) {
	case *object.Instance:
		return r.C
	case *object.Class:
		// `Foo.class` is `Class` in MRI; we don't model the Class
		// metaclass yet, so return nil and let callers fall back.
		return nil
	case *object.Integer:
		return lookupCoreClass(env, "Integer")
	case *object.Float:
		return lookupCoreClass(env, "Float")
	case *object.String, *object.FrozenString:
		return lookupCoreClass(env, "String")
	case *object.Symbol:
		return lookupCoreClass(env, "Symbol")
	case *object.Array:
		return lookupCoreClass(env, "Array")
	case *object.Hash:
		return lookupCoreClass(env, "Hash")
	case *object.Range:
		return lookupCoreClass(env, "Range")
	case *object.Nil:
		return lookupCoreClass(env, "NilClass")
	case *object.Boolean:
		if r.Value {
			return lookupCoreClass(env, "TrueClass")
		}
		return lookupCoreClass(env, "FalseClass")
	}
	return bootstrapObjectClass(env)
}

func lookupCoreClass(env *object.Environment, name string) *object.Class {
	if v, ok := env.Get(name); ok {
		if c, ok := v.(*object.Class); ok {
			return c
		}
	}
	return bootstrapObjectClass(env)
}

// callOnClass dispatches a method call where the receiver is a Class
// object: `Foo.new`, `Foo.kind`, etc.
func callOnClass(env *object.Environment, cls *object.Class, name string, args []object.RubyObject) (object.RubyObject, bool, error) {
	if cls.Name == "Data" && name == "define" {
		v, err := dataDefine(env, args)
		return v, true, err
	}
	if cls.Name == "Struct" && name == "new" {
		v, err := structDefine(env, args)
		return v, true, err
	}
	if name == "new" {
		// Built-in core classes: their `.new` constructs the
		// corresponding primitive rather than dispatching to a user
		// `initialize`.
		switch cls.Name {
		case "Array":
			v, err := arrayClassNew(args)
			return v, true, err
		case "Hash":
			h := object.NewHash()
			if len(args) == 1 {
				h.Default = args[0]
			}
			return h, true, nil
		case "String":
			if len(args) == 0 {
				return object.NewString(""), true, nil
			}
			if s, ok := stringText(env, args[0]); ok {
				return object.NewString(s), true, nil
			}
			return nil, true, errorf("evaluator: String.new arg must be a String")
		}
		v, err := classNew(env, cls, args)
		return v, true, err
	}
	if name == "superclass" {
		if cls.Super != nil {
			return cls.Super, true, nil
		}
		return object.NIL, true, nil
	}
	if name == "name" {
		return object.NewString(cls.Name), true, nil
	}
	if m, found := cls.LookupClassMethod(name); found {
		if um, ok := m.(*object.UserMethod); ok {
			v, err := invokeMethodOn(env, cls, um, args, nil)
			return v, true, err
		}
	}
	return nil, false, nil
}

// classNew creates an Instance of cls, then calls `initialize` if cls
// defines one, threading args through.
func classNew(env *object.Environment, cls *object.Class, args []object.RubyObject) (object.RubyObject, error) {
	inst := object.NewInstance(cls)
	if m, found := cls.LookupMethod("initialize"); found {
		if um, ok := m.(*object.UserMethod); ok {
			if _, err := invokeMethodOn(env, inst, um, args, nil); err != nil {
				return nil, err
			}
		}
	}
	return inst, nil
}

// invokeMethodOnWithBlock is the block-carrying form of invokeMethodOn.
func invokeMethodOnWithBlock(env *object.Environment, recv object.RubyObject, m *object.UserMethod, args []object.RubyObject, blk *ast.BlockExpression) (object.RubyObject, error) {
	return invokeMethodOn(env, recv, m, args, blk)
}

// invokeMethodOn binds self to recv and calls the user method with the
// given args + optional block.
func invokeMethodOn(env *object.Environment, recv object.RubyObject, m *object.UserMethod, args []object.RubyObject, blk any) (object.RubyObject, error) {
	defEnv, _ := m.DefEnv.(*object.Environment)
	if defEnv == nil {
		defEnv = env
	}
	callEnv := object.NewEnclosedEnvironment(defEnv)
	callEnv.MethodFrame = true
	callEnv.Self = recv
	callEnv.CurrentMethodName = m.Name
	callEnv.CurrentMethodArgs = args
	callEnv.CurrentKwargs = env.CurrentKwargs
	if blk != nil {
		callEnv.CurrentBlock = blk
	}
	if inst, ok := recv.(*object.Instance); ok {
		callEnv.CurrentClass = inst.C
	}
	if cls, ok := recv.(*object.Class); ok {
		callEnv.CurrentClass = cls
	}
	return runMethodBody(callEnv, m, args)
}
