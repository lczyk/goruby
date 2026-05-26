package evaluator

import (
	"strings"

	"github.com/lczyk/goruby/ast"
	"github.com/lczyk/goruby/evaluator/builtinapi"
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

	// Track whether this is a fresh-create vs reopen so the inherited
	// hook only fires on initial creation, matching MRI semantics.
	preexisting := classExists(env, name)
	cls := lookupOrCreateNestedClass(env, name, super, false)

	bodyEnv := object.NewEnclosedEnvironment(env)
	bodyEnv.CurrentClass = cls
	bodyEnv.Self = cls

	// Fire super.inherited(cls) hook on first creation when the
	// superclass defines one. MRI's contract for class introspection
	// and DSL frameworks (rspec, rails, minitest's own Runnable
	// subclass-registry).
	if !preexisting && super != nil {
		if m, found := super.LookupClassMethod("inherited"); found {
			switch mm := m.(type) {
			case *object.UserMethod:
				if _, err := invokeMethodOn(env, super, mm, []object.RubyObject{cls}, nil); err != nil {
					return nil, err
				}
			case *object.BuiltinMethod:
				if _, err := mm.Fn(env, super, []object.RubyObject{cls}, nil); err != nil {
					return nil, err
				}
			}
		}
	}

	if n.Body != nil {
		if _, err := evalBlockStatement(bodyEnv, n.Body); err != nil {
			return nil, err
		}
	}
	return cls, nil
}

// classExists returns true if name resolves to a Class in env's chain
// (including enclosing-class constants and the global env), used to
// distinguish reopen from fresh-create for hook routing.
func classExists(env *object.Environment, name string) bool {
	if strings.Contains(name, "::") {
		return false
	}
	if encl := env.EnclosingClass(); encl != nil {
		if _, ok := encl.Constants[name].(*object.Class); ok {
			return true
		}
	}
	if v, ok := env.Get(name); ok {
		_, isClass := v.(*object.Class)
		return isClass
	}
	return false
}

// lookupOrCreateNestedClass is lookupOrCreateClass that also honours
// the enclosing-class scope: when called inside `module Outer ; class
// Inner ; end ; end`, the new class is also registered as
// Outer::Inner (in addition to the global namespace, since mri
// effectively does that too -- the global is the canonical home but
// nested constants can be looked up via the outer's Constants table).
func lookupOrCreateNestedClass(env *object.Environment, name string, super *object.Class, isModule bool) *object.Class {
	// Scoped name (Outer::Inner): resolve Outer first, then create /
	// reopen Inner inside its Constants. Handles arbitrary nesting.
	// Outermost name still pulls from the global env when no in-scope
	// class has it.
	if strings.Contains(name, "::") {
		parts := strings.Split(name, "::")
		var parent *object.Class
		// Outermost: try EnclosingClass then global.
		head := parts[0]
		if encl := env.EnclosingClass(); encl != nil {
			if c, ok := encl.Constants[head].(*object.Class); ok {
				parent = c
			}
		}
		if parent == nil {
			if v, ok := env.Get(head); ok {
				if c, ok := v.(*object.Class); ok {
					parent = c
				}
			}
		}
		if parent == nil {
			// Outermost not found -- create as a global class so the
			// next part can hang off it. Mirrors MRI's autovivify of
			// `class X::Y` when X happens to be undefined (actually
			// MRI raises here; ours autocreates for robustness against
			// require-order ambiguities).
			parent = lookupOrCreateClass(env, head, nil)
		}
		for i := 1; i < len(parts)-1; i++ {
			child, ok := parent.Constants[parts[i]].(*object.Class)
			if !ok {
				child = object.NewClass(parts[i], nil)
				child.IsModule = true
				child.Parent = parent
				parent.Constants[parts[i]] = child
			}
			parent = child
		}
		leaf := parts[len(parts)-1]
		if c, ok := parent.Constants[leaf].(*object.Class); ok {
			if isModule {
				c.IsModule = true
			}
			if c.Parent == nil {
				c.Parent = parent
			}
			return c
		}
		cls := object.NewClass(leaf, super)
		if isModule {
			cls.IsModule = true
		}
		cls.Parent = parent
		parent.Constants[leaf] = cls
		return cls
	}
	enclosing := env.EnclosingClass()
	if enclosing != nil {
		if existing, ok := enclosing.Constants[name]; ok {
			if c, ok := existing.(*object.Class); ok {
				return c
			}
		}
	}
	// Reopen a global class/module by name if one exists. Lets
	// `module Foo` inside any scope find an existing top-level Foo
	// (matching MRI's nesting-then-Object lookup for the constant
	// receiver of a class def).
	if existing, ok := env.Get(name); ok {
		if c, ok := existing.(*object.Class); ok {
			if enclosing != nil {
				enclosing.Constants[name] = c
			}
			return c
		}
	}
	// New class. Create under enclosing (no global bleed) when
	// nested; bind globally for top-level defs.
	cls := object.NewClass(name, super)
	if isModule {
		cls.IsModule = true
	}
	if enclosing != nil {
		cls.Parent = enclosing
		enclosing.Constants[name] = cls
	} else {
		env.SetGlobal(name, cls)
	}
	return cls
}

