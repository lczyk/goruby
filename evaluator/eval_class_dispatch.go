package evaluator

import (
	"github.com/lczyk/goruby/ast"
	"github.com/lczyk/goruby/object"
	"github.com/lczyk/goruby/parser"
)

// evalClassEvalString parses src as Ruby source and evaluates it
// in a class-body context for cls. `def`s inside install on
// cls.Methods, attr_* helpers go through classBodyDSL, etc.
func evalClassEvalString(env *object.Environment, cls *object.Class, src string) (object.RubyObject, error) {
	prog, err := parser.ParseFile("(class_eval)", []byte(src), 0, parser.WithVersion(env.Version()))
	if err != nil {
		return nil, errorf("evaluator: class_eval: parse: %s", err.Error())
	}
	bodyEnv := object.NewEnclosedEnvironment(env)
	bodyEnv.CurrentClass = cls
	bodyEnv.Self = cls
	return evalBlockStatement(bodyEnv, &ast.BlockStatement{Statements: prog.Statements})
}

// evalClassEvalBlock runs the block with self rebound to cls. Methods
// defined inside still install via the def routing (which sees
// EnclosingClass via the bodyEnv).
func evalClassEvalBlock(env *object.Environment, cls *object.Class, bm *goBlockMarker) (object.RubyObject, error) {
	bodyEnv := object.NewEnclosedEnvironment(env)
	bodyEnv.CurrentClass = cls
	bodyEnv.Self = cls
	if bm.blk != nil {
		return evalBlockStatement(bodyEnv, bm.blk.Body)
	}
	// Proc-shaped marker: recover the underlying BlockExpression via
	// proc.Params (which is a *goBlockMarker carrying the original
	// AST when the proc came from a literal block via procFromGoBlock).
	if bm.proc != nil {
		if inner, ok := bm.proc.Params.(*goBlockMarker); ok && inner != nil && inner.blk != nil {
			return evalBlockStatement(bodyEnv, inner.blk.Body)
		}
		// Procs from procFromBlock store body directly.
		if body, ok := bm.proc.Body.(*ast.BlockStatement); ok {
			return evalBlockStatement(bodyEnv, body)
		}
	}
	// Procs without an attached AST -- just invoke via the marker's fn.
	return bm.fn(nil)
}

// Class-receiver dispatch migrated onto object.ModuleClass. A Class
// object's recv.Class() returns ClassClass for a regular class and
// ModuleClass for a module; ClassClass.Super = ModuleClass, so
// registering on ModuleClass makes these visible on both, matching
// the IsModule-agnostic behaviour of the previous callOnClass.

