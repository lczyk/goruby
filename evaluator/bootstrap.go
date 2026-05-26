package evaluator

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/lczyk/goruby/ast"
	"github.com/lczyk/goruby/evaluator/stdlib"
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
		// The marker's fn knows how to invoke whatever it wraps (a
		// literal block via invokeBlock, or a deeper Proc via
		// invokeProc). To stay arity-tolerant for non-lambda block
		// shapes, adjust args against the wrapped block's real param
		// list when available (marker.blk). Without this, a 1-param
		// block wrapped through a class-method forward (`def f(&b);
		// g(&b); end`) loses non-lambda lenience and rejects
		// `proc.call(self, args)` style calls on a 1-param block.
		if b.blk != nil && !p.IsLambda {
			args = adjustProcArgs(b.blk.Parameters, args)
		}
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
	// Procs (non-lambda) tolerate arity mismatch: extras are dropped,
	// missing params bind to nil. Lambdas + methods are strict.
	if !p.IsLambda {
		args = adjustProcArgs(params, args)
	}
	if err := bindParams(inner, params, args); err != nil {
		return nil, err
	}
	// Pre-declare locals introduced anywhere in the proc body so a
	// read inside a branch that didn't run still sees nil -- matches
	// MRI's parser-time lvar introduction. Same semantics as the
	// method-body predeclare.
	if body != nil {
		for _, s := range body.Statements {
			predeclareNode(inner, s)
		}
	}
	return evalBlockStatement(inner, body)
}

// adjustProcArgs trims / pads args to match a non-lambda proc's
// parameter list. Mirrors ruby block-arity tolerance:
//   - extra args (beyond the last positional, when no splat) -> dropped
//   - missing positionals -> nil
//
// Splats and keyword params are pass-through; the caller's bindParams
// still does the real binding work, just on a length-aligned slice.
func adjustProcArgs(params []*ast.FunctionParameter, args []object.RubyObject) []object.RubyObject {
	// Find the splat position (if any). With a splat, extras are
	// already absorbed; nothing to do.
	for _, p := range params {
		if p.IsSplat || p.IsKeyword || p.IsKeywordRest {
			return args
		}
	}
	if len(args) > len(params) {
		args = args[:len(params)]
	}
	for len(args) < len(params) {
		args = append(args, object.NIL)
	}
	return args
}

// procFromGoBlock wraps a goBlockMarker as a Proc so user code can
// pass it on (via `&blk` capture) to other methods.
func procFromGoBlock(b *goBlockMarker) *object.Proc {
	return &object.Proc{Params: b, IsLambda: false}
}

// procFromBound wraps a (receiver, method-name) pair as a Proc-shaped
// value typed as Method, used by `Object#method(:name)`. invokeProc
// handles the marker; .class reports Method (not Proc) per MRI.
func procFromBound(env *object.Environment, recv object.RubyObject, name string) *object.Proc {
	return &object.Proc{
		Params:   &boundMethodMarker{Recv: recv, Name: name, Env: env},
		IsMethod: true,
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
// blk holds the AST-level BlockExpression when the marker originates
// from a literal `{ ... }` -- some builtins (Proc.new, Hash.new,
// user-class .new) need to forward the block AST to a user method.
type goBlockMarker struct {
	fn  func([]object.RubyObject) (object.RubyObject, error)
	blk *ast.BlockExpression
	// proc holds the originating Proc value when the marker was built
	// from a `&proc` capture. Lets callers that need self-rebinding
	// (instance_eval / instance_exec) recover the Proc body and run
	// it under a fresh self.
	proc *object.Proc
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
	bootstrapKernelModule(env)
	bootstrapComparableModule(env)
	stdlib.BootstrapMathModule(env)
	bootstrapLoadPath(env)
	bootstrapENV(env)
	bootstrapRbConfig(env)
	bootstrapFileUtils(env)
	bootstrapSingleton(env)
	bootstrapMonitor(env)
	bootstrapSetClass(env)
	bootstrapStringIO(env)
	bootstrapQueueClass(env)
	bootstrapMutexClass(env)
	bootstrapThreadClass(env)
	bootstrapTimeClass(env)
	bootstrapEtcModule(env)
	bootstrapProcessModule(env)
	bootstrapMarshalModule(env)
	bootstrapWarningModule(env)
	bootstrapGemModule(env)
	// ARGV default: empty Array unless CLI seeded via WithARGV.
	if _, ok := env.Get("ARGV"); !ok {
		env.SetGlobal("ARGV", object.NewArray())
	}
	// Top-level identity constants. Rake's clean / cpu_counter probe
	// RUBY_ENGINE / RUBY_VERSION before deciding which fallback to take.
	if _, ok := env.Get("RUBY_VERSION"); !ok {
		env.SetGlobal("RUBY_VERSION", object.NewString("3.4.0"))
		env.SetGlobal("RUBY_ENGINE", object.NewString("goruby"))
		env.SetGlobal("RUBY_ENGINE_VERSION", object.NewString("0.1.0"))
		env.SetGlobal("RUBY_PLATFORM", object.NewString(runtime.GOOS+"-"+runtime.GOARCH))
		env.SetGlobal("RUBY_RELEASE_DATE", object.NewString("2026-01-01"))
		env.SetGlobal("RUBY_DESCRIPTION", object.NewString("goruby 0.1.0"))
	}
	// Expose the Proc / Method classes at top level so rake's
	// rule.rb (which name-checks `Method === arg`) resolves.
	if _, ok := env.Get("Method"); !ok {
		env.SetGlobal("Method", object.MethodClass)
	}
	if _, ok := env.Get("Proc"); !ok {
		env.SetGlobal("Proc", object.ProcClass)
	}
	bootstrapPathnameClass(env)
	bootstrapDir(env)
	bootstrapIO(env)
	bootstrapRegexpClass(env)
	stdlib.BootstrapEncodingClass(env)
	stdlib.BootstrapComplexClass(env)
	stdlib.BootstrapRationalClass(env)
	stdlib.BootstrapDateClass(env)
	stdlib.BootstrapPrimeModule(env)
	bootstrapEnumeratorClass(env)
	bootstrapLazyClass(env)
	stdlib.BootstrapOptionParser(env)
	stdlib.BootstrapStringScanner(env)
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

// kernelSingleton is the package-level Kernel module shared across
// envs. Per-env bootstrap binds it under the "Kernel" name and
// ensures Object.Includes points at it. Avoids appending multiple
// duplicate Kernels across in-process test fixture runs.
var kernelSingleton *object.Class

// bootstrapKernelModule installs Kernel as a module that ObjectClass
// includes. Gems that reopen Kernel to add top-level helpers
// (minitest's `module Kernel; def describe ... end; end`) then
// reach main via the Object include chain. Idempotent across envs.
func bootstrapKernelModule(env *object.Environment) {
	if existing, ok := env.Get("Kernel"); ok {
		if _, isClass := existing.(*object.Class); isClass {
			return
		}
	}
	if kernelSingleton != nil {
		// Reuse the existing singleton so user methods added in earlier
		// envs don't disappear (but they shouldn't accumulate either:
		// each env's load order replays the same gem code). Per-test
		// state pollution is a known caveat documented in the plan.
		env.SetGlobal("Kernel", kernelSingleton)
		// Ensure Object.Includes contains it exactly once.
		found := false
		for _, inc := range object.ObjectClass.Includes {
			if inc == kernelSingleton {
				found = true
				break
			}
		}
		if !found {
			object.ObjectClass.Includes = append(object.ObjectClass.Includes, kernelSingleton)
		}
		return
	}
	m := object.NewClass("Kernel", nil)
	m.IsModule = true
	// Modules have no Super -- NewClass defaults to Object, but that
	// creates a cycle once Object includes Kernel.
	m.Super = nil
	kernelSingleton = m
	// Register the kernel builtins (raise/puts/print/p/...) on the
	// module so they're reachable through normal dispatch, including
	// from inside a class body that defines method_missing. Without
	// this, `raise` inside a method_missing body re-enters
	// method_missing (no method called "raise" on the receiver) and
	// infinite-loops.
	registerKernelBuiltin := func(name string) {
		m.AddMethod(name, &object.BuiltinMethod{
			Name: name,
			Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
				return callKernel(env, name, args)
			},
		})
	}
	for _, name := range []string{
		"puts", "print", "printf", "p", "pp", "sprintf", "format",
		"raise", "fail",
		"Integer", "Float", "String", "Array",
		"gets", "putc", "rand", "srand",
		"require", "require_relative", "load",
		"loop", "block_given?", "lambda", "proc",
		"caller", "exit", "abort",
		"system", "exec", "spawn",
		"sleep", "at_exit", "binding",
		"warn", "open", "readline",
		"trap",
		"__method__", "__callee__",
		"Rational", "Complex",
		"eval",
	} {
		registerKernelBuiltin(name)
	}
	// catch / throw -- block-aware, need direct dispatch (not via
	// callKernel which doesn't see the block).
	// Kernel#open -- routes to File.open. When given a block, yields
	// the file Instance and ensures close on return. Rake's
	// test_rake_makefile_loader writes a sample makefile via this.
	m.AddMethod("open", &object.BuiltinMethod{Name: "open", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		if len(args) < 1 {
			return nil, errorf("evaluator: open expects 1..3 args")
		}
		path, ok := stringText(env, args[0])
		if !ok {
			return nil, errorf("evaluator: open: expected String path, got %T", args[0])
		}
		mode := "r"
		if len(args) >= 2 {
			if m, ok := stringText(env, args[1]); ok {
				mode = m
			}
		}
		// Write modes -- open via os.Create / os.OpenFile, yield a
		// write-only Instance with #<< / #write / #puts / #print, then
		// close. Buffer in-memory and flush on block exit so heredoc
		// `io << <<-MF ... MF` shapes write atomically.
		if mode == "w" || mode == "w+" || mode == "wb" || mode == "a" {
			io := newOpenWriteIO(path, mode == "a")
			var res object.RubyObject = object.NIL
			var blkErr error
			if block != nil {
				res, blkErr = invokeBlockValue(env, block, []object.RubyObject{io})
			}
			if err := flushOpenWriteIO(io); err != nil {
				return raiseBuiltin(env, "Errno::EACCES", err.Error())
			}
			if blkErr != nil {
				return nil, blkErr
			}
			if block == nil {
				return io, nil
			}
			return res, nil
		}
		// Read mode -- delegate to File.new which slurps + exposes each / each_line.
		fileCls, ok := env.Get("File")
		if !ok {
			return nil, errorf("evaluator: open: File class missing")
		}
		c, ok := fileCls.(*object.Class)
		if !ok {
			return nil, errorf("evaluator: open: File class missing")
		}
		newFn, ok := c.ClassMethods["new"]
		if !ok {
			return nil, errorf("evaluator: open: File.new missing")
		}
		newUser, ok := newFn.(*object.UserMethod)
		if !ok {
			return nil, errorf("evaluator: open: File.new unexpected shape")
		}
		nat, ok := newUser.Body.(nativeFn)
		if !ok {
			return nil, errorf("evaluator: open: File.new unexpected shape")
		}
		inst, err := nat.Fn(env, []object.RubyObject{object.NewString(path)})
		if err != nil {
			return nil, err
		}
		if block != nil {
			res, blkErr := invokeBlockValue(env, block, []object.RubyObject{inst})
			return res, blkErr
		}
		return inst, nil
	}})

	m.AddMethod("throw", &object.BuiltinMethod{Name: "throw", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		if len(args) < 1 || len(args) > 2 {
			return nil, errorf("evaluator: throw expects 1..2 args, got %d", len(args))
		}
		tag := args[0]
		var val object.RubyObject = object.NIL
		if len(args) == 2 {
			val = args[1]
		}
		return nil, &throwSignal{Tag: tag, Value: val}
	}})
	m.AddMethod("catch", &object.BuiltinMethod{Name: "catch", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		var tag object.RubyObject
		if len(args) >= 1 {
			tag = args[0]
		} else {
			// MRI: bare catch creates a fresh anonymous tag (an Object).
			// Stub: use a fresh empty String -- tag-equality is by
			// pointer-identity since no two String literals share an
			// address.
			tag = object.NewString("__catch__")
		}
		var result object.RubyObject
		var err error
		if bm, ok := block.(*goBlockMarker); ok && bm != nil && bm.fn != nil {
			result, err = bm.fn([]object.RubyObject{tag})
		} else {
			return nil, errorf("evaluator: catch needs a block")
		}
		if err != nil {
			if ts, isThrow := err.(*throwSignal); isThrow {
				if rubyEqual(ts.Tag, tag) {
					return ts.Value, nil
				}
				// Tag mismatch: re-raise so outer catch can match.
				return nil, ts
			}
			return nil, err
		}
		return result, nil
	}})
	env.SetGlobal("Kernel", m)
	// Wire Kernel into ObjectClass's include chain so methods added
	// to Kernel later (via `module Kernel; def foo ...`) become
	// visible on every instance.
	already := false
	for _, inc := range object.ObjectClass.Includes {
		if inc == m {
			already = true
			break
		}
	}
	if !already {
		object.ObjectClass.Includes = append(object.ObjectClass.Includes, m)
	}
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