// evalSingletonClassExpression handles `class << expr ... end`. Methods
// defined in the body install onto the host's singleton table rather
// than the enclosing class's instance methods:
//
//   - Class host (incl. `class << self` inside a class body) -> the
//     host's ClassMethods (== `def self.foo`).
//   - Instance host -> the receiver's per-object SingletonMethods.
//
// We don't materialise a separate singleton class object; the body
// runs with SingletonHost set on the env so evalFunctionLiteral routes
// the def accordingly. Returns the host as a proxy for the singleton
// class -- close enough for `class << obj; def x; end; end` idioms.
func evalSingletonClassExpression(env *object.Environment, n *ast.SingletonClassExpression) (object.RubyObject, error) {
	host, err := Eval(n.Expr, env)
	if err != nil {
		return nil, err
	}
	switch host.(type) {
	case *object.Class, *object.Instance:
		// supported
	default:
		return nil, errorf("evaluator: class << on %T not yet supported", host)
	}
	bodyEnv := object.NewEnclosedEnvironment(env)
	bodyEnv.SingletonHost = host
	// For Class hosts, Self inside the body becomes the materialised
	// singleton class object -- matches MRI. Lets the eigenclass-as-
	// value idiom `(class << host; self; end).attr_accessor :x`
	// reach the singleton class for attr_* installation.
	// For Instance hosts the existing host-as-self stays (the
	// per-object SingletonMethods path is unchanged).
	selfForBody := host
	if cls, ok := host.(*object.Class); ok {
		selfForBody = cls.EnsureClassSingleton()
	}
	bodyEnv.Self = selfForBody
	if n.Body != nil {
		if _, err := evalBlockStatement(bodyEnv, n.Body); err != nil {
			return nil, err
		}
	}
	// Return the singleton class for Class hosts so the
	// caller can chain attr_* / def via the returned value
	// (the cattr_accessor idiom).
	return selfForBody, nil
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
		innerID, ok := n.Inner.(*ast.Identifier)
		if !ok {
			return nil, errorf("evaluator: unsupported ::Inner %T", n.Inner)
		}
		if v, ok := env.GetGlobal(innerID.Value); ok {
			return v, nil
		}
		return nil, errorf("evaluator: NameError: uninitialized constant ::%s", innerID.Value)
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
	if v, ok := lookupConstant(cls, innerID.Value); ok {
		return v, nil
	}
	// const_missing hook: if cls (or an ancestor) defines
	// const_missing as a class method, dispatch it with the missing
	// constant name as a Symbol arg.
	if m, found := cls.LookupClassMethod("const_missing"); found {
		switch mm := m.(type) {
		case *object.UserMethod:
			return invokeMethodOn(env, cls, mm, []object.RubyObject{env.Symbols().Intern(innerID.Value)}, nil)
		case *object.BuiltinMethod:
			return mm.Fn(env, cls, []object.RubyObject{env.Symbols().Intern(innerID.Value)}, nil)
		}
	}
	return raiseBuiltin(env, "NameError", "uninitialized constant "+cls.Name+"::"+innerID.Value)
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
	key := "@" + n.Name.Value
	switch self := env.EnclosingSelf().(type) {
	case *object.Instance:
		if v, ok := self.Ivars[key]; ok {
			return v, nil
		}
	case *object.Class:
		// Class-instance variables: @var at class scope (or in a
		// `def self.foo` method) attaches to the class object itself.
		if v, ok := self.Ivars[key]; ok {
			return v, nil
		}
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
		// Default Object.inherited as a noop. Lets user-defined
		// inherited hooks call super without crashing the chain.
		object.ObjectClass.ClassMethods["inherited"] = &object.BuiltinMethod{
			Name: "inherited",
			Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
				return object.NIL, nil
			},
		}
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
	cls := findCallClass(env)
	if cls == nil || cls.Super == nil {
		return raiseBuiltin(env, "NoMethodError", "super: no superclass method `"+name+"'")
	}
	// Class-method super: self is the Class itself. Walk from the
	// *defining* class's super (not self's super) so the hook chain
	// climbs through where the method was actually defined. Matches
	// MRI semantics for inherited / included / extended hooks where
	// super has to keep climbing from the definition site, not the
	// dispatch receiver.
	if selfCls, ok := self.(*object.Class); ok {
		if cls.Super == nil {
			return raiseBuiltin(env, "NoMethodError", "super: no superclass method `"+name+"'")
		}
		m, found := cls.Super.LookupClassMethod(name)
		if !found {
			return raiseBuiltin(env, "NoMethodError", "super: no superclass method `"+name+"'")
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
		switch mm := m.(type) {
		case *object.UserMethod:
			return invokeMethodOn(env, selfCls, mm, args, nil)
		case *object.BuiltinMethod:
			return mm.Fn(env, selfCls, args, nil)
		}
		return nil, errorf("evaluator: super on non-user method %T", m)
	}
	// Instance / builtin path. self may be *Instance, *Integer,
	// *String, etc. Walk the receiver's linearised MRO and find the
	// next class after the defining class -- that's where super
	// resumes. Plain Super-chain walk is insufficient when the
	// defining class is an included Module: after Mix in
	// (Sub -> Mix -> Base -> Object), MRI continues to Base, not to
	// Mix.Super (Object) which is what a naive walk would pick.
	m, found := lookupSuperMRO(env, self, cls, name)
	if !found {
		return raiseBuiltin(env, "NoMethodError", "super: no superclass method `"+name+"'")
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
	if um, ok := m.(*object.UserMethod); ok {
		return invokeMethodOn(env, self, um, args, blockArg)
	}
	return m.Call(env, self, args, blockArg)
}

// lookupSuper finds the next implementation of name above the class
// where the current method lives. Walks the defining class's Includes
// first, then climbs its Super chain (each step consults that class's
// Methods + Includes via LookupMethod). Returns the matched method,
// false when nothing further up defines name.
func lookupSuper(defining *object.Class, name string) (object.RubyMethod, bool) {
	for _, inc := range defining.Includes {
		if m, ok := inc.LookupMethod(name); ok {
			return m, true
		}
	}
	if defining.Super != nil {
		return defining.Super.LookupMethod(name)
	}
	return nil, false
}

// lookupSuperMRO finds the next implementation of name in the
// receiver's linearised MRO past the defining class. Compute MRO by
// walking recv.Class()'s super chain, splicing each class's Includes
// (in declaration order) just after the class. Then scan the list
// starting one past the index of defining; pick the first class with
// a matching own method.
func lookupSuperMRO(env *object.Environment, recv object.RubyObject, defining *object.Class, name string) (object.RubyMethod, bool) {
	rootCls := classOfRaw(env, recv)
	if rootCls == nil {
		return lookupSuper(defining, name)
	}
	mro := []*object.Class{}
	for cur := rootCls; cur != nil; cur = cur.Super {
		mro = append(mro, cur)
		mro = append(mro, cur.Includes...)
	}
	startIdx := -1
	for i, c := range mro {
		if c == defining {
			startIdx = i
			break
		}
	}
	if startIdx < 0 {
		// defining class isn't in the receiver's MRO -- happens for
		// some metaclass shapes. Fall back to the old chain walk so
		// the lookup doesn't silently return nothing.
		return lookupSuper(defining, name)
	}
	for i := startIdx + 1; i < len(mro); i++ {
		if m, ok := mro[i].Methods[name]; ok {
			return m, true
		}
	}
	return nil, false
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

// singletonBodyDSL handles the class-level helpers used inside a
// `class << host` body. Currently covers attr_reader / attr_writer /
// attr_accessor -- they synthesise methods on host's singleton-method
// table so that `host.name` / `host.name=` resolve. For a Class host
// the singleton table is its ClassMethods (mirrors `def self.foo`);
// for an Instance host it's the per-object SingletonMethods.
//
// `include` inside an eigenclass body would mix into the singleton
// class -- not supported yet, returns handled=false so dispatch falls
// through to the regular call path (which currently errors -- the
// idiom is rare enough not to chase).
func singletonBodyDSL(env *object.Environment, host object.RubyObject, name string, args []object.RubyObject) (object.RubyObject, bool, error) {
	switch name {
	case "attr_reader":
		for _, a := range args {
			n, ok := symbolOrString(env, a)
			if !ok {
				return nil, true, errorf("evaluator: attr_reader: expected Symbol or String, got %T", a)
			}
			if err := installSingletonMethod(host, n, makeAttrReader(n)); err != nil {
				return nil, true, err
			}
		}
		return object.NIL, true, nil
	case "attr_writer":
		for _, a := range args {
			n, ok := symbolOrString(env, a)
			if !ok {
				return nil, true, errorf("evaluator: attr_writer: expected Symbol or String, got %T", a)
			}
			if err := installSingletonMethod(host, n+"=", makeAttrWriter(n)); err != nil {
				return nil, true, err
			}
		}
		return object.NIL, true, nil
	case "attr_accessor":
		for _, a := range args {
			n, ok := symbolOrString(env, a)
			if !ok {
				return nil, true, errorf("evaluator: attr_accessor: expected Symbol or String, got %T", a)
			}
			if err := installSingletonMethod(host, n, makeAttrReader(n)); err != nil {
				return nil, true, err
			}
			if err := installSingletonMethod(host, n+"=", makeAttrWriter(n)); err != nil {
				return nil, true, err
			}
		}
		return object.NIL, true, nil
	}
	return nil, false, nil
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
			// MRI calls `mod.included(base)` after a successful
			// include, if the hook is defined. PrivateReader-style
			// extension uses this to copy class methods over to the
			// includer. Best-effort: only invoke when the hook is
			// available on mod's ClassMethods (mri also looks at the
			// singleton class, equivalent here).
			if hook, ok := mod.ClassMethods["included"]; ok {
				if _, err := hook.Call(env, mod, []object.RubyObject{cls}, nil); err != nil {
					return nil, true, err
				}
			}
		}
		return object.NIL, true, nil
	case "private", "public", "protected":
		// Bare form (no args) flips the body's visibility mode -- the
		// Identifier path in evalIdentifier handles that and we never
		// reach here. With args, retroactively mark each named method
		// with the requested visibility, leaving CurrentVisibility
		// alone (matches MRI: `private :foo` does not change subsequent
		// def visibility).
		if len(args) == 0 {
			switch name {
			case "private":
				cls.CurrentVisibility = "private"
			case "public":
				cls.CurrentVisibility = "public"
			case "protected":
				cls.CurrentVisibility = "protected"
			}
			return object.NIL, true, nil
		}
		for _, a := range args {
			mname, ok := symbolOrString(env, a)
			if !ok {
				return nil, true, errorf("evaluator: %s: expected Symbol or String, got %T", name, a)
			}
			switch name {
			case "private":
				if cls.Private == nil {
					cls.Private = map[string]bool{}
				}
				cls.Private[mname] = true
				delete(cls.Protected, mname)
			case "protected":
				if cls.Protected == nil {
					cls.Protected = map[string]bool{}
				}
				cls.Protected[mname] = true
				delete(cls.Private, mname)
			case "public":
				delete(cls.Private, mname)
				delete(cls.Protected, mname)
			}
		}
		return object.NIL, true, nil
	case "private_class_method", "public_class_method":
		// Class-method visibility flips. We don't model per-method
		// visibility for ClassMethods, so accept the call and noop.
		// Sufficient for rake/clean.rb's
		// `private_class_method :file_already_gone?` line.
		return object.NIL, true, nil
	}
	return nil, false, nil
}

// symbolOrString aliases builtinapi.SymbolOrString; see string.go for
// the stringText sibling helper.
var symbolOrString = builtinapi.SymbolOrString

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
	// Wrap the literal block as a marker that captures env (the
	// call-site env, where the block was syntactically defined).
	// Otherwise the method body's &block reification later uses
	// callEnv.Outer() == m.DefEnv as the proc's closure env, which
	// is the def-time env (class body), not the call-site env --
	// so self inside the reified block ends up as the class
	// rather than the receiver of the call.
	callerEnv := env
	marker := &goBlockMarker{
		fn: func(a []object.RubyObject) (object.RubyObject, error) {
			return invokeBlock(callerEnv, blk, a)
		},
		blk: blk,
	}
	return invokeMethodOn(env, recv, m, args, marker)
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
	// Prefer the method's defining class (set at def time) so super
	// inside the body climbs strictly later in the ancestry. Crucial
	// for methods that came in via include: a `class Application
	// include TaskManager; def initialize; super; end; end` shape
	// needs CurrentClass == Application initially, then == TaskManager
	// after super dispatches into it -- without DefClass tracking,
	// every callEnv would see the receiver's class (Application) and
	// super would loop back to the same method.
	if m.DefClass != nil {
		callEnv.CurrentClass = m.DefClass
	} else if inst, ok := recv.(*object.Instance); ok {
		callEnv.CurrentClass = inst.C
	} else if cls, ok := recv.(*object.Class); ok {
		callEnv.CurrentClass = cls
	} else if cls := classOfRaw(env, recv); cls != nil {
		// Builtin receivers (Integer, String, ...) need CurrentClass
		// set so super lookups inside the method body have an anchor.
		callEnv.CurrentClass = cls
	}
	// Switch CurrentFile to the method's defining source so errors
	// raised inside the body report against the def's file, not
	// whichever require_relative chain happened to land there.
	if m.SourceFile != "" {
		prev := env.SetCurrentFile(m.SourceFile)
		defer env.SetCurrentFile(prev)
	}
	pushMethodFrame(env, m)
	defer env.PopCallFrame()
	return runMethodBody(callEnv, m, args)
}
