package evaluator

import (
	"github.com/lczyk/goruby/ast"
	"github.com/lczyk/goruby/evaluator/builtinapi"
	"github.com/lczyk/goruby/object"
)

// init wires the builtinapi.InvokeCurrentBlock function-pointer so
// stdlib stubs (e.g. OptionParser.new) can yield to the block without
// pulling the evaluator's *ast.BlockExpression / goBlockMarker into
// builtinapi.
func init() {
	builtinapi.InvokeCurrentBlock = func(env *object.Environment, args []object.RubyObject) (object.RubyObject, bool, error) {
		v, err := invokeBlockValue(env, env.CurrentBlock, args)
		if env.CurrentBlock == nil {
			return nil, false, err
		}
		return v, true, err
	}
	builtinapi.InvokeBlockValue = invokeBlockValue
}

// invokeBlockValue invokes a stored block value (whatever shape it
// took when captured: *ast.BlockExpression, *goBlockMarker, *Proc).
// Returns (nil, nil) when blk is nil so callers can guard on the
// payload.
func invokeBlockValue(env *object.Environment, blk any, args []object.RubyObject) (object.RubyObject, error) {
	if blk == nil {
		return nil, nil
	}
	if be, ok := blk.(*ast.BlockExpression); ok && be != nil {
		return invokeBlock(env, be, args)
	}
	if bm, ok := blk.(*goBlockMarker); ok && bm != nil {
		return bm.fn(args)
	}
	if p, ok := blk.(*object.Proc); ok && p != nil {
		return invokeProc(env, p, args)
	}
	return nil, nil
}

// invokeBlock evaluates blk with the given positional args bound to its
// parameters. Uses an enclosed env so block-local writes don't leak,
// while still seeing the caller's locals (matching ruby block scoping).
func invokeBlock(env *object.Environment, blk *ast.BlockExpression, args []object.RubyObject) (object.RubyObject, error) {
	inner := object.NewEnclosedEnvironment(env)
	// Auto-splat: ruby blocks destructure a single Array arg when the
	// block declares multiple positional params -- the idiom that lets
	// `{|k, v| ...}` work for `each_with_index` / `Hash#each` / etc.
	if len(args) == 1 && len(blk.Parameters) > 1 {
		if arr, ok := args[0].(*object.Array); ok {
			args = arr.Elements
		}
	}
	if len(blk.Parameters) == 0 {
		// Bind numbered block params (`_1`, `_2`, ...) -- ruby 2.7+ --
		// and the anonymous `it` (ruby 3.4+) when the block declares
		// no explicit params. Cheap and side-effect-free if the block
		// doesn't use them.
		if len(args) > 0 {
			inner.Set("it", args[0])
		}
		for i, a := range args {
			inner.Set(numberedParamName(i+1), a)
		}
	} else {
		if err := bindParams(inner, blk.Parameters, args); err != nil {
			return nil, err
		}
	}
	return evalBlockStatement(inner, blk.Body)
}

func numberedParamName(i int) string {
	switch i {
	case 1:
		return "_1"
	case 2:
		return "_2"
	case 3:
		return "_3"
	case 4:
		return "_4"
	case 5:
		return "_5"
	case 6:
		return "_6"
	case 7:
		return "_7"
	case 8:
		return "_8"
	case 9:
		return "_9"
	}
	return ""
}

func evalYield(env *object.Environment, n *ast.YieldExpression) (object.RubyObject, error) {
	blkAny := env.EnclosingBlock()
	if blkAny == nil {
		return nil, errorf("evaluator: LocalJumpError: no block given (yield)")
	}
	args, err := evalExpressions(env, n.Arguments)
	if err != nil {
		return nil, err
	}
	switch blk := blkAny.(type) {
	case *ast.BlockExpression:
		return invokeBlock(env, blk, args)
	case *goBlockMarker:
		return blk.fn(args)
	}
	return nil, errorf("evaluator: unexpected block payload %T", blkAny)
}