// bootstrapSetClass installs a placeholder Set class backed by an
// @items Array. Just enough surface for rake's thread_pool.rb usage:
// new, add, include?, each, empty?, size. Insertion-order
// preservation via Array (not MRI's hash-backed ordering); equality
// uses rubyEqualDispatch so Symbols / Strings dedupe naturally.
// Idempotent.
func bootstrapSetClass(env *object.Environment) {
	if _, ok := env.Get("Set"); ok {
		return
	}
	c := object.NewClass("Set", nil)
	c.ClassMethods["new"] = &object.BuiltinMethod{Name: "new", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		inst := object.NewInstance(c)
		items := object.NewArray()
		inst.Ivars["@items"] = items
		// Set.new(enumerable) populates from each element, deduping.
		// Accept an Array directly (the common case); other Enumerables
		// can be added when a fixture pins them.
		if len(args) >= 1 {
			if arr, ok := args[0].(*object.Array); ok {
				for _, e := range arr.Elements {
					already := false
					for _, x := range items.Elements {
						if rubyEqualDispatch(env, e, x) {
							already = true
							break
						}
					}
					if !already {
						items.Elements = append(items.Elements, e)
					}
				}
			}
		}
		return inst, nil
	}}
	itemsOf := func(recv object.RubyObject) *object.Array {
		inst, _ := recv.(*object.Instance)
		if inst == nil {
			return nil
		}
		if a, ok := inst.Ivars["@items"].(*object.Array); ok {
			return a
		}
		a := object.NewArray()
		inst.Ivars["@items"] = a
		return a
	}
	c.Methods["add"] = &object.BuiltinMethod{Name: "add", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		if len(args) != 1 {
			return nil, errorf("evaluator: Set#add: expected 1 arg, got %d", len(args))
		}
		a := itemsOf(recv)
		for _, e := range a.Elements {
			if rubyEqualDispatch(env, e, args[0]) {
				return recv, nil
			}
		}
		a.Elements = append(a.Elements, args[0])
		return recv, nil
	}}
	c.Methods["<<"] = c.Methods["add"]
	c.Methods["delete"] = &object.BuiltinMethod{Name: "delete", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		if len(args) != 1 {
			return nil, errorf("evaluator: Set#delete: expected 1 arg, got %d", len(args))
		}
		a := itemsOf(recv)
		for i, e := range a.Elements {
			if rubyEqualDispatch(env, e, args[0]) {
				a.Elements = append(a.Elements[:i], a.Elements[i+1:]...)
				return recv, nil
			}
		}
		return recv, nil
	}}
	c.Methods["include?"] = &object.BuiltinMethod{Name: "include?", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		if len(args) != 1 {
			return nil, errorf("evaluator: Set#include?: expected 1 arg, got %d", len(args))
		}
		a := itemsOf(recv)
		for _, e := range a.Elements {
			if rubyEqualDispatch(env, e, args[0]) {
				return object.TRUE, nil
			}
		}
		return object.FALSE, nil
	}}
	c.Methods["size"] = &object.BuiltinMethod{Name: "size", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		return object.NewInteger(int64(len(itemsOf(recv).Elements))), nil
	}}
	c.Methods["length"] = c.Methods["size"]
	c.Methods["count"] = c.Methods["size"]
	c.Methods["empty?"] = &object.BuiltinMethod{Name: "empty?", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		return object.BooleanOf(len(itemsOf(recv).Elements) == 0), nil
	}}
	c.Methods["to_a"] = &object.BuiltinMethod{Name: "to_a", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		a := itemsOf(recv)
		out := make([]object.RubyObject, len(a.Elements))
		copy(out, a.Elements)
		return object.NewArray(out...), nil
	}}
	env.SetGlobal("Set", c)
}

