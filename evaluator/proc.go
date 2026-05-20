package evaluator

import (
	"math"

	"github.com/lczyk/goruby/ast"
	"github.com/lczyk/goruby/object"
)

// procFromLambda wraps a `-> { ... }` literal as a Proc value.
func procFromLambda(env *object.Environment, n *ast.FunctionLiteral) *object.Proc {
	return &object.Proc{
		Params:   n.Parameters,
		Body:     n.Body,
		DefEnv:   env,
		IsLambda: true,
	}
}

// procFromBlock wraps a BlockExpression as a Proc value (non-lambda).
// Used by `Proc.new` and `&blk` capture.
func procFromBlock(env *object.Environment, b *ast.BlockExpression) *object.Proc {
	return &object.Proc{
		Params:   b.Parameters,
		Body:     b.Body,
		DefEnv:   env,
		IsLambda: false,
	}
}

// invokeProc calls p with args, returning its body's value.
func invokeProc(env *object.Environment, p *object.Proc, args []object.RubyObject) (object.RubyObject, error) {
	if name, ok := p.Params.(symbolProcMarker); ok {
		if len(args) == 0 {
			return nil, errorf("evaluator: ArgumentError: &:%s needs a receiver", string(name))
		}
		return callMethod(env, args[0], string(name), args[1:])
	}
	if b, ok := p.Params.(*goBlockMarker); ok {
		return b.fn(args)
	}
	if bm, ok := p.Params.(*boundMethodMarker); ok {
		return callMethod(env, bm.Recv, bm.Name, args)
	}
	params, _ := p.Params.([]*ast.FunctionParameter)
	body, _ := p.Body.(*ast.BlockStatement)
	defEnv, _ := p.DefEnv.(*object.Environment)
	if defEnv == nil {
		defEnv = env
	}
	inner := object.NewEnclosedEnvironment(defEnv)
	if err := bindParams(inner, params, args); err != nil {
		return nil, err
	}
	return evalBlockStatement(inner, body)
}

// procFromGoBlock wraps a goBlockMarker as a Proc so user code can
// pass it on (via `&blk` capture) to other methods.
func procFromGoBlock(b *goBlockMarker) *object.Proc {
	return &object.Proc{Params: b, IsLambda: false}
}

// procFromBound wraps a (receiver, method-name) pair as a Proc, used
// by `Object#method(:name)`. invokeProc handles the marker.
func procFromBound(env *object.Environment, recv object.RubyObject, name string) *object.Proc {
	return &object.Proc{
		Params: &boundMethodMarker{Recv: recv, Name: name, Env: env},
	}
}

type boundMethodMarker struct {
	Recv object.RubyObject
	Name string
	Env  *object.Environment
}

// procFromSymbol synthesises the Proc behind `&:name`. When invoked it
// calls the named method on its first argument, forwarding any extras.
// Stored with a magic marker; invokeProc recognises it.
func procFromSymbol(name string) *object.Proc {
	return &object.Proc{
		Params:   symbolProcMarker(name),
		IsLambda: false,
	}
}

type symbolProcMarker string

// procArity returns the required-arg count for the Proc, mirroring
// ruby's Proc#arity (a negative result for splatted procs).
func procArity(p *object.Proc) object.RubyObject {
	params, _ := p.Params.([]*ast.FunctionParameter)
	required := 0
	hasSplat := false
	for _, prm := range params {
		if prm.IsSplat {
			hasSplat = true
			continue
		}
		if prm.Default == nil {
			required++
		}
	}
	if hasSplat {
		return object.NewInteger(int64(-(required + 1)))
	}
	return object.NewInteger(int64(required))
}

// goBlockMarker wraps a Go callback so it can be carried on a
// method's CurrentBlock slot the same way an *ast.BlockExpression is.
// `yield` dispatches through it when the surrounding method was called
// from Go with a synthesised block (used by Enumerable derivations).
type goBlockMarker struct {
	fn func([]object.RubyObject) (object.RubyObject, error)
}

// bootstrapBuiltins installs the standing built-in classes (Object,
// Exception hierarchy, Proc) on first eval. Idempotent.
func bootstrapBuiltins(env *object.Environment) {
	bootstrapObjectClass(env)
	bootstrapExceptionHierarchy(env)
	bootstrapProcClass(env)
	bootstrapDataClass(env)
	bootstrapCoreClasses(env)
	bootstrapEnumerableModule(env)
	bootstrapComparableModule(env)
	bootstrapMathModule(env)
	bootstrapIO(env)
	bootstrapRegexpClass(env)
}

// bootstrapComparableModule registers Comparable so `include Comparable`
// resolves. The evaluator already derives `<`, `<=`, etc. from `<=>`
// automatically (see comparableFromSpaceship); the module is a marker.
func bootstrapComparableModule(env *object.Environment) *object.Class {
	if existing, ok := env.Get("Comparable"); ok {
		if c, ok := existing.(*object.Class); ok {
			return c
		}
	}
	m := object.NewClass("Comparable", nil)
	m.IsModule = true
	env.SetGlobal("Comparable", m)
	return m
}

// bootstrapEnumerableModule installs the Enumerable module so
// `include Enumerable` resolves. Method dispatch derives the standard
// enumerable methods (map, select, reduce, etc.) from `each` on
// receivers whose class includes Enumerable.
func bootstrapEnumerableModule(env *object.Environment) *object.Class {
	if existing, ok := env.Get("Enumerable"); ok {
		if c, ok := existing.(*object.Class); ok {
			return c
		}
	}
	m := object.NewClass("Enumerable", nil)
	m.IsModule = true
	env.SetGlobal("Enumerable", m)
	return m
}

// bootstrapCoreClasses installs placeholder Class objects for the
// built-in primitive types so identifier lookups like `Array.new` and
// `Hash.new` resolve. Method dispatch on instances of these primitives
// still happens via the type-switched fast path in callMethod.
func bootstrapCoreClasses(env *object.Environment) {
	for _, name := range []string{"Array", "Hash", "String", "Integer", "Float", "Symbol", "Range", "Numeric", "NilClass", "TrueClass", "FalseClass"} {
		if _, ok := env.Get(name); !ok {
			env.SetGlobal(name, object.NewClass(name, nil))
		}
	}
	// Float::INFINITY, ::NAN
	if f, ok := env.Get("Float"); ok {
		if c, ok := f.(*object.Class); ok {
			if _, has := c.Constants["INFINITY"]; !has {
				c.Constants["INFINITY"] = object.NewFloat(math.Inf(1))
				c.Constants["NAN"] = object.NewFloat(math.NaN())
			}
		}
	}
}

// bootstrapProcClass installs a minimal Proc class so `Proc.new { ... }`
// resolves. The class's `new` is special-cased in callOnClass to take
// the surrounding block as the Proc body.
func bootstrapProcClass(env *object.Environment) *object.Class {
	if existing, ok := env.Get("Proc"); ok {
		if c, ok := existing.(*object.Class); ok {
			return c
		}
	}
	c := object.NewClass("Proc", nil)
	env.SetGlobal("Proc", c)
	return c
}
