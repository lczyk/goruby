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
	c.Methods["public_send"] = c.Methods["send"]
}