// bootstrapStringIO installs a minimal StringIO class -- the
// in-memory IO-like buffer that minitest's capture_io uses and
// many gem tests reach for. Surface: new, puts, print, write,
// <<, string, read, rewind, close, closed?.
// Backed by an @buf String + @pos Integer.
func bootstrapStringIO(env *object.Environment) {
	if _, ok := env.Get("StringIO"); ok {
		return
	}
	c := object.NewClass("StringIO", nil)
	c.ClassMethods["new"] = &object.BuiltinMethod{Name: "new", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		inst := object.NewInstance(c)
		s := ""
		if len(args) >= 1 {
			if t, ok := stringText(env, args[0]); ok {
				s = t
			}
		}
		inst.Ivars["@buf"] = object.NewString(s)
		inst.Ivars["@pos"] = object.NewInteger(0)
		return inst, nil
	}}
	getBuf := func(recv object.RubyObject) *object.String {
		inst, _ := recv.(*object.Instance)
		if inst == nil {
			return nil
		}
		if s, ok := inst.Ivars["@buf"].(*object.String); ok {
			return s
		}
		s := object.NewString("")
		inst.Ivars["@buf"] = s
		return s
	}
	getPos := func(recv object.RubyObject) int64 {
		inst, _ := recv.(*object.Instance)
		if inst == nil {
			return 0
		}
		if p, ok := inst.Ivars["@pos"].(*object.Integer); ok {
			return p.Value
		}
		return 0
	}
	setPos := func(recv object.RubyObject, p int64) {
		inst, _ := recv.(*object.Instance)
		if inst != nil {
			inst.Ivars["@pos"] = object.NewInteger(p)
		}
	}
	appendStr := func(recv object.RubyObject, s string) {
		buf := getBuf(recv)
		buf.Buf = append(buf.Buf, []byte(s)...)
	}
	c.Methods["string"] = &object.BuiltinMethod{Name: "string", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		return getBuf(recv), nil
	}}
	c.Methods["puts"] = &object.BuiltinMethod{Name: "puts", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		if len(args) == 0 {
			appendStr(recv, "\n")
			return object.NIL, nil
		}
		for _, a := range args {
			s, _ := stringText(env, a)
			if s == "" {
				s = env.Inspect(a)
			}
			appendStr(recv, s)
			if !strings.HasSuffix(s, "\n") {
				appendStr(recv, "\n")
			}
		}
		return object.NIL, nil
	}}
	c.Methods["print"] = &object.BuiltinMethod{Name: "print", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		for _, a := range args {
			s, _ := stringText(env, a)
			appendStr(recv, s)
		}
		return object.NIL, nil
	}}
	c.Methods["write"] = c.Methods["print"]
	c.Methods["<<"] = &object.BuiltinMethod{Name: "<<", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		if len(args) == 1 {
			s, _ := stringText(env, args[0])
			appendStr(recv, s)
		}
		return recv, nil
	}}
	c.Methods["read"] = &object.BuiltinMethod{Name: "read", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		buf := getBuf(recv)
		pos := getPos(recv)
		all := string(buf.Buf)
		if pos >= int64(len(all)) {
			return object.NewString(""), nil
		}
		rest := all[pos:]
		setPos(recv, int64(len(all)))
		return object.NewString(rest), nil
	}}
	c.Methods["rewind"] = &object.BuiltinMethod{Name: "rewind", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		setPos(recv, 0)
		return object.NewInteger(0), nil
	}}
	c.Methods["close"] = &object.BuiltinMethod{Name: "close", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		return object.NIL, nil
	}}
	c.Methods["closed?"] = &object.BuiltinMethod{Name: "closed?", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		return object.FALSE, nil
	}}
	c.Methods["pos"] = &object.BuiltinMethod{Name: "pos", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		return object.NewInteger(getPos(recv)), nil
	}}
	c.Methods["sync"] = &object.BuiltinMethod{Name: "sync", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		return object.TRUE, nil
	}}
	c.Methods["sync="] = &object.BuiltinMethod{Name: "sync=", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		if len(args) == 1 {
			return args[0], nil
		}
		return object.NIL, nil
	}}
	env.SetGlobal("StringIO", c)

	// Tempfile: real fs-backed temp files via os.CreateTemp. Each
	// instance carries the underlying *os.File path. Reuse the
	// StringIO surface for the in-memory write/read shape; flush
	// syncs to disk so the shelled-out diff in minitest sees real
	// bytes at the path.
	t := object.NewClass("Tempfile", nil)
	tempOpen := &object.BuiltinMethod{Name: "open", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, blockAny any) (object.RubyObject, error) {
		prefix := "goruby-temp"
		if len(args) >= 1 {
			if s, ok := stringText(env, args[0]); ok {
				prefix = s
			}
		}
		f, err := os.CreateTemp("", prefix+"-*")
		if err != nil {
			return raiseBuiltin(env, "IOError", err.Error())
		}
		inst := object.NewInstance(c)
		inst.Ivars["@buf"] = object.NewString("")
		inst.Ivars["@pos"] = object.NewInteger(0)
		inst.Ivars["@path"] = object.NewString(f.Name())
		inst.Ivars["@__file__"] = object.NewString(f.Name())
		_ = f.Close()
		// Run the block with the temp instance, then unlink the file
		// on return (matching Tempfile.open's contract).
		if b, ok := blockAny.(*goBlockMarker); ok && b != nil && b.fn != nil {
			defer os.Remove(f.Name())
			if _, err := b.fn([]object.RubyObject{inst}); err != nil {
				return nil, err
			}
		}
		return inst, nil
	}}
	t.ClassMethods["open"] = tempOpen
	t.ClassMethods["new"] = tempOpen
	env.SetGlobal("Tempfile", t)
	c.Methods["path"] = &object.BuiltinMethod{Name: "path", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		inst, _ := recv.(*object.Instance)
		if inst != nil {
			if p, ok := inst.Ivars["@path"].(*object.String); ok {
				return p, nil
			}
		}
		return object.NewString(""), nil
	}}
	c.Methods["flush"] = &object.BuiltinMethod{Name: "flush", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		// When the instance is a real Tempfile (has @__file__ ivar),
		// write the in-memory buffer to disk so external readers see
		// the content.
		inst, ok := recv.(*object.Instance)
		if !ok {
			return recv, nil
		}
		path, hasPath := inst.Ivars["@__file__"].(*object.String)
		if !hasPath {
			return recv, nil
		}
		buf, ok := inst.Ivars["@buf"].(*object.String)
		if !ok {
			return recv, nil
		}
		if err := os.WriteFile(string(path.Buf), buf.Buf, 0644); err != nil {
			return raiseBuiltin(env, "IOError", err.Error())
		}
		return recv, nil
	}}
}

// bootstrapEtcModule installs an empty Etc placeholder. minitest and
// rake both unconditionally `require "etc"` at load time; both then
// do `if Etc.respond_to?(:nprocessors)` to decide whether to use it.
// An empty module returns false for respond_to? on any method name,
// so callers fall through to their fallback paths cleanly.
func bootstrapEtcModule(env *object.Environment) {
	if _, ok := env.Get("Etc"); ok {
		return
	}
	c := object.NewClass("Etc", nil)
	c.IsModule = true
	// nprocessors returns 1 -- the conservative default for callers
	// that read it for parallelism sizing (minitest, rake). Real
	// host CPU count from runtime.NumCPU() would be just as honest
	// but the parallelism is serial-fallback in our threading model
	// anyway, so reporting 1 matches actual behaviour.
	c.ClassMethods["nprocessors"] = &object.BuiltinMethod{
		Name: "nprocessors",
		Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
			return object.NewInteger(1), nil
		},
	}
	env.SetGlobal("Etc", c)
}

// bootstrapProcessModule installs a minimal Process module. Real
// host pid / euid lookups are honest -- they don't change semantics
// of pure-ruby callers and match what MRI's Process module returns.
func bootstrapProcessModule(env *object.Environment) {
	if _, ok := env.Get("Process"); ok {
		return
	}
	c := object.NewClass("Process", nil)
	c.IsModule = true
	c.ClassMethods["pid"] = &object.BuiltinMethod{
		Name: "pid",
		Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
			return object.NewInteger(int64(os.Getpid())), nil
		},
	}
	c.ClassMethods["clock_gettime"] = &object.BuiltinMethod{
		Name: "clock_gettime",
		Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
			// MRI signature: clock_gettime(clock_id, unit=:float_second).
			// Go has only one monotonic clock; treat CLOCK_MONOTONIC
			// kinds via time.Since on a fixed monoStart, and CLOCK_REALTIME
			// kinds via time.Now (UnixNano). Unit selects integer/float
			// nanosecond, microsecond, etc.
			now := time.Now()
			elapsedNs := now.UnixNano()
			realtime := true
			if len(args) >= 1 {
				if sym, ok := args[0].(*object.Symbol); ok {
					name := env.Symbols().Name(sym.ID)
					if strings.Contains(strings.ToLower(name), "monotonic") {
						realtime = false
					}
				}
				if inst, ok := args[0].(*object.Integer); ok {
					// CLOCK_MONOTONIC=1 / CLOCK_MONOTONIC_RAW=4 on linux.
					if inst.Value == 1 || inst.Value == 4 || inst.Value == 6 {
						realtime = false
					}
				}
			}
			if !realtime {
				// Monotonic source: use elapsed since monoStart for stability.
				elapsedNs = int64(time.Since(monoStart))
			}
			unit := "float_second"
			if len(args) >= 2 {
				if sym, ok := args[1].(*object.Symbol); ok {
					unit = env.Symbols().Name(sym.ID)
				}
			}
			switch unit {
			case "nanosecond":
				return object.NewInteger(elapsedNs), nil
			case "microsecond":
				return object.NewInteger(elapsedNs / 1000), nil
			case "millisecond":
				return object.NewInteger(elapsedNs / 1000000), nil
			case "second":
				return object.NewInteger(elapsedNs / 1000000000), nil
			case "float_microsecond":
				return object.NewFloat(float64(elapsedNs) / 1000.0), nil
			case "float_millisecond":
				return object.NewFloat(float64(elapsedNs) / 1000000.0), nil
			default: // float_second
				return object.NewFloat(float64(elapsedNs) / 1e9), nil
			}
		},
	}
	env.SetGlobal("Process", c)
}

var monoStart = time.Now()

// bootstrapMarshalModule installs a Marshal placeholder. minitest's
// sanitize_exception calls Marshal.dump as a serialisability probe;
// returning an opaque non-nil value here lets the no-error branch
// run, which keeps the original exception as-is. We don't implement
// the actual byte format -- callers that need the buffer round-trip
// will surface as separate gaps.
func bootstrapMarshalModule(env *object.Environment) {
	if _, ok := env.Get("Marshal"); ok {
		return
	}
	c := object.NewClass("Marshal", nil)
	c.IsModule = true
	c.ClassMethods["dump"] = &object.BuiltinMethod{
		Name: "dump",
		Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
			if len(args) < 1 {
				return nil, errorf("evaluator: Marshal.dump expects 1+ args, got %d", len(args))
			}
			var b bytes.Buffer
			if err := marshalDump(env, &b, args[0]); err != nil {
				return raiseBuiltin(env, "TypeError", err.Error())
			}
			return object.NewStringFromBytes(b.Bytes()), nil
		},
	}
	c.ClassMethods["load"] = &object.BuiltinMethod{
		Name: "load",
		Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
			if len(args) < 1 {
				return nil, errorf("evaluator: Marshal.load expects 1+ args, got %d", len(args))
			}
			data, ok := stringText(env, args[0])
			if !ok {
				return raiseBuiltin(env, "TypeError", "Marshal.load: argument must be a String")
			}
			r := bytes.NewReader([]byte(data))
			return marshalLoad(env, r)
		},
	}
	c.ClassMethods["restore"] = c.ClassMethods["load"]
	env.SetGlobal("Marshal", c)
}

