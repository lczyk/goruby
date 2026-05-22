package evaluator

import (
	"github.com/lczyk/goruby/ast"
	"github.com/lczyk/goruby/object"
)

// blockCallback is the invocation shape callMethodWithBlock uses to
// stay agnostic of whether the "block" came from a literal `{ ... }`
// or from a Proc (e.g. `&:sym` / `&blk`).
type blockCallback func(args []object.RubyObject) (object.RubyObject, error)

// callMethodWithBlock dispatches receiver methods that take a block.
// Used for `obj.method { ... }`, `obj.method(&proc)`, and `obj.method(&:sym)`.
func callMethodWithBlock(env *object.Environment, recv object.RubyObject, name string, args []object.RubyObject, blk *ast.BlockExpression) (object.RubyObject, error) {
	cb := func(a []object.RubyObject) (object.RubyObject, error) {
		return invokeBlock(env, blk, a)
	}
	return dispatchWithBlock(env, recv, name, args, &goBlockMarker{fn: cb, blk: blk})
}

// callMethodWithProc routes a call whose block came from a `&proc`
// (or `&:sym`) capture rather than a literal block.
func callMethodWithProc(env *object.Environment, recv object.RubyObject, name string, args []object.RubyObject, p *object.Proc) (object.RubyObject, error) {
	cb := func(a []object.RubyObject) (object.RubyObject, error) {
		return invokeProc(env, p, a)
	}
	return dispatchWithBlock(env, recv, name, args, &goBlockMarker{fn: cb})
}

// dispatchWithBlock is the shared dispatch path: walk recv.Class()'s
// ancestry via object.Send, threading the block marker through. For
// Class receivers, user-defined class methods (`def self.foo`) win
// before the chain walk, matching callMethod's behaviour.
func dispatchWithBlock(env *object.Environment, recv object.RubyObject, name string, args []object.RubyObject, marker *goBlockMarker) (object.RubyObject, error) {
	if cls, ok := recv.(*object.Class); ok {
		if m, found := cls.LookupClassMethod(name); found {
			if um, ok := m.(*object.UserMethod); ok {
				return invokeMethodOnWithBlock(env, cls, um, args, marker.blk)
			}
		}
	}
	if v, ok, err := object.Send(env, recv, name, args, marker); ok {
		return v, err
	}
	if inst, ok := recv.(*object.Instance); ok {
		return nil, errorf("evaluator: NoMethodError: undefined method `%s' for instance of %s", name, inst.C.Name)
	}
	return nil, errorf("evaluator: NoMethodError: undefined method `%s' for %T", name, recv)
}

// iterStep runs one iteration step via the block callback. Translates
// nextSignal into a normal value-returning step and breakSignal into a
// stop signal. Bubbles other errors verbatim.
func iterStep(invoke blockCallback, args []object.RubyObject) (val object.RubyObject, stop bool, err error) {
	val, err = invoke(args)
	if err == nil {
		return val, false, nil
	}
	if ns, ok := err.(*nextSignal); ok {
		v := ns.Value
		if v == nil {
			v = object.NIL
		}
		return v, false, nil
	}
	if bs, ok := err.(*breakSignal); ok {
		v := bs.Value
		if v == nil {
			v = object.NIL
		}
		return v, true, nil
	}
	return nil, false, err
}

// yieldOne is iterStep specialised for the single-arg yield -- a
// thin shim over the variadic form that keeps the call sites
// uncluttered. Alloc-neutral with the inline `[]RubyObject{v}` form
// (the slice is heap-allocated per call either way; escape analysis
// can't prove invoke won't retain it across the call boundary), but
// the helper makes future per-loop buffer reuse trivial.
func yieldOne(invoke blockCallback, v object.RubyObject) (object.RubyObject, bool, error) {
	return iterStep(invoke, []object.RubyObject{v})
}

// addBlockMethod registers a block-aware builtin on cls. The Fn unpacks
// the *goBlockMarker the dispatcher passes when a block is present;
// callers that don't expect a no-block path can return an error.
func addBlockMethod(cls *object.Class, name string, fn func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, blk *ast.BlockExpression) (object.RubyObject, error)) {
	cls.AddMethod(name, &object.BuiltinMethod{
		Name: name,
		Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
			if block == nil {
				return nil, errorf("evaluator: %s requires a block", name)
			}
			bm, ok := block.(*goBlockMarker)
			if !ok {
				return nil, errorf("evaluator: %s: unexpected block payload %T", name, block)
			}
			return fn(env, recv, args, bm.fn, bm.blk)
		},
	})
}

// addBlockOrPlainMethod registers a method whose behaviour depends on
// whether the caller passed a block. The plain form runs when block is
// nil; the block form runs otherwise. Used for names like `map`,
// `select`, `sort` etc. that are valid with or without a block.
func addBlockOrPlainMethod(cls *object.Class, name string,
	plain func(env *object.Environment, recv object.RubyObject, args []object.RubyObject) (object.RubyObject, error),
	block func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, invoke blockCallback, blk *ast.BlockExpression) (object.RubyObject, error),
) {
	cls.AddMethod(name, &object.BuiltinMethod{
		Name: name,
		Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, blkAny any) (object.RubyObject, error) {
			if blkAny == nil {
				if plain == nil {
					return nil, errorf("evaluator: %s requires a block", name)
				}
				return plain(env, recv, args)
			}
			bm, ok := blkAny.(*goBlockMarker)
			if !ok {
				return nil, errorf("evaluator: %s: unexpected block payload %T", name, blkAny)
			}
			return block(env, recv, args, bm.fn, bm.blk)
		},
	})
}
