package evaluator

import (
	"github.com/lczyk/goruby/ast"

	"github.com/lczyk/goruby/object"
)

// Universal block-taking methods: then / yield_self / tap, and the
// send-family (which forwards a block to the named method). Registered
// on ObjectClass so they override object_methods.go's non-block send
// registrations -- inheritance still surfaces them to BasicObjectClass
// callers since nothing else shadows them.

func init() {
	c := object.ObjectClass

	addBlockMethod(c, "then", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
		return invoke([]object.RubyObject{recv})
	})
	c.Methods["yield_self"] = c.Methods["then"]

	addBlockMethod(c, "tap", func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, _ *ast.BlockExpression) (object.RubyObject, error) {
		if _, err := invoke([]object.RubyObject{recv}); err != nil {
			return nil, err
		}
		return recv, nil
	})

	sendFn := func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, blk *ast.BlockExpression) (object.RubyObject, error) {
		if len(args) < 1 {
			return nil, errorf("evaluator: send needs a method name")
		}
		mname, ok := symbolOrString(env, args[0])
		if !ok {
			return nil, errorf("evaluator: send: method name must be Symbol or String")
		}
		marker := &goBlockMarker{fn: invoke, blk: blk}
		return dispatchWithBlock(env, recv, mname, args[1:], marker)
	}
	// Block-aware send-family. The non-block form is handled by
	// object_methods.go's existing send registrations.
	addBlockOrPlainMethod(c, "send",
		func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
			if len(args) < 1 {
				return nil, errorf("evaluator: send needs a method name")
			}
			mname, ok := symbolOrString(env, args[0])
			if !ok {
				return nil, errorf("evaluator: send: method name must be Symbol or String")
			}
			return callMethod(env, recv, mname, args[1:])
		},
		sendFn)
	c.Methods["__send__"] = c.Methods["send"]

	// public_send is dispatch-identical to send but enforces
	// visibility: private + protected methods raise NoMethodError
	// instead of silently dispatching.
	publicSendCheck := func(env *object.Environment, recv object.RubyObject, mname string) (object.RubyObject, error) {
		inst, ok := recv.(*object.Instance)
		if !ok {
			return nil, nil
		}
		if isPrivateMethod(inst.C, mname) {
			return raiseBuiltin(env, "NoMethodError", "private method `"+mname+"' called for instance of "+inst.C.Name)
		}
		if isProtectedMethod(inst.C, mname) && !callerIsKin(env, inst.C) {
			return raiseBuiltin(env, "NoMethodError", "protected method `"+mname+"' called for instance of "+inst.C.Name)
		}
		return nil, nil
	}
	publicSendBlock := func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, blk *ast.BlockExpression) (object.RubyObject, error) {
		if len(args) < 1 {
			return nil, errorf("evaluator: public_send needs a method name")
		}
		mname, ok := symbolOrString(env, args[0])
		if !ok {
			return nil, errorf("evaluator: public_send: method name must be Symbol or String")
		}
		if _, err := publicSendCheck(env, recv, mname); err != nil {
			return nil, err
		}
		marker := &goBlockMarker{fn: invoke, blk: blk}
		return dispatchWithBlock(env, recv, mname, args[1:], marker)
	}
	addBlockOrPlainMethod(c, "public_send",
		func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error) {
			if len(args) < 1 {
				return nil, errorf("evaluator: public_send needs a method name")
			}
			mname, ok := symbolOrString(env, args[0])
			if !ok {
				return nil, errorf("evaluator: public_send: method name must be Symbol or String")
			}
			if _, err := publicSendCheck(env, recv, mname); err != nil {
				return nil, err
			}
			return callMethod(env, recv, mname, args[1:])
		},
		publicSendBlock)
}