// marshalDump writes a minimal binary encoding of v to b. Format
// is internal to goruby (not MRI-compatible); supports primitives
// + Array/Hash recursively.
func marshalDump(env *object.Environment, b *bytes.Buffer, v object.RubyObject) error {
	switch x := v.(type) {
	case *object.Nil:
		b.WriteByte('n')
	case *object.Boolean:
		if x.Value {
			b.WriteByte('t')
		} else {
			b.WriteByte('f')
		}
	case *object.Integer:
		b.WriteByte('i')
		_, _ = fmt.Fprintf(b, "%d", x.Value)
		b.WriteByte(0)
	case *object.Float:
		b.WriteByte('d')
		_, _ = fmt.Fprintf(b, "%v", x.Value)
		b.WriteByte(0)
	case *object.String:
		b.WriteByte('s')
		buf := make([]byte, 4)
		binary.BigEndian.PutUint32(buf, uint32(len(x.Buf)))
		b.Write(buf)
		b.Write(x.Buf)
	case *object.FrozenString:
		s := env.Strings().Get(x.ID)
		b.WriteByte('s')
		buf := make([]byte, 4)
		binary.BigEndian.PutUint32(buf, uint32(len(s)))
		b.Write(buf)
		b.WriteString(s)
	case *object.Symbol:
		b.WriteByte('y')
		name := env.Symbols().Name(x.ID)
		b.WriteString(name)
		b.WriteByte(0)
	case *object.Array:
		b.WriteByte('a')
		buf := make([]byte, 4)
		binary.BigEndian.PutUint32(buf, uint32(len(x.Elements)))
		b.Write(buf)
		for _, e := range x.Elements {
			if err := marshalDump(env, b, e); err != nil {
				return err
			}
		}
	case *object.Hash:
		b.WriteByte('h')
		buf := make([]byte, 4)
		binary.BigEndian.PutUint32(buf, uint32(len(x.Entries)))
		b.Write(buf)
		for _, e := range x.Entries {
			if err := marshalDump(env, b, e.Key); err != nil {
				return err
			}
			if err := marshalDump(env, b, e.Value); err != nil {
				return err
			}
		}
	case *object.Instance:
		// Generic Instance: dump nothing recoverable (round-trip
		// becomes nil). Lets sanitize_exception's probe succeed
		// without erroring -- minitest only checks the no-error
		// branch, doesn't inspect the bytes.
		b.WriteByte('o')
		name := ""
		if x.C != nil {
			name = x.C.Name
		}
		b.WriteString(name)
		b.WriteByte(0)
	default:
		return fmt.Errorf("no _dump_data is defined for class %T", v)
	}
	return nil
}

func marshalLoad(env *object.Environment, r *bytes.Reader) (object.RubyObject, error) {
	tag, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	switch tag {
	case 'n':
		return object.NIL, nil
	case 't':
		return object.TRUE, nil
	case 'f':
		return object.FALSE, nil
	case 'i':
		s, err := readUntilZero(r)
		if err != nil {
			return nil, err
		}
		var n int64
		_, _ = fmt.Sscanf(s, "%d", &n)
		return object.NewInteger(n), nil
	case 'd':
		s, err := readUntilZero(r)
		if err != nil {
			return nil, err
		}
		var f float64
		_, _ = fmt.Sscanf(s, "%v", &f)
		return object.NewFloat(f), nil
	case 's':
		buf := make([]byte, 4)
		if _, err := r.Read(buf); err != nil {
			return nil, err
		}
		n := int(binary.BigEndian.Uint32(buf))
		body := make([]byte, n)
		if n > 0 {
			if _, err := r.Read(body); err != nil {
				return nil, err
			}
		}
		return object.NewStringFromBytes(body), nil
	case 'y':
		s, err := readUntilZero(r)
		if err != nil {
			return nil, err
		}
		return env.Symbols().Intern(s), nil
	case 'a':
		buf := make([]byte, 4)
		if _, err := r.Read(buf); err != nil {
			return nil, err
		}
		n := int(binary.BigEndian.Uint32(buf))
		out := make([]object.RubyObject, 0, n)
		for i := 0; i < n; i++ {
			v, err := marshalLoad(env, r)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		return object.NewArray(out...), nil
	case 'o':
		_, _ = readUntilZero(r)
		// Class name read but discarded; load returns nil for
		// previously-dumped generic instances.
		return object.NIL, nil
	case 'h':
		buf := make([]byte, 4)
		if _, err := r.Read(buf); err != nil {
			return nil, err
		}
		n := int(binary.BigEndian.Uint32(buf))
		entries := make([]object.HashEntry, 0, n)
		for i := 0; i < n; i++ {
			k, err := marshalLoad(env, r)
			if err != nil {
				return nil, err
			}
			v, err := marshalLoad(env, r)
			if err != nil {
				return nil, err
			}
			entries = append(entries, object.HashEntry{Key: k, Value: v})
		}
		return object.NewHash(entries...), nil
	}
	return nil, fmt.Errorf("unknown marshal tag %c", tag)
}

func readUntilZero(r *bytes.Reader) (string, error) {
	var b []byte
	for {
		c, err := r.ReadByte()
		if err != nil {
			return "", err
		}
		if c == 0 {
			return string(b), nil
		}
		b = append(b, c)
	}
}

// bootstrapWarningModule installs Warning -- MRI uses it as the
// warn() target for deprecation messages. Stub: warn delegates to
// env.Stderr. Minitest's process_args references the constant.
func bootstrapWarningModule(env *object.Environment) {
	if _, ok := env.Get("Warning"); ok {
		return
	}
	c := object.NewClass("Warning", nil)
	c.IsModule = true
	c.ClassMethods["warn"] = &object.BuiltinMethod{
		Name: "warn",
		Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
			return kernelWarn(env, args)
		},
	}
	c.ClassMethods["[]"] = &object.BuiltinMethod{
		Name: "[]",
		Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
			return object.FALSE, nil
		},
	}
	c.ClassMethods["[]="] = &object.BuiltinMethod{
		Name: "[]=",
		Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
			if len(args) >= 2 {
				return args[1], nil
			}
			return object.NIL, nil
		},
	}
	env.SetGlobal("Warning", c)
}

// bootstrapGemModule installs the Gem module -- referenced by
// gem-aware code that probes for gemspecs (`gem "name"`, Gem.ruby,
// Gem::LoadError). All paths stub out: gem returns true unless
// a probe wants a strict raise. Sufficient for rake test/helper.rb
// shape where `gem "coveralls"` is wrapped in begin/rescue.
func bootstrapGemModule(env *object.Environment) {
	if _, ok := env.Get("Gem"); ok {
		return
	}
	c := object.NewClass("Gem", nil)
	c.IsModule = true
	loadError := object.NewClass("LoadError", nil)
	if std, ok := env.Get("StandardError"); ok {
		if se, ok := std.(*object.Class); ok {
			loadError.Super = se
		}
	}
	c.Constants["LoadError"] = loadError
	c.ClassMethods["ruby"] = &object.BuiltinMethod{
		Name: "ruby",
		Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
			path, err := os.Executable()
			if err != nil {
				return object.NewString("ruby"), nil
			}
			return object.NewString(path), nil
		},
	}
	c.ClassMethods["loaded_specs"] = &object.BuiltinMethod{
		Name: "loaded_specs",
		Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
			return object.NewHash(), nil
		},
	}
	env.SetGlobal("Gem", c)
	// Kernel#gem - top-level gem lookup. Stub: return true (gem is
	// assumed loaded via the bundled fixture path).
	if k, ok := env.Get("Kernel"); ok {
		if km, ok := k.(*object.Class); ok && km.Methods != nil {
			km.AddMethod("gem", &object.BuiltinMethod{
				Name: "gem",
				Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
					return object.TRUE, nil
				},
			})
		}
	}
}

