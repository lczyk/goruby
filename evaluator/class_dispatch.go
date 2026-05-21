package evaluator

import (
	"github.com/lczyk/goruby/ast"
	"github.com/lczyk/goruby/object"
)

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
			return nil, errorf("evaluator: NoMethodError: undefined method `define' for %s", cls.Name)
		}
		return dataDefine(env, args)
	})

	add("superclass", func(env *object.Environment, cls *object.Class, args []object.RubyObject) (object.RubyObject, error) {
		if cls.Super != nil {
			return cls.Super, nil
		}
		return object.NIL, nil
	})

	add("name", func(env *object.Environment, cls *object.Class, args []object.RubyObject) (object.RubyObject, error) {
		return object.NewString(cls.Name), nil
	})
}