func init() {
	c := object.ModuleClass

	add := func(name string, fn func(env *object.Environment, cls *object.Class, args []object.RubyObject) (object.RubyObject, error)) {
		c.AddMethod(name, &object.BuiltinMethod{
			Name: name,
			Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
				cls, ok := recv.(*object.Class)
				if !ok {
					return nil, errorf("evaluator: Class#%s called on non-Class receiver %T", name, recv)
				}
				return fn(env, cls, args)
			},
		})
	}

	// `new` is block-aware: Proc.new / Hash.new / Array.new (sized) all
	// behave differently when a block is present, and user-class `new`
	// must forward the block to `initialize` so the body's `block_given?`
	// / `yield` / `&blk` capture work.
	c.AddMethod("new", &object.BuiltinMethod{
		Name: "new",
		Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, blockAny any) (object.RubyObject, error) {
			cls, ok := recv.(*object.Class)
			if !ok {
				return nil, errorf("evaluator: Class#new called on non-Class receiver %T", recv)
			}
			var invoke blockCallback
			var blk *ast.BlockExpression
			if bm, ok := blockAny.(*goBlockMarker); ok && bm != nil {
				invoke = bm.fn
				blk = bm.blk
			}
			if invoke != nil {
				switch cls.Name {
				case "Class":
					// Class.new(super?) { body } -- create a fresh
					// anonymous class and class_eval the body on it.
					// MRI minitest/spec's create method uses this
					// shape: `Class.new(self) do @name = ... end`.
					newCls := object.NewClass("", nil)
					if len(args) == 1 {
						if sup, ok := args[0].(*object.Class); ok {
							newCls.Super = sup
						}
					}
					if blk != nil {
						marker := &goBlockMarker{fn: invoke, blk: blk}
						if _, err := evalClassEvalBlock(env, newCls, marker); err != nil {
							return nil, err
						}
					}
					return newCls, nil
				case "Proc":
					if blk == nil {
						return nil, errorf("evaluator: Proc.new needs a literal block")
					}
					return procFromBlock(env, blk), nil
				case "Hash":
					h := object.NewHash()
					if blk != nil {
						h.DefaultBlock = procFromBlock(env, blk)
					}
					return h, nil
				case "Enumerator":
					// Eager-evaluation Enumerator.new { |y| ... }: run the
					// block now with a collecting yielder; the resulting
					// Enumerator wraps the buffered values. The yielder is
					// a Proc -- the bouncy idiom `each_point(&yielder)`
					// then forwards it as the block to each_point, which
					// invokes it via block.call(point), appending to the
					// buffer.
					collected := []object.RubyObject{}
					yielder := procFromGoBlock(&goBlockMarker{fn: func(a []object.RubyObject) (object.RubyObject, error) {
						switch len(a) {
						case 0:
						case 1:
							collected = append(collected, a[0])
						default:
							collected = append(collected, object.NewArray(a...))
						}
						return object.NIL, nil
					}})
					if _, err := invoke([]object.RubyObject{yielder}); err != nil {
						return nil, err
					}
					return &object.Enumerator{Receiver: object.NewArray(collected...), Method: "each"}, nil
				case "Array":
					if len(args) != 1 {
						return nil, errorf("evaluator: Array.new { ... } expects 1 size arg")
					}
					n, ok := args[0].(*object.Integer)
					if !ok {
						return nil, errorf("evaluator: Array.new size must be Integer")
					}
					if n.Value < 0 {
						return nil, errorf("evaluator: ArgumentError: negative array size")
					}
					out := make([]object.RubyObject, 0, n.Value)
					for i := int64(0); i < n.Value; i++ {
						v, stop, err := iterStep(invoke, []object.RubyObject{object.NewInteger(i)})
						if err != nil {
							return nil, err
						}
						if stop {
							return object.NewArray(out...), nil
						}
						out = append(out, v)
					}
					return object.NewArray(out...), nil
				}
				// User-class .new { ... }: instantiate, then forward to
				// initialize when defined so the block is visible via
				// `block_given?` / `yield` / `&blk` capture. Pass the
				// goBlockMarker itself when there's no literal block
				// shape (e.g. `Cls.new(&proc)` from a Proc capture) --
				// invokeMethodOn handles either flavour and that's
				// what initialize's &block parameter needs to see.
				inst := object.NewInstance(cls)
				if m, found := cls.LookupMethod("initialize"); found {
					if um, ok := m.(*object.UserMethod); ok {
						var blockArg any = blk
						if blk == nil {
							blockArg = blockAny
						}
						if _, err := invokeMethodOn(env, inst, um, args, blockArg); err != nil {
							return nil, err
						}
					}
				}
				return inst, nil
			}
			switch cls.Name {
			case "Class":
				// Class.new([super]) returns a new anonymous Class. The
				// super arg, when given, becomes the new class's Super;
				// otherwise default to Object. Anonymous Name stays "";
				// callers that bind to a constant (e.g.
				// Outer::C = Class.new) get Name + Parent stamped via
				// the scoped-assign path.
				newCls := object.NewClass("", nil)
				if len(args) == 1 {
					if sup, ok := args[0].(*object.Class); ok {
						newCls.Super = sup
					}
				}
				return newCls, nil
			case "Data":
				return dataDefine(env, args)
			case "Struct":
				return structDefine(env, args)
			case "Array":
				return arrayClassNew(args)
			case "Hash":
				h := object.NewHash()
				if len(args) == 1 {
					h.Default = args[0]
				}
				return h, nil
			case "String":
				if len(args) == 0 {
					return object.NewString(""), nil
				}
				if s, ok := stringText(env, args[0]); ok {
					return object.NewString(s), nil
				}
				return nil, errorf("evaluator: String.new arg must be a String")
			}
			return classNew(env, cls, args)
		},
	})

	add("define", func(env *object.Environment, cls *object.Class, args []object.RubyObject) (object.RubyObject, error) {
		if cls.Name != "Data" {
			return raiseBuiltin(env, "NoMethodError", "undefined method `define' for "+cls.Name)
		}
		return dataDefine(env, args)
	})

	add("superclass", func(env *object.Environment, cls *object.Class, args []object.RubyObject) (object.RubyObject, error) {
		if cls.Super != nil {
			return cls.Super, nil
		}
		return object.NIL, nil
	})

	// Module#ancestors returns the linearised ancestry: self, then
	// includes (in declaration order), then super, then super's
	// includes, ... Matches MRI's MRO. rake's define_task uses it for
	// `task_class.ancestors.include?(Rake::FileTask)` to detect file
	// tasks.
	add("ancestors", func(env *object.Environment, cls *object.Class, args []object.RubyObject) (object.RubyObject, error) {
		out := []object.RubyObject{}
		for cur := cls; cur != nil; cur = cur.Super {
			out = append(out, cur)
			for _, inc := range cur.Includes {
				out = append(out, inc)
			}
		}
		return object.NewArray(out...), nil
	})

	add("name", func(env *object.Environment, cls *object.Class, args []object.RubyObject) (object.RubyObject, error) {
		return object.NewString(cls.QualifiedName()), nil
	})
	// Module#to_s / Module#inspect return the class/module name. MRI's
	// inspect distinguishes anonymous classes via #<Class:0x...>; we
	// just return the Name (empty for anonymous), which is what most
	// callers introspect. Module#to_str is also commonly used for
	// implicit String conversion.
	clsToS := func(env *object.Environment, cls *object.Class, args []object.RubyObject) (object.RubyObject, error) {
		return object.NewString(cls.QualifiedName()), nil
	}
	add("to_s", clsToS)
	add("inspect", clsToS)

	// method_defined?(name) -- returns true if name is a public or
	// protected instance method of cls (including inherited via the
	// superclass / include chain). MRI distinguishes private via
	// `private_method_defined?`; until visibility tracking lands we
	// treat every defined method as public-or-protected (the rake
	// rake_extension dsl only cares whether the name is taken).
	mdef := func(env *object.Environment, cls *object.Class, args []object.RubyObject) (object.RubyObject, error) {
		if len(args) < 1 || len(args) > 2 {
			return nil, errorf("evaluator: method_defined?: expected 1..2 args, got %d", len(args))
		}
		name, ok := symbolOrString(env, args[0])
		if !ok {
			return nil, errorf("evaluator: method_defined?: name must be Symbol or String")
		}
		_, found := cls.LookupMethod(name)
		if found {
			return object.TRUE, nil
		}
		return object.FALSE, nil
	}
	add("method_defined?", mdef)

	// attr_reader / attr_writer / attr_accessor as Module instance
	// methods, so calls like `Foo.attr_accessor :bar` (or the
	// minitest `(class << self; self; end).attr_accessor :seed`
	// idiom) install reader/writer methods on the receiver class.
	// Without this, attr_* was only reachable as the bare-name DSL
	// inside a class body.
	addAttr := func(name string, reader, writer bool) {
		add(name, func(env *object.Environment, cls *object.Class, args []object.RubyObject) (object.RubyObject, error) {
			for _, a := range args {
				n, ok := symbolOrString(env, a)
				if !ok {
					return nil, errorf("evaluator: %s: expected Symbol or String, got %T", name, a)
				}
				if reader {
					cls.Methods[n] = makeAttrReader(n)
				}
				if writer {
					cls.Methods[n+"="] = makeAttrWriter(n)
				}
			}
			return object.NIL, nil
		})
	}
	addAttr("attr_reader", true, false)
	addAttr("attr_writer", false, true)
	addAttr("attr_accessor", true, true)

	// undef_method(name) removes name from cls.Methods so dispatch on
	// instances of cls fails with NoMethodError. MRI also installs a
	// stub that raises explicitly; deletion approximates -- callers
	// using respond_to? before invoking still see the absence.
	add("undef_method", func(env *object.Environment, cls *object.Class, args []object.RubyObject) (object.RubyObject, error) {
		for _, a := range args {
			n, ok := symbolOrString(env, a)
			if !ok {
				return nil, errorf("evaluator: undef_method: name must be Symbol or String")
			}
			delete(cls.Methods, n)
		}
		return cls, nil
	})

	// remove_method(name) is similar to undef_method but only deletes
	// from the class itself; inherited definitions remain reachable.
	// Our impl is the same (we don't model "undef inserts a guard"
	// distinct from "remove deletes" -- both just clear the entry).
	add("remove_method", func(env *object.Environment, cls *object.Class, args []object.RubyObject) (object.RubyObject, error) {
		for _, a := range args {
			n, ok := symbolOrString(env, a)
			if !ok {
				return nil, errorf("evaluator: remove_method: name must be Symbol or String")
			}
			delete(cls.Methods, n)
		}
		return cls, nil
	})

	// const_defined?(name, inherit=true) -- does cls have a constant
	// by that name? With inherit=false, only checks own Constants;
	// otherwise walks Super. MRI also looks up globals when cls is
	// Object -- mirror that since rake's Object.const_defined?(:RUBY_ENGINE)
	// shape depends on it.
	add("const_defined?", func(env *object.Environment, cls *object.Class, args []object.RubyObject) (object.RubyObject, error) {
		if len(args) < 1 || len(args) > 2 {
			return nil, errorf("evaluator: const_defined?: expected 1..2 args, got %d", len(args))
		}
		name, ok := symbolOrString(env, args[0])
		if !ok {
			return nil, errorf("evaluator: const_defined?: name must be Symbol or String")
		}
		inherit := true
		if len(args) == 2 {
			if args[1] == object.FALSE || args[1] == object.NIL {
				inherit = false
			}
		}
		check := func(c *object.Class) bool {
			_, ok := c.Constants[name]
			return ok
		}
		if check(cls) {
			return object.TRUE, nil
		}
		if inherit {
			for cur := cls.Super; cur != nil; cur = cur.Super {
				if check(cur) {
					return object.TRUE, nil
				}
			}
			// Object.const_defined? also looks at the global env so
			// top-level constants resolve through it.
			if cls == object.ObjectClass {
				if _, ok := env.GetGlobal(name); ok {
					return object.TRUE, nil
				}
			}
		}
		return object.FALSE, nil
	})

	// const_get(name) -- returns the value of the named constant,
	// walking the class chain. Raises NameError if missing.
	add("const_get", func(env *object.Environment, cls *object.Class, args []object.RubyObject) (object.RubyObject, error) {
		if len(args) < 1 || len(args) > 2 {
			return nil, errorf("evaluator: const_get: expected 1..2 args, got %d", len(args))
		}
		name, ok := symbolOrString(env, args[0])
		if !ok {
			return nil, errorf("evaluator: const_get: name must be Symbol or String")
		}
		if v, ok := lookupConstant(cls, name); ok {
			return v, nil
		}
		if cls == object.ObjectClass {
			if v, ok := env.GetGlobal(name); ok {
				return v, nil
			}
		}
		return raiseBuiltin(env, "NameError", "uninitialized constant "+cls.Name+"::"+name)
	})

	// const_set(name, value) -- sets the named constant on cls.
	add("const_set", func(env *object.Environment, cls *object.Class, args []object.RubyObject) (object.RubyObject, error) {
		if len(args) != 2 {
			return nil, errorf("evaluator: const_set: expected 2 args, got %d", len(args))
		}
		name, ok := symbolOrString(env, args[0])
		if !ok {
			return nil, errorf("evaluator: const_set: name must be Symbol or String")
		}
		if cls.Constants == nil {
			cls.Constants = map[string]object.RubyObject{}
		}
		cls.Constants[name] = args[1]
		return args[1], nil
	})

	// constants -- returns symbol names of all constants on cls.
	add("constants", func(env *object.Environment, cls *object.Class, args []object.RubyObject) (object.RubyObject, error) {
		out := []object.RubyObject{}
		for name := range cls.Constants {
			out = append(out, env.Symbols().Intern(name))
		}
		return object.NewArray(out...), nil
	})

	// instance_methods(inherited=true) -- returns symbol names of
	// instance methods. With inherited=false, only methods defined
	// directly on cls (not via Super or Includes). MRI also filters
	// private methods out; we don't model visibility on Module-level
	// methods yet so we include everything. Good enough for rake's
	// `private(*FileUtils.instance_methods(false))` pattern.
	// class_eval / module_eval -- runtime metaprogramming. Real
	// implementations parse the String arg as ruby source and eval it
	// with self rebound to cls; the block form evals the literal block
	// the same way. Today we don't support either path: the String
	// form would need a parser entrypoint that takes a fragment, and
	// the block form needs the same self-rebind plumbing instance_eval
	// is missing. Stub to nil so the calls at least resolve --
	// methods that would have been installed simply don't appear, so
	// follow-on dispatches NoMethodError where MRI would have
	// succeeded. Acceptable for the rake-load milestone (load
	// completes; runtime invocations that rely on the class_eval'd
	// methods will surface as separate gaps).
	// class_eval(String, filename=nil, lineno=nil) parses the String
	// as Ruby and evaluates it with self == the receiving class and
	// CurrentClass == the class, so `def foo; end` inside the string
	// installs on cls.Methods. The block form (class_eval { ... })
	// runs the block with self rebound to the class, same routing.
	// String form is what rake's delegation pattern in file_list.rb
	// uses to synthesise Array-proxy methods at load time.
	classEvalFn := &object.BuiltinMethod{
		Name: "class_eval",
		Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
			cls, ok := recv.(*object.Class)
			if !ok {
				return nil, errorf("evaluator: class_eval: non-Class receiver %T", recv)
			}
			if len(args) >= 1 {
				if s, ok := args[0].(*object.String); ok {
					return evalClassEvalString(env, cls, s.Value())
				}
			}
			if bm, ok := block.(*goBlockMarker); ok && bm != nil && bm.fn != nil {
				return evalClassEvalBlock(env, cls, bm)
			}
			return object.NIL, nil
		},
	}
	c.AddMethod("class_eval", classEvalFn)
	c.AddMethod("module_eval", classEvalFn)

	instanceMethodsFn := func(env *object.Environment, cls *object.Class, args []object.RubyObject) (object.RubyObject, error) {
		inherited := true
		if len(args) >= 1 {
			switch args[0].(type) {
			case *object.Boolean:
				if args[0] == object.FALSE {
					inherited = false
				}
			case *object.Nil:
				inherited = false
			}
		}
		seen := map[string]bool{}
		out := []object.RubyObject{}
		collect := func(c *object.Class) {
			for name := range c.Methods {
				if seen[name] {
					continue
				}
				seen[name] = true
				out = append(out, env.Symbols().Intern(name))
			}
		}
		collect(cls)
		if inherited {
			for cur := cls.Super; cur != nil; cur = cur.Super {
				collect(cur)
			}
			for _, inc := range cls.Includes {
				collect(inc)
			}
		}
		return object.NewArray(out...), nil
	}
	add("instance_methods", instanceMethodsFn)
	// private_instance_methods / protected_instance_methods --
	// goruby tracks private via cls.Private set; protected tracking
	// isn't uniform so just return [].
	add("private_instance_methods", func(env *object.Environment, cls *object.Class, args []object.RubyObject) (object.RubyObject, error) {
		inherited := true
		if len(args) >= 1 {
			if b, ok := args[0].(*object.Boolean); ok {
				inherited = b.Value
			}
		}
		out := make([]object.RubyObject, 0)
		seen := map[string]bool{}
		cur := cls
		for cur != nil {
			for name := range cur.Private {
				if !seen[name] {
					seen[name] = true
					out = append(out, env.Symbols().Intern(name))
				}
			}
			if !inherited {
				break
			}
			cur = cur.Super
		}
		return object.NewArray(out...), nil
	})
	add("protected_instance_methods", func(env *object.Environment, cls *object.Class, args []object.RubyObject) (object.RubyObject, error) {
		return object.NewArray(), nil
	})
	// methods -- returns the class's class-method names (and
	// inherited from the metaclass chain by default).
	add("methods", func(env *object.Environment, cls *object.Class, args []object.RubyObject) (object.RubyObject, error) {
		out := make([]object.RubyObject, 0, len(cls.ClassMethods))
		seen := map[string]bool{}
		for name := range cls.ClassMethods {
			if !seen[name] {
				out = append(out, env.Symbols().Intern(name))
				seen[name] = true
			}
		}
		return object.NewArray(out...), nil
	})
	// instance_method(:name) -- returns the method bound to no
	// receiver. Goruby wraps in the existing Proc shape with the
	// MethodClass marker (full UnboundMethod semantics are skipped;
	// the bound form returned by Object#method already works).
	add("instance_method", func(env *object.Environment, cls *object.Class, args []object.RubyObject) (object.RubyObject, error) {
		if len(args) != 1 {
			return nil, errorf("evaluator: instance_method: expected 1 arg, got %d", len(args))
		}
		name, ok := symbolOrString(env, args[0])
		if !ok {
			return nil, errorf("evaluator: instance_method: expected Symbol or String")
		}
		m, found := cls.LookupMethod(name)
		if !found {
			return raiseBuiltin(env, "NameError", "undefined method `"+name+"' for class `"+cls.Name+"'")
		}
		// Use the same Proc-with-MethodClass shape Object#method uses;
		// no real receiver attached so Method#receiver returns nil and
		// #call would need a receiver via bind first (not modelled).
		p := &object.Proc{
			Params:   &boundMethodMarker{Name: name, Recv: nil},
			IsMethod: true,
		}
		if um, ok := m.(*object.UserMethod); ok {
			p.Body = um.Body
		}
		return p, nil
	})
	// public_instance_methods: we do not uniformly track per-method
	// visibility, so the public-filtered form aliases to the full
	// instance_methods list. minitest's runnable_methods uses this.
	add("public_instance_methods", instanceMethodsFn)
	// MRI also exposes public_method_defined?; alias to method_defined?
	// since we don't track visibility on user methods yet. Same goes
	// for protected_method_defined? -- treat it as "defined and not
	// private", which collapses to method_defined? here.
	add("public_method_defined?", mdef)
	add("protected_method_defined?", func(env *object.Environment, cls *object.Class, args []object.RubyObject) (object.RubyObject, error) {
		// No protected tracking yet -- always false rather than
		// claiming a private method is protected.
		return object.FALSE, nil
	})
	add("private_method_defined?", func(env *object.Environment, cls *object.Class, args []object.RubyObject) (object.RubyObject, error) {
		return object.FALSE, nil
	})

	// define_method(name) { ... } installs the block as an instance
	// method named `name` on the receiver class. Returns the method
	// name as a Symbol (matching MRI). The block runs with `self`
	// rebound to the method's receiver -- but enclosing-scope lookups
	// continue to resolve through the block's defining env via
	// invokeBlock.
	addBlockMethod(c, "define_method", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, blk *ast.BlockExpression) (object.RubyObject, error) {
		cls, ok := recv.(*object.Class)
		if !ok {
			return nil, errorf("evaluator: define_method called on non-Class receiver %T", recv)
		}
		if len(args) != 1 {
			return nil, errorf("evaluator: define_method: expected 1 arg, got %d", len(args))
		}
		mname, ok := symbolOrString(env, args[0])
		if !ok {
			return nil, errorf("evaluator: define_method: name must be Symbol or String")
		}
		// Capture the block AST + its defining env so the installed
		// method can rebind self to the dispatch receiver while still
		// resolving closure lookups through the block's lexical scope.
		// Without the rebind, @ivars inside the block read class-scope
		// state instead of the per-instance ivars of the dispatch
		// receiver -- minitest/mock's define_method-driven proxy
		// hits this on every call.
		blockNode := blk
		defEnv := env
		cb := invoke
		cls.AddMethod(mname, &object.BuiltinMethod{
			Name: mname,
			Fn: func(callEnv *object.Environment, callRecv object.RubyObject, callArgs []object.RubyObject, callBlock any) (object.RubyObject, error) {
				if blockNode == nil {
					// Fallback: no AST available (synthetic block);
					// fall back to the un-rebound callback.
					return cb(callArgs)
				}
				inner := object.NewEnclosedEnvironment(defEnv)
				inner.Self = callRecv
				if callInst, ok := callRecv.(*object.Instance); ok {
					inner.Self = callInst
				}
				// Mark the frame as a method frame so super and other
				// frame-walking helpers (findMethodName / findCallClass)
				// see this as the surrounding method.
				inner.MethodFrame = true
				inner.CurrentMethodName = mname
				inner.CurrentClass = cls
				// Bind the defined method's &b capture param (if the
				// block declared one as |..., &b|) to a Proc wrapping
				// the literal block the caller passed.
				if cap := blockNode.CapturedBlock; cap != nil && cap.Name != nil {
					var bound object.RubyObject = object.NIL
					switch blk := callBlock.(type) {
					case *ast.BlockExpression:
						if blk != nil {
							bound = procFromBlock(callEnv, blk)
						}
					case *goBlockMarker:
						if blk != nil {
							bound = procFromGoBlock(blk)
						}
					}
					inner.Set(cap.Name.Value, bound)
				}
				inner.CurrentBlock = callBlock
				return invokeBlock(inner, blockNode, callArgs)
			},
		})
		return env.Symbols().Intern(mname), nil
	})
}