// bootstrapTimeClass installs a Time class backed by Go's time.Time
// stored on @__unix__ (int64 unix seconds) + @__nsec__ (int64
// nanoseconds-within-second). Surface: Time.now, Time.at(secs),
// Time.mktime(y,m,d,h,m,s), instance comparison via <=>, to_i,
// to_s, strftime (Go layout fallback for common MRI directives).
// Idempotent.
func bootstrapTimeClass(env *object.Environment) {
	if _, ok := env.Get("Time"); ok {
		return
	}
	c := object.NewClass("Time", nil)
	makeInst := func(t time.Time) object.RubyObject {
		inst := object.NewInstance(c)
		inst.Ivars["@__unix__"] = object.NewInteger(t.Unix())
		inst.Ivars["@__nsec__"] = object.NewInteger(int64(t.Nanosecond()))
		return inst
	}
	timeOf := func(recv object.RubyObject) (time.Time, bool) {
		inst, _ := recv.(*object.Instance)
		if inst == nil {
			return time.Time{}, false
		}
		sec, _ := inst.Ivars["@__unix__"].(*object.Integer)
		nsec, _ := inst.Ivars["@__nsec__"].(*object.Integer)
		if sec == nil {
			return time.Time{}, false
		}
		ns := int64(0)
		if nsec != nil {
			ns = nsec.Value
		}
		return time.Unix(sec.Value, ns), true
	}
	c.ClassMethods["now"] = &object.BuiltinMethod{Name: "now", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		return makeInst(time.Now()), nil
	}}
	c.ClassMethods["at"] = &object.BuiltinMethod{Name: "at", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		if len(args) == 0 {
			return nil, errorf("evaluator: Time.at: missing seconds arg")
		}
		sec, ok := args[0].(*object.Integer)
		if !ok {
			return nil, errorf("evaluator: Time.at: expected Integer, got %T", args[0])
		}
		return makeInst(time.Unix(sec.Value, 0)), nil
	}}
	c.ClassMethods["mktime"] = &object.BuiltinMethod{Name: "mktime", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		getInt := func(i int, dflt int) int {
			if i >= len(args) {
				return dflt
			}
			if n, ok := args[i].(*object.Integer); ok {
				return int(n.Value)
			}
			return dflt
		}
		t := time.Date(getInt(0, 1970), time.Month(getInt(1, 1)), getInt(2, 1),
			getInt(3, 0), getInt(4, 0), getInt(5, 0), 0, time.Local)
		return makeInst(t), nil
	}}
	c.ClassMethods["local"] = c.ClassMethods["mktime"]
	// Time.utc / Time.gm: like mktime but UTC zone instead of local.
	c.ClassMethods["utc"] = &object.BuiltinMethod{Name: "utc", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		getInt := func(i int, dflt int) int {
			if i >= len(args) {
				return dflt
			}
			if n, ok := args[i].(*object.Integer); ok {
				return int(n.Value)
			}
			return dflt
		}
		t := time.Date(getInt(0, 1970), time.Month(getInt(1, 1)), getInt(2, 1),
			getInt(3, 0), getInt(4, 0), getInt(5, 0), 0, time.UTC)
		return makeInst(t), nil
	}}
	c.ClassMethods["gm"] = c.ClassMethods["utc"]
	c.Methods["to_i"] = &object.BuiltinMethod{Name: "to_i", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		t, ok := timeOf(recv)
		if !ok {
			return object.NewInteger(0), nil
		}
		return object.NewInteger(t.Unix()), nil
	}}
	c.Methods["to_f"] = &object.BuiltinMethod{Name: "to_f", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		t, ok := timeOf(recv)
		if !ok {
			return object.NewFloat(0), nil
		}
		return object.NewFloat(float64(t.UnixNano()) / 1e9), nil
	}}
	c.Methods["to_s"] = &object.BuiltinMethod{Name: "to_s", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		t, ok := timeOf(recv)
		if !ok {
			return object.NewString(""), nil
		}
		return object.NewString(t.Format("2006-01-02 15:04:05 -0700")), nil
	}}
	c.Methods["inspect"] = c.Methods["to_s"]
	c.Methods["<=>"] = &object.BuiltinMethod{Name: "<=>", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		if len(args) != 1 {
			return object.NIL, nil
		}
		a, ok := timeOf(recv)
		if !ok {
			return object.NIL, nil
		}
		b, bok := timeOf(args[0])
		if !bok {
			// Other side isn't a Time. Try reversed dispatch:
			// other.<=>(self), then negate. Matches MRI's "coerce-style"
			// fallback for cross-type comparisons (Rake::LateTime relies
			// on this so `Time.now < LATE` works).
			if other, ok := args[0].(*object.Instance); ok {
				if m, found := other.C.LookupMethod("<=>"); found {
					var rv object.RubyObject
					var err error
					switch mm := m.(type) {
					case *object.UserMethod:
						rv, err = invokeMethodOn(env, other, mm, []object.RubyObject{recv}, nil)
					case *object.BuiltinMethod:
						rv, err = mm.Fn(env, other, []object.RubyObject{recv}, nil)
					}
					if err == nil {
						if i, ok := rv.(*object.Integer); ok {
							return object.NewInteger(-i.Value), nil
						}
					}
				}
			}
			return object.NIL, nil
		}
		switch {
		case a.Before(b):
			return object.NewInteger(-1), nil
		case a.After(b):
			return object.NewInteger(1), nil
		}
		return object.NewInteger(0), nil
	}}
	// Comparable derivations on ObjectClass pick up <, <=, >, >=
	// automatically from <=>. == is dispatched separately because
	// Comparable doesn't synthesise it -- handle here.
	c.Methods["=="] = &object.BuiltinMethod{Name: "==", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		if len(args) != 1 {
			return object.FALSE, nil
		}
		a, ok := timeOf(recv)
		if !ok {
			return object.FALSE, nil
		}
		b, ok := timeOf(args[0])
		if !ok {
			return object.FALSE, nil
		}
		return object.BooleanOf(a.Equal(b)), nil
	}}
	c.Methods["-"] = &object.BuiltinMethod{Name: "-", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		if len(args) != 1 {
			return nil, errorf("evaluator: Time#-: expected 1 arg")
		}
		a, ok := timeOf(recv)
		if !ok {
			return nil, errorf("evaluator: Time#-: bad receiver")
		}
		if b, ok := timeOf(args[0]); ok {
			return object.NewFloat(a.Sub(b).Seconds()), nil
		}
		// Number arg -> shifts by that many seconds.
		switch n := args[0].(type) {
		case *object.Integer:
			return makeInst(a.Add(-time.Duration(n.Value) * time.Second)), nil
		case *object.Float:
			return makeInst(a.Add(-time.Duration(n.Value * float64(time.Second)))), nil
		}
		return nil, errorf("evaluator: Time#-: expected Time/Numeric, got %T", args[0])
	}}
	c.Methods["strftime"] = &object.BuiltinMethod{Name: "strftime", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		if len(args) != 1 {
			return nil, errorf("evaluator: Time#strftime: expected 1 arg")
		}
		fmtStr, ok := args[0].(*object.String)
		if !ok {
			return nil, errorf("evaluator: Time#strftime: expected String, got %T", args[0])
		}
		t, ok := timeOf(recv)
		if !ok {
			return object.NewString(""), nil
		}
		return object.NewString(strftime(fmtStr.Value(), t)), nil
	}}
	timeInt := func(extract func(t time.Time) int) *object.BuiltinMethod {
		return &object.BuiltinMethod{Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
			t, ok := timeOf(recv)
			if !ok {
				return object.NewInteger(0), nil
			}
			return object.NewInteger(int64(extract(t))), nil
		}}
	}
	c.Methods["year"] = timeInt(func(t time.Time) int { return t.Year() })
	c.Methods["month"] = timeInt(func(t time.Time) int { return int(t.Month()) })
	c.Methods["mon"] = c.Methods["month"]
	c.Methods["day"] = timeInt(func(t time.Time) int { return t.Day() })
	c.Methods["mday"] = c.Methods["day"]
	c.Methods["hour"] = timeInt(func(t time.Time) int { return t.Hour() })
	c.Methods["min"] = timeInt(func(t time.Time) int { return t.Minute() })
	c.Methods["sec"] = timeInt(func(t time.Time) int { return t.Second() })
	c.Methods["wday"] = timeInt(func(t time.Time) int { return int(t.Weekday()) })
	c.Methods["yday"] = timeInt(func(t time.Time) int { return t.YearDay() })
	c.Methods["usec"] = timeInt(func(t time.Time) int { return t.Nanosecond() / 1000 })
	c.Methods["nsec"] = timeInt(func(t time.Time) int { return t.Nanosecond() })
	env.SetGlobal("Time", c)
}

// strftime implements a subset of MRI's strftime directives, enough
// for typical %Y/%m/%d/%H/%M/%S/%A/%B uses. Unknown directives pass
// through literally.
func strftime(fmt string, t time.Time) string {
	var b strings.Builder
	for i := 0; i < len(fmt); i++ {
		c := fmt[i]
		if c != '%' || i+1 >= len(fmt) {
			b.WriteByte(c)
			continue
		}
		i++
		switch fmt[i] {
		case 'Y':
			b.WriteString(t.Format("2006"))
		case 'm':
			b.WriteString(t.Format("01"))
		case 'd':
			b.WriteString(t.Format("02"))
		case 'H':
			b.WriteString(t.Format("15"))
		case 'M':
			b.WriteString(t.Format("04"))
		case 'S':
			b.WriteString(t.Format("05"))
		case 'A':
			b.WriteString(t.Weekday().String())
		case 'a':
			b.WriteString(t.Format("Mon"))
		case 'B':
			b.WriteString(t.Month().String())
		case 'b':
			b.WriteString(t.Format("Jan"))
		case '%':
			b.WriteByte('%')
		default:
			b.WriteByte('%')
			b.WriteByte(fmt[i])
		}
	}
	return b.String()
}

