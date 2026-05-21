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

	cls := lookupOrCreateNestedClass(env, name, super, false)

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

// lookupOrCreateNestedClass is lookupOrCreateClass that also honours
// the enclosing-class scope: when called inside `module Outer ; class
// Inner ; end ; end`, the new class is also registered as
// Outer::Inner (in addition to the global namespace, since mri
// effectively does that too -- the global is the canonical home but
// nested constants can be looked up via the outer's Constants table).
func lookupOrCreateNestedClass(env *object.Environment, name string, super *object.Class, isModule bool) *object.Class {
	enclosing := env.EnclosingClass()
	if enclosing != nil {
		if existing, ok := enclosing.Constants[name]; ok {
			if c, ok := existing.(*object.Class); ok {
				return c
			}
		}
	}
	cls := lookupOrCreateClass(env, name, super)
	if isModule {
		cls.IsModule = true
	}
	if enclosing != nil {
		enclosing.Constants[name] = cls
	}
	return cls
}

// evalModuleExpression treats a module as a class with IsModule=true
// and no superclass. Method lookup walks Includes so module methods
// reach instances of including classes.
func evalModuleExpression(env *object.Environment, n *ast.ModuleExpression) (object.RubyObject, error) {
	mod := lookupOrCreateNestedClass(env, n.Name.Value, nil, true)

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
// root env, or creates and stores a fresh one. User classes without an
// explicit superclass default to Object so universal methods (send,
// method, is_a?, ...) are reachable via Send's chain walk.
func lookupOrCreateClass(env *object.Environment, name string, super *object.Class) *object.Class {
	if existing, ok := env.Get(name); ok {
		if c, ok := existing.(*object.Class); ok {
			return c
		}
	}
	if super == nil {
		super = object.ObjectClass
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

// bootstrapObjectClass returns the predefined `Object` class. The class
// itself is the package-level object.ObjectClass; this function only
// ensures it's bound in env so ruby code can reach it by name.
func bootstrapObjectClass(env *object.Environment) *object.Class {
	if _, ok := env.Get("Object"); !ok {
		env.SetGlobal("Object", object.ObjectClass)
		env.SetGlobal("BasicObject", object.BasicObjectClass)
		env.SetGlobal("Numeric", object.NumericClass)
		env.SetGlobal("Integer", object.IntegerClass)
		env.SetGlobal("Float", object.FloatClass)
		env.SetGlobal("String", object.StringClass)
		env.SetGlobal("Symbol", object.SymbolClass)
		env.SetGlobal("Array", object.ArrayClass)
		env.SetGlobal("Hash", object.HashClass)
		env.SetGlobal("Proc", object.ProcClass)
		env.SetGlobal("Range", object.RangeClass)
		env.SetGlobal("NilClass", object.NilClassClass)
		env.SetGlobal("TrueClass", object.TrueClassClass)
		env.SetGlobal("FalseClass", object.FalseClassClass)
		env.SetGlobal("Module", object.ModuleClass)
		env.SetGlobal("Class", object.ClassClass)
	}
	return object.ObjectClass
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
	// Forward the block: an explicit `super(args) do ... end` passes a
	// fresh literal block; a bare `super` (no explicit block / args)
	// forwards the current method's block transparently. Wrap in a
	// goBlockMarker that closes over the current env, so the block's
	// lexical scope (the calling method's locals -- e.g. an outer
	// `name` parameter) remains reachable when the called method
	// later invokes the block.
	var blockArg any
	if n.Block != nil {
		blk := n.Block
		callerEnv := env
		blockArg = &goBlockMarker{
			fn:  func(a []object.RubyObject) (object.RubyObject, error) { return invokeBlock(callerEnv, blk, a) },
			blk: blk,
		}
	} else if n.Arguments == nil {
		if cb := env.EnclosingBlock(); cb != nil {
			blockArg = cb
		}
	}
	return invokeMethodOn(env, inst, um, args, blockArg)
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

// dispatchClass returns the class to consult for method lookup on
// recv. Today it is exactly classOfRaw, but every method-dispatch site
// must route through here so that the future eigenclass machinery
// (singleton classes attached to individual objects) can be wired in
// at this one chokepoint instead of being grepped into every call
// site.
func dispatchClass(env *object.Environment, recv object.RubyObject) *object.Class {
	return classOfRaw(env, recv)
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

// classOfRaw returns the dispatch class for recv. Every builtin type
// returns its package-level class from .Class(), so this reads off the
// value directly. Falls back to ObjectClass for receivers whose
// Class() returns nil (Regex, UserMethod, ...).
func classOfRaw(_ *object.Environment, recv object.RubyObject) *object.Class {
	if c := recv.Class(); c != nil {
		return c
	}
	return object.ObjectClass
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

func init() {
	// Wire the object-package UserMethod.Call hook so dispatchers that
	// only have a RubyMethod can run user code w/out a back-import.
	object.UserMethodInvoker = func(env *object.Environment, recv object.RubyObject, m *object.UserMethod, args []object.RubyObject, block any) (object.RubyObject, error) {
		return invokeMethodOn(env, recv, m, args, block)
	}
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
	// Switch CurrentFile to the method's defining source so errors
	// raised inside the body report against the def's file, not
	// whichever require_relative chain happened to land there.
	if m.SourceFile != "" {
		prev := env.SetCurrentFile(m.SourceFile)
		defer env.SetCurrentFile(prev)
	}
	return runMethodBody(callEnv, m, args)
}