// bootstrapMonitor installs a placeholder Monitor module. Real
// MRI Monitor is `Mutex` + reentrancy tracking; rake's TaskManager
// only uses it for `@monitor.synchronize { ... }` style guards which
// we can flatten to direct block execution. synchronize calls the
// block with no args; no-block calls return self. Idempotent.
func bootstrapMonitor(env *object.Environment) {
	if _, ok := env.Get("Monitor"); ok {
		return
	}
	c := object.NewClass("Monitor", nil)
	c.Methods["synchronize"] = &object.BuiltinMethod{
		Name: "synchronize",
		Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
			if bm, ok := block.(*goBlockMarker); ok && bm != nil && bm.fn != nil {
				return bm.fn(nil)
			}
			return recv, nil
		},
	}
	// new_cond returns a ConditionVariable bound to the monitor. With
	// serial-fallback semantics there's nothing to wait on -- wait /
	// signal / broadcast all noop. Used by rake's ThreadPool#join
	// when the queue drains.
	condCls := object.NewClass("ConditionVariable", nil)
	condCls.Methods["wait"] = &object.BuiltinMethod{Name: "wait", Fn: func(*object.Environment, object.RubyObject, []object.RubyObject, any) (object.RubyObject, error) {
		return object.NIL, nil
	}}
	condCls.Methods["signal"] = condCls.Methods["wait"]
	condCls.Methods["broadcast"] = condCls.Methods["wait"]
	c.Methods["new_cond"] = &object.BuiltinMethod{
		Name: "new_cond",
		Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
			return object.NewInstance(condCls), nil
		},
	}
	env.SetGlobal("Monitor", c)
}

// bootstrapQueueClass installs a Queue class backed by an @items
// Array. enq/<< / push append, deq/pop/shift removes from front,
// empty?/size/length report state. Mirrors MRI's stdlib Thread::Queue
// well enough for rake's ThreadPool@queue use. With serial-fallback
// Thread semantics the threadsafe-blocking aspects are noops --
// deq(true) on an empty queue raises ThreadError to match MRI.
func bootstrapQueueClass(env *object.Environment) {
	if _, ok := env.Get("Queue"); ok {
		return
	}
	c := object.NewClass("Queue", nil)
	c.ClassMethods["new"] = &object.BuiltinMethod{Name: "new", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		inst := object.NewInstance(c)
		inst.Ivars["@items"] = object.NewArray()
		return inst, nil
	}}
	itemsOf := func(recv object.RubyObject) *object.Array {
		inst, _ := recv.(*object.Instance)
		if inst == nil {
			return nil
		}
		if a, ok := inst.Ivars["@items"].(*object.Array); ok {
			return a
		}
		a := object.NewArray()
		inst.Ivars["@items"] = a
		return a
	}
	c.Methods["enq"] = &object.BuiltinMethod{Name: "enq", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		if len(args) >= 1 {
			itemsOf(recv).Elements = append(itemsOf(recv).Elements, args[0])
		}
		return recv, nil
	}}
	c.Methods["<<"] = c.Methods["enq"]
	c.Methods["push"] = c.Methods["enq"]
	c.Methods["deq"] = &object.BuiltinMethod{Name: "deq", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		a := itemsOf(recv)
		if len(a.Elements) == 0 {
			// deq(true) is non-blocking; MRI raises ThreadError on
			// empty. Plain deq blocks, but we don't model blocking;
			// raise the same error so callers expecting it (rake's
			// process_queue_item) catch and handle the empty case.
			return raiseBuiltin(env, "ThreadError", "queue empty")
		}
		v := a.Elements[0]
		a.Elements = a.Elements[1:]
		return v, nil
	}}
	c.Methods["pop"] = c.Methods["deq"]
	c.Methods["shift"] = c.Methods["deq"]
	c.Methods["empty?"] = &object.BuiltinMethod{Name: "empty?", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		return object.BooleanOf(len(itemsOf(recv).Elements) == 0), nil
	}}
	c.Methods["size"] = &object.BuiltinMethod{Name: "size", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		return object.NewInteger(int64(len(itemsOf(recv).Elements))), nil
	}}
	c.Methods["length"] = c.Methods["size"]
	env.SetGlobal("Queue", c)
}

// bootstrapMutexClass installs Mutex with synchronize / try_lock /
// unlock. Serial fallback: try_lock always succeeds (no contention
// possible w/ single goroutine), synchronize just runs the block.
// Idempotent.
func bootstrapMutexClass(env *object.Environment) {
	if _, ok := env.Get("Mutex"); ok {
		return
	}
	c := object.NewClass("Mutex", nil)
	c.ClassMethods["new"] = &object.BuiltinMethod{Name: "new", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		return object.NewInstance(c), nil
	}}
	c.Methods["synchronize"] = &object.BuiltinMethod{
		Name: "synchronize",
		Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
			if bm, ok := block.(*goBlockMarker); ok && bm != nil && bm.fn != nil {
				return bm.fn(nil)
			}
			return recv, nil
		},
	}
	c.Methods["try_lock"] = &object.BuiltinMethod{Name: "try_lock", Fn: func(*object.Environment, object.RubyObject, []object.RubyObject, any) (object.RubyObject, error) {
		return object.TRUE, nil
	}}
	c.Methods["lock"] = &object.BuiltinMethod{Name: "lock", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		return recv, nil
	}}
	c.Methods["unlock"] = c.Methods["lock"]
	c.Methods["locked?"] = &object.BuiltinMethod{Name: "locked?", Fn: func(*object.Environment, object.RubyObject, []object.RubyObject, any) (object.RubyObject, error) {
		return object.FALSE, nil
	}}
	env.SetGlobal("Mutex", c)
}

// makeQueueAccessibleAsThreadQueue ensures Queue is reachable both
// as top-level Queue and Thread::Queue. Modern minitest uses the
// latter shape.
func makeQueueAccessibleAsThreadQueue(env *object.Environment, threadCls *object.Class) {
	if q, ok := env.Get("Queue"); ok {
		if qc, ok := q.(*object.Class); ok {
			threadCls.Constants["Queue"] = qc
		}
	}
}

// bootstrapThreadClass installs a Thread class with new (serial-
// fallback inline-invoke), current (returns a placeholder thread
// object), and the few introspection methods rake reads. Idempotent.
func bootstrapThreadClass(env *object.Environment) {
	if _, ok := env.Get("Thread"); ok {
		return
	}
	c := object.NewClass("Thread", nil)
	c.ClassMethods["new"] = &object.BuiltinMethod{
		Name: "new",
		Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
			inst := object.NewInstance(c)
			inst.Ivars["@status"] = object.NewString("run")
			if bm, ok := block.(*goBlockMarker); ok && bm != nil && bm.fn != nil {
				// Serial fallback: run the block immediately on the
				// calling goroutine. Result + raised error become the
				// thread's terminal state.
				_, err := bm.fn(nil)
				if err != nil {
					inst.Ivars["@status"] = object.FALSE
					return inst, err
				}
			}
			inst.Ivars["@status"] = object.FALSE // joined / dead
			return inst, nil
		},
	}
	c.ClassMethods["current"] = &object.BuiltinMethod{
		Name: "current",
		Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
			// Memoize the main-thread Instance so Thread.current[:key]
			// reads/writes hit the same fiber-local-style store across
			// repeated calls. We model fiber-locals via Ivars on the
			// instance.
			if v, ok := env.Get("__main_thread__"); ok {
				if inst, ok := v.(*object.Instance); ok {
					return inst, nil
				}
			}
			inst := object.NewInstance(c)
			env.SetGlobal("__main_thread__", inst)
			return inst, nil
		},
	}
	c.Methods["[]"] = &object.BuiltinMethod{Name: "[]", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		inst, ok := recv.(*object.Instance)
		if !ok || len(args) != 1 {
			return object.NIL, nil
		}
		key, ok := symbolOrString(env, args[0])
		if !ok {
			return object.NIL, nil
		}
		if v, ok := inst.Ivars["@@"+key]; ok {
			return v, nil
		}
		return object.NIL, nil
	}}
	c.Methods["[]="] = &object.BuiltinMethod{Name: "[]=", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		inst, ok := recv.(*object.Instance)
		if !ok || len(args) != 2 {
			return object.NIL, nil
		}
		key, ok := symbolOrString(env, args[0])
		if !ok {
			return object.NIL, nil
		}
		if inst.Ivars == nil {
			inst.Ivars = map[string]object.RubyObject{}
		}
		inst.Ivars["@@"+key] = args[1]
		return args[1], nil
	}}
	c.Methods["status"] = &object.BuiltinMethod{Name: "status", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		if inst, ok := recv.(*object.Instance); ok {
			if v, ok := inst.Ivars["@status"]; ok {
				return v, nil
			}
		}
		return object.FALSE, nil
	}}
	c.Methods["join"] = &object.BuiltinMethod{Name: "join", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		return recv, nil
	}}
	c.Methods["alive?"] = &object.BuiltinMethod{Name: "alive?", Fn: func(*object.Environment, object.RubyObject, []object.RubyObject, any) (object.RubyObject, error) {
		return object.FALSE, nil
	}}
	env.SetGlobal("Thread", c)
	makeQueueAccessibleAsThreadQueue(env, c)
}

// bootstrapDir installs the Dir module with the class methods rake
// reads at load: pwd, chdir (block form caller-restores cwd). Glob
// is also commonly needed; install with a minimal recursive pattern
// implementation. Idempotent.
func bootstrapDir(env *object.Environment) {
	if _, ok := env.Get("Dir"); ok {
		return
	}
	c := object.NewClass("Dir", nil)
	c.IsModule = true
	c.ClassMethods["pwd"] = &object.UserMethod{Name: "pwd", Body: nativeFn{Fn: func(_ *object.Environment, _ []object.RubyObject) (object.RubyObject, error) {
		wd, err := os.Getwd()
		if err != nil {
			return object.NewString("."), nil
		}
		return object.NewString(wd), nil
	}}}
	c.ClassMethods["getwd"] = c.ClassMethods["pwd"]
	c.ClassMethods["chdir"] = &object.BuiltinMethod{Name: "chdir", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		if len(args) != 1 {
			return nil, errorf("evaluator: Dir.chdir: expected 1 arg, got %d", len(args))
		}
		path, ok := args[0].(*object.String)
		if !ok {
			return nil, errorf("evaluator: Dir.chdir: expected String, got %T", args[0])
		}
		// Block form: chdir, run block, chdir back, return block value.
		if bm, ok := block.(*goBlockMarker); ok && bm != nil && bm.fn != nil {
			orig, _ := os.Getwd()
			if err := os.Chdir(path.Value()); err != nil {
				return nil, errorf("evaluator: Dir.chdir: %s", err.Error())
			}
			defer os.Chdir(orig)
			return bm.fn(nil)
		}
		if err := os.Chdir(path.Value()); err != nil {
			return nil, errorf("evaluator: Dir.chdir: %s", err.Error())
		}
		return object.NewInteger(0), nil
	}}
	c.ClassMethods["tmpdir"] = &object.UserMethod{Name: "tmpdir", Body: nativeFn{Fn: func(_ *object.Environment, _ []object.RubyObject) (object.RubyObject, error) {
		return object.NewString(os.TempDir()), nil
	}}}
	c.ClassMethods["glob"] = &object.UserMethod{Name: "glob", Body: nativeFn{Fn: func(_ *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
		if len(args) < 1 {
			return nil, errorf("evaluator: Dir.glob: expected pattern arg")
		}
		pat, ok := args[0].(*object.String)
		if !ok {
			return nil, errorf("evaluator: Dir.glob: expected String pattern, got %T", args[0])
		}
		// filepath.Glob handles the common single-segment patterns
		// (* / ? / [...]) rake hits. MRI extras like ** (recursive)
		// fall through unsupported -- callers needing them can run on
		// a deeper impl later.
		matches, err := filepath.Glob(pat.Value())
		if err != nil {
			return nil, errorf("evaluator: Dir.glob: %s", err.Error())
		}
		out := make([]object.RubyObject, len(matches))
		for i, m := range matches {
			out[i] = object.NewString(m)
		}
		return object.NewArray(out...), nil
	}}}
	// Dir[pattern] is the bracket-form alias for Dir.glob.
	c.ClassMethods["[]"] = c.ClassMethods["glob"]
	c.ClassMethods["mktmpdir"] = &object.BuiltinMethod{Name: "mktmpdir", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		prefix := "d"
		if len(args) >= 1 {
			if s, ok := args[0].(*object.String); ok {
				prefix = s.Value()
			}
		}
		path, err := os.MkdirTemp("", prefix)
		if err != nil {
			return nil, errorf("evaluator: Dir.mktmpdir: %s", err.Error())
		}
		if block == nil {
			return object.NewString(path), nil
		}
		// Block form: yield the path, ensure removal on exit.
		defer os.RemoveAll(path)
		return invokeBlockValue(env, block, []object.RubyObject{object.NewString(path)})
	}}
	env.SetGlobal("Dir", c)
}

// bootstrapSingleton installs the Singleton module with an `included`
// hook that synthesises `.instance` on the including class -- the
// minimum needed for `class Foo; include Singleton; end; Foo.instance`
// to produce a cached single object. Skips the constructor-visibility
// swap MRI does (private new); callers that rely on it can fail
// later, that's a smaller gap. Idempotent.
func bootstrapSingleton(env *object.Environment) {
	if _, ok := env.Get("Singleton"); ok {
		return
	}
	c := object.NewClass("Singleton", nil)
	c.IsModule = true
	c.ClassMethods["included"] = &object.BuiltinMethod{
		Name: "included",
		Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
			if len(args) != 1 {
				return nil, errorf("evaluator: Singleton.included: expected 1 arg, got %d", len(args))
			}
			base, ok := args[0].(*object.Class)
			if !ok {
				return nil, errorf("evaluator: Singleton.included: expected Class, got %T", args[0])
			}
			base.ClassMethods["instance"] = &object.BuiltinMethod{
				Name: "instance",
				Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
					cls, ok := recv.(*object.Class)
					if !ok {
						return nil, errorf("evaluator: Singleton#instance: expected Class receiver, got %T", recv)
					}
					if cls.Ivars == nil {
						cls.Ivars = map[string]object.RubyObject{}
					}
					if inst, ok := cls.Ivars["@__singleton_instance"]; ok {
						return inst, nil
					}
					inst := object.NewInstance(cls)
					cls.Ivars["@__singleton_instance"] = inst
					return inst, nil
				},
			}
			return object.NIL, nil
		},
	}
	env.SetGlobal("Singleton", c)
}

// bootstrapFileUtils installs a placeholder FileUtils module so gems
// that `include FileUtils` or iterate FileUtils.commands at load time
// don't NameError. The module is intentionally empty -- .commands
// returns [] so any `FileUtils.commands.each { ... }` loop is a
// noop. Real implementations of cp/rm/mv/mkdir_p/... can land later
// when an execution-rung fixture needs them; the placeholder gets
// us through the load phase. Idempotent.
func bootstrapFileUtils(env *object.Environment) {
	if _, ok := env.Get("FileUtils"); ok {
		return
	}
	c := object.NewClass("FileUtils", nil)
	c.IsModule = true
	emptyList := func(_ *object.Environment, _ []object.RubyObject) (object.RubyObject, error) {
		return object.NewArray(), nil
	}
	c.ClassMethods["commands"] = &object.UserMethod{Name: "commands", Body: nativeFn{Fn: emptyList}}
	c.ClassMethods["options_of"] = &object.UserMethod{Name: "options_of", Body: nativeFn{Fn: emptyList}}
	c.ClassMethods["options"] = &object.UserMethod{Name: "options", Body: nativeFn{Fn: emptyList}}

	// Real fs ops backed by os pkg. Rake's test/helper.rb setup +
	// teardown call mkdir_p / rm_rf; the test suite expects real
	// behaviour, not a noop.
	c.ClassMethods["mkdir_p"] = &object.UserMethod{Name: "mkdir_p", Body: nativeFn{Fn: func(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
		for _, a := range args {
			s, ok := stringText(env, a)
			if !ok {
				continue
			}
			if err := os.MkdirAll(s, 0755); err != nil {
				return raiseBuiltin(env, "IOError", err.Error())
			}
		}
		return object.NIL, nil
	}}}
	c.ClassMethods["mkdir"] = c.ClassMethods["mkdir_p"]
	c.ClassMethods["rm_rf"] = &object.UserMethod{Name: "rm_rf", Body: nativeFn{Fn: func(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
		for _, a := range args {
			s, ok := stringText(env, a)
			if !ok {
				if arr, ok := a.(*object.Array); ok {
					for _, e := range arr.Elements {
						if t, ok := stringText(env, e); ok {
							_ = os.RemoveAll(t)
						}
					}
				}
				continue
			}
			_ = os.RemoveAll(s)
		}
		return object.NIL, nil
	}}}
	c.ClassMethods["rm_f"] = c.ClassMethods["rm_rf"]
	c.ClassMethods["rm"] = c.ClassMethods["rm_rf"]
	c.ClassMethods["rm_r"] = c.ClassMethods["rm_rf"]
	c.ClassMethods["remove_entry_secure"] = c.ClassMethods["rm_rf"]
	c.ClassMethods["cp"] = &object.UserMethod{Name: "cp", Body: nativeFn{Fn: func(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
		if len(args) < 2 {
			return nil, errorf("evaluator: FileUtils.cp: expected 2 args")
		}
		src, _ := stringText(env, args[0])
		dst, _ := stringText(env, args[1])
		data, err := os.ReadFile(src)
		if err != nil {
			return raiseBuiltin(env, "IOError", err.Error())
		}
		if err := os.WriteFile(dst, data, 0644); err != nil {
			return raiseBuiltin(env, "IOError", err.Error())
		}
		return object.NIL, nil
	}}}
	c.ClassMethods["touch"] = &object.UserMethod{Name: "touch", Body: nativeFn{Fn: func(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
		for _, a := range args {
			s, ok := stringText(env, a)
			if !ok {
				continue
			}
			f, err := os.OpenFile(s, os.O_RDWR|os.O_CREATE, 0644)
			if err == nil {
				_ = f.Close()
			}
		}
		return object.NIL, nil
	}}}
	// FileUtils.chmod -- delegate to os.Chmod.
	c.ClassMethods["chmod"] = &object.UserMethod{Name: "chmod", Body: nativeFn{Fn: func(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
		if len(args) < 2 {
			return nil, errorf("evaluator: FileUtils.chmod: expected mode + paths")
		}
		modeInt, ok := args[0].(*object.Integer)
		if !ok {
			return nil, errorf("evaluator: FileUtils.chmod: expected Integer mode, got %T", args[0])
		}
		for _, p := range args[1:] {
			s, ok := stringText(env, p)
			if !ok {
				if arr, ok := p.(*object.Array); ok {
					for _, e := range arr.Elements {
						if t, ok := stringText(env, e); ok {
							_ = os.Chmod(t, os.FileMode(modeInt.Value))
						}
					}
				}
				continue
			}
			_ = os.Chmod(s, os.FileMode(modeInt.Value))
		}
		return object.NIL, nil
	}}}
	c.ClassMethods["chmod_R"] = c.ClassMethods["chmod"]
	// FileUtils ships its methods as both module-level (class methods)
	// AND instance methods on includers (it's a module that uses
	// module_function). Mirror by also exposing the same impls as
	// Methods so `include FileUtils` + `mkdir_p ...` from the
	// includer's instance dispatches correctly.
	c.IsModule = true
	for name, m := range c.ClassMethods {
		if _, ok := c.Methods[name]; !ok {
			c.Methods[name] = m
		}
	}
	env.SetGlobal("FileUtils", c)
}

// bootstrapRbConfig installs a minimal RbConfig::CONFIG hash so the
// many `require "rbconfig"; RbConfig::CONFIG[k]` shapes in real-world
// gems (rake's backtrace.rb / file_utils.rb / win32.rb / cpu_counter
// / application all read it at load time) don't NameError. Values
// are best-effort: host_os maps from runtime.GOOS, the rest are
// neutral placeholders that callers commonly tolerate
// (empty strings, "ruby", etc). Promoting individual keys to live
// values is fine when a callsite shows the placeholder is misleading.
// Idempotent.
func bootstrapRbConfig(env *object.Environment) {
	if _, ok := env.Get("RbConfig"); ok {
		return
	}
	c := object.NewClass("RbConfig", nil)
	c.IsModule = true
	cfg := object.NewHash()
	put := func(k, v string) {
		cfg.Entries = append(cfg.Entries, object.HashEntry{
			Key:   object.NewString(k),
			Value: object.NewString(v),
		})
	}
	hostOS := "linux"
	switch goos := runtime.GOOS; goos {
	case "darwin":
		hostOS = "darwin"
	case "windows":
		hostOS = "mswin"
	case "freebsd", "openbsd", "netbsd":
		hostOS = goos
	}
	put("host_os", hostOS)
	put("bindir", "/__goruby_sys__/bin")
	put("ruby_install_name", "ruby")
	put("EXEEXT", "")
	put("prefix", "/__goruby_sys__")
	put("libdir", "/__goruby_sys__/lib")
	put("sitelibdir", "/__goruby_sys__/site")
	put("rubylibdir", "/__goruby_sys__/lib/ruby")
	// rubylibprefix is used by rake/backtrace.rb to suppress "system"
	// stdlib frames. A nonexistent sentinel path won't match any real
	// frame but is distinct enough that backtrace logic that explicitly
	// builds a path from it (e.g. `path + ":12"`) still produces a
	// usable string.
	put("rubylibprefix", "/__goruby_sys__")
	put("RUBY_INSTALL_NAME", "ruby")
	put("ruby_version", "3.4.0")
	c.Constants["CONFIG"] = cfg
	env.SetGlobal("RbConfig", c)
}

// bootstrapENV installs the ENV constant: a Hash snapshot of the
// process environment at startup. Read access (ENV["X"]) returns the
// current snapshot value or nil for unset names. Writes (ENV["X"]="y")
// modify the snapshot only -- they do NOT propagate to the actual
// process env. Acceptable for now: rake & friends use ENV for
// load-time configuration reads; the few writes that happen are
// runtime-invocation noise. If a corpus fixture surfaces a real write
// dependency we can promote ENV to a live-proxy Class with class
// methods delegating to os.Setenv. Idempotent.
func bootstrapENV(env *object.Environment) {
	if _, ok := env.Get("ENV"); ok {
		return
	}
	h := object.NewHash()
	for _, kv := range os.Environ() {
		eq := strings.IndexByte(kv, '=')
		if eq < 0 {
			continue
		}
		key := object.NewString(kv[:eq])
		val := object.NewString(kv[eq+1:])
		h.Entries = append(h.Entries, object.HashEntry{Key: key, Value: val})
	}
	env.SetGlobal("ENV", h)
}

// bootstrapLoadPath installs the `$LOAD_PATH` global as an empty Array
// (also bound to the `$:` alias) so callers can `$LOAD_PATH.unshift
// "<dir>"` before requiring. kernelRequire iterates the entries when
// resolving a non-stdlib name. Idempotent.
func bootstrapLoadPath(env *object.Environment) {
	if _, ok := env.Get("$LOAD_PATH"); ok {
		return
	}
	arr := object.NewArray()
	env.SetGlobal("$LOAD_PATH", arr)
	// Ruby aliases `$:` to the same array object. Bind to the same
	// pointer so push/unshift on either side is observed by both.
	env.SetGlobal("$:", arr)
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

// newOpenWriteIO produces a write-buffer Instance for Kernel#open in
// "w"/"a" modes. The path + append flag live on ivars; << / write /
// puts / print all append to @__buf__; flushOpenWriteIO drains to disk.
func newOpenWriteIO(path string, appendMode bool) *object.Instance {
	if openWriteIOClass == nil {
		c := object.NewClass("WriteIO", nil)
		c.AddMethod("<<", &object.BuiltinMethod{Name: "<<", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
			inst := recv.(*object.Instance)
			s, _ := stringText(env, args[0])
			buf, _ := inst.Ivars["@__buf__"].(*object.String)
			buf.Buf = append(buf.Buf, []byte(s)...)
			return recv, nil
		}})
		c.AddMethod("write", &object.BuiltinMethod{Name: "write", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
			inst := recv.(*object.Instance)
			total := 0
			buf, _ := inst.Ivars["@__buf__"].(*object.String)
			for _, a := range args {
				s, _ := stringText(env, a)
				buf.Buf = append(buf.Buf, []byte(s)...)
				total += len(s)
			}
			return object.NewInteger(int64(total)), nil
		}})
		c.AddMethod("puts", &object.BuiltinMethod{Name: "puts", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
			inst := recv.(*object.Instance)
			buf, _ := inst.Ivars["@__buf__"].(*object.String)
			for _, a := range args {
				s, _ := stringText(env, a)
				buf.Buf = append(buf.Buf, []byte(s)...)
				if len(s) == 0 || s[len(s)-1] != '\n' {
					buf.Buf = append(buf.Buf, '\n')
				}
			}
			if len(args) == 0 {
				buf.Buf = append(buf.Buf, '\n')
			}
			return object.NIL, nil
		}})
		c.AddMethod("print", &object.BuiltinMethod{Name: "print", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
			inst := recv.(*object.Instance)
			buf, _ := inst.Ivars["@__buf__"].(*object.String)
			for _, a := range args {
				s, _ := stringText(env, a)
				buf.Buf = append(buf.Buf, []byte(s)...)
			}
			return object.NIL, nil
		}})
		c.AddMethod("close", &object.BuiltinMethod{Name: "close", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
			return object.NIL, nil
		}})
		c.AddMethod("path", &object.BuiltinMethod{Name: "path", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
			return recv.(*object.Instance).Ivars["@__path__"], nil
		}})
		openWriteIOClass = c
	}
	inst := object.NewInstance(openWriteIOClass)
	inst.Ivars["@__path__"] = object.NewString(path)
	inst.Ivars["@__append__"] = object.BooleanOf(appendMode)
	inst.Ivars["@__buf__"] = object.NewString("")
	return inst
}

var openWriteIOClass *object.Class

// flushOpenWriteIO writes the buffered bytes to disk.
func flushOpenWriteIO(inst *object.Instance) error {
	pathStr, _ := inst.Ivars["@__path__"].(*object.String)
	buf, _ := inst.Ivars["@__buf__"].(*object.String)
	appendMode, _ := inst.Ivars["@__append__"].(*object.Boolean)
	flags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	if appendMode != nil && appendMode.Value {
		flags = os.O_WRONLY | os.O_CREATE | os.O_APPEND
	}
	f, err := os.OpenFile(pathStr.Value(), flags, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(buf.Buf)
	return err
}

// bootstrapPathnameClass installs a tiny Pathname surface: Pathname.new(str)
// stores the string on @path and exposes to_s / to_path / to_str so any
// callee that converts via duck-typing gets the underlying string. Rake's
// internal Rake.from_pathname uses to_s, so this is enough.
func bootstrapPathnameClass(env *object.Environment) {
	if _, ok := env.Get("Pathname"); ok {
		return
	}
	c := object.NewClass("Pathname", nil)
	c.ClassMethods["new"] = &object.BuiltinMethod{Name: "new", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		if len(args) != 1 {
			return nil, errorf("evaluator: Pathname.new: expected 1 arg, got %d", len(args))
		}
		s, ok := stringText(env, args[0])
		if !ok {
			return nil, errorf("evaluator: Pathname.new: expected String, got %T", args[0])
		}
		inst := object.NewInstance(c)
		inst.Ivars["@path"] = object.NewString(s)
		return inst, nil
	}}
	toS := &object.BuiltinMethod{Name: "to_s", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		inst := recv.(*object.Instance)
		if v, ok := inst.Ivars["@path"]; ok {
			return v, nil
		}
		return object.NewString(""), nil
	}}
	c.AddMethod("to_s", toS)
	c.AddMethod("to_path", toS)
	c.AddMethod("to_str", toS)
	c.AddMethod("inspect", &object.BuiltinMethod{Name: "inspect", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		inst := recv.(*object.Instance)
		s := ""
		if v, ok := inst.Ivars["@path"].(*object.String); ok {
			s = v.Value()
		}
		return object.NewString("#<Pathname:" + s + ">"), nil
	}})
	c.AddMethod("==", &object.BuiltinMethod{Name: "==", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		if len(args) != 1 {
			return object.FALSE, nil
		}
		l, _ := stringText(env, recv.(*object.Instance).Ivars["@path"])
		r, _ := stringText(env, args[0])
		return object.BooleanOf(l == r), nil
	}})
	env.SetGlobal("Pathname", c)
}
