package evaluator

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/lczyk/goruby/ast"
	"github.com/lczyk/goruby/evaluator/stdlib"
	"github.com/lczyk/goruby/object"
	"github.com/lczyk/goruby/parser"
)

// callKernel dispatches Kernel-level (implicit-self) calls. Until a full
// method registry exists, the literals corpus only needs `puts` and `p`.
// User-defined top-level methods take precedence over kernel builtins
// (matching MRI's `Object#method` resolution).
func callKernel(env *object.Environment, name string, args []object.RubyObject) (object.RubyObject, error) {
	if m, ok := env.GetMethod(name); ok {
		if um, ok := m.(*object.UserMethod); ok {
			return callUserMethod(env, um, args)
		}
	}
	switch name {
	case "puts":
		return kernelPuts(env, args)
	case "warn":
		return kernelWarn(env, args)
	case "p", "pp":
		return kernelP(env, args)
	case "print":
		return kernelPrint(env, args)
	case "printf":
		return kernelPrintf(env, args)
	case "sprintf", "format":
		return kernelSprintf(env, args)
	case "raise", "fail":
		return kernelRaise(env, args)
	case "private_class_method", "public_class_method":
		// Top-level noop. Per-method class-method visibility isn't
		// modelled; the call exists to silence rake/clean.rb's
		// Cleaner module body invocations.
		return object.NIL, nil
	case "Integer":
		return kernelInteger(env, args)
	case "Float":
		return kernelFloat(env, args)
	case "String":
		return kernelString(env, args)
	case "Array":
		return kernelArray(args)
	case "gets":
		return stdinGets(env, args)
	case "putc":
		return kernelPutc(env, args)
	case "eval":
		return kernelEval(env, args)
	case "exit", "exit!":
		return kernelExit(env, args)
	case "abort":
		return kernelAbort(env, args)
	case "require_relative":
		return kernelRequireRelative(env, args)
	case "require":
		return kernelRequire(env, args)
	case "load":
		return kernelLoad(env, args)
	case "sleep":
		// MRI Kernel#sleep([dur]): sleeps for dur seconds (default
		// forever). We don't model wall-clock waits meaningfully in
		// the corpus, so accept Integer/Float and return the
		// rounded-down seconds value without actually sleeping. Rake's
		// multitask test calls sleep purely to interleave threads.
		if len(args) == 0 {
			return object.NewInteger(0), nil
		}
		switch d := args[0].(type) {
		case *object.Integer:
			return object.NewInteger(d.Value), nil
		case *object.Float:
			return object.NewInteger(int64(d.Value)), nil
		}
		return object.NewInteger(0), nil
	case "rand":
		return kernelRand(env, args)
	case "srand":
		return kernelSrand(env, args)
	case "lambda":
		return nil, errorf("evaluator: Kernel#lambda without block not supported; use ->( ){ ... }")
	case "Complex":
		return stdlib.KernelComplex(env, args)
	case "caller":
		// Read the root-env call stack. Each frame is the callee
		// (current method) at push time. MRI's caller drops the
		// topmost frame (the receiver of caller itself) and returns
		// the outer callers innermost-first.
		stk := env.CallStack()
		if len(stk) <= 1 {
			return object.NewArray(), nil
		}
		out := make([]object.RubyObject, 0, len(stk)-1)
		for i := len(stk) - 2; i >= 0; i-- {
			out = append(out, object.NewString(stk[i]))
		}
		return object.NewArray(out...), nil
	case "system":
		return kernelSystem(env, args)
	case "`":
		return kernelBacktick(env, args)
	}
	return raiseBuiltin(env, "NoMethodError", "undefined method `"+name+"' for main:Object")
}

// kernelPutc implements Kernel#putc: writes a single character /
// byte to stdout. Accepts an Integer (taken mod 256) or a String
// (writes its first byte). Returns the argument.
func kernelPutc(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	if len(args) != 1 {
		return nil, errorf("evaluator: putc: wrong number of arguments (given %d, expected 1)", len(args))
	}
	w := env.Stdout()
	switch v := args[0].(type) {
	case *object.Integer:
		b := byte(v.Value & 0xff)
		_, _ = w.Write([]byte{b})
		return v, nil
	case *object.String:
		if len(v.Buf) == 0 {
			return v, nil
		}
		_, _ = w.Write(v.Buf[:1])
		return v, nil
	}
	return nil, errorf("evaluator: putc: expected Integer or String, got %T", args[0])
}

// kernelExit implements Kernel#exit / Kernel#exit!: raise the
// exitSignal which evalProgram catches and converts to a clean return.
// Accepts an Integer exit code or a Boolean (true=0, false=1). No
// difference from exit! at this level; we don't run at_exit / ensures
// either way.
func kernelExit(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	var code int64
	if len(args) >= 1 {
		switch v := args[0].(type) {
		case *object.Integer:
			code = v.Value
		case *object.Boolean:
			if !v.Value {
				code = 1
			}
		}
	}
	// Raise as a Ruby SystemExit so `rescue SystemExit` and
	// `rescue Exception` can catch. evalProgram unwraps an
	// unrescued SystemExit into the exitSignal shape.
	bootstrapExceptionHierarchy(env)
	if cls, ok := env.Get("SystemExit"); ok {
		if c, ok := cls.(*object.Class); ok {
			exc := newExceptionInstance(c, "exit")
			exc.Ivars["@status"] = object.NewInteger(code)
			attachBacktrace(env, exc)
			return nil, &raiseSignal{Exception: exc}
		}
	}
	return nil, &exitSignal{Code: code}
}

// kernelSystem implements Kernel#system(cmd, ...). Spawns cmd via
// the shell when given a single String, or as argv when given multiple
// args. Returns true on exit-0, false on non-zero exit, nil when the
// command couldn't be started (executable missing, fork failure).
// stdout/stderr are inherited so output appears in the caller's
// streams.
func kernelSystem(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	if len(args) == 0 {
		return nil, errorf("evaluator: system: missing command")
	}
	parts := make([]string, 0, len(args))
	for _, a := range args {
		// Skip env-Hash (leading) and opts-Hash (trailing) -- MRI
		// shapes that goruby doesn't propagate through exec.
		if _, isHash := a.(*object.Hash); isHash {
			continue
		}
		s, ok := stringText(env, a)
		if !ok {
			return nil, errorf("evaluator: system: arg must be String, got %T", a)
		}
		parts = append(parts, s)
	}
	if len(parts) == 0 {
		return object.NIL, nil
	}
	var cmd *exec.Cmd
	if len(parts) == 1 {
		// Single-arg form goes through /bin/sh so shell metacharacters
		// (pipes, redirection, env-var interpolation) work. Matches
		// MRI's "command line" form.
		cmd = exec.Command("/bin/sh", "-c", parts[0])
	} else {
		cmd = exec.Command(parts[0], parts[1:]...)
	}
	cmd.Stdout = env.Stdout()
	cmd.Stderr = env.Stderr()
	cmd.Stdin = nil
	if err := cmd.Run(); err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return object.FALSE, nil
		}
		// Couldn't start at all -- MRI returns nil.
		return object.NIL, nil
	}
	return object.TRUE, nil
}

// kernelBacktick implements Kernel#` (the backtick operator): runs
// the command via the shell and returns its stdout as a String.
// Stderr is inherited. Returns "" when the command couldn't be
// started.
func kernelBacktick(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	if len(args) != 1 {
		return nil, errorf("evaluator: backtick: expected 1 arg, got %d", len(args))
	}
	s, ok := stringText(env, args[0])
	if !ok {
		return nil, errorf("evaluator: backtick: arg must be String, got %T", args[0])
	}
	cmd := exec.Command("/bin/sh", "-c", s)
	cmd.Stderr = env.Stderr()
	out, err := cmd.Output()
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return object.NewString(string(out)), nil
		}
		return object.NewString(""), nil
	}
	return object.NewString(string(out)), nil
}

// kernelAbort implements Kernel#abort: optional String arg goes to
// stderr, then raises exitSignal with code 1. Used by mariolang-rb
// etc. when an interpreter wants to bail out on a runtime error.
func kernelAbort(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	if len(args) >= 1 {
		if s, ok := stringText(env, args[0]); ok {
			// Mirror MRI: stderr write + newline.
			env.Stderr().Write([]byte(s))
			env.Stderr().Write([]byte{'\n'})
		}
	}
	return nil, &exitSignal{Code: 1}
}

// kernelRand implements Kernel#rand. Forms:
//
//	rand       -> Float in [0, 1)
//	rand(n)    -> Integer in [0, n) when n is Integer, or Float when n is Float
//	rand(a..b) -> Integer in [a, b]
//
// Routes through math/rand for the PRNG. Not seeded against MRI -- if
// fixture parity matters under a specific seed, Kernel#srand can be
// invoked first to align.
func kernelRand(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	if len(args) == 0 {
		return object.NewFloat(randFloat()), nil
	}
	switch a := args[0].(type) {
	case *object.Integer:
		if a.Value <= 0 {
			return object.NewFloat(randFloat()), nil
		}
		return object.NewInteger(int64(randomIntn(int(a.Value)))), nil
	case *object.Float:
		if a.Value <= 0 {
			return object.NewFloat(randFloat()), nil
		}
		return object.NewFloat(randFloat() * a.Value), nil
	case *object.Range:
		b, bOK := a.Begin.(*object.Integer)
		e, eOK := a.End.(*object.Integer)
		if !bOK || !eOK {
			return nil, errorf("evaluator: rand(Range): only Integer endpoints supported")
		}
		lo := b.Value
		hi := e.Value
		if a.Exclusive {
			hi--
		}
		if hi < lo {
			return nil, errorf("evaluator: rand(Range): empty range")
		}
		span := hi - lo + 1
		return object.NewInteger(lo + int64(randomIntn(int(span)))), nil
	}
	return nil, errorf("evaluator: rand: unsupported argument type %T", args[0])
}

// kernelSrand seeds the global PRNG used by Kernel#rand and Array#sample.
// Returns the previous seed (we don't track it; report 0).
func kernelSrand(_ *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	if len(args) >= 1 {
		if s, ok := args[0].(*object.Integer); ok {
			randSeed(s.Value)
			return object.NewInteger(0), nil
		}
	}
	randSeed(0)
	return object.NewInteger(0), nil
}

// kernelLoad implements Kernel#load(path[, wrap]) -- evaluates the
// file unconditionally (ignoring require's once-only memo). The wrap
// flag is accepted but not honoured -- goruby has no anonymous-module
// scope wrapping; tests that rely on it get globally-visible defs,
// which matches the rake clean / linked_list test expectations.
func kernelLoad(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	if len(args) < 1 || len(args) > 2 {
		return nil, errorf("evaluator: load: wrong number of arguments (given %d, expected 1..2)", len(args))
	}
	name, ok := stringText(env, args[0])
	if !ok {
		return nil, errorf("evaluator: load: expected String, got %T", args[0])
	}
	// Resolve: absolute path wins; otherwise $LOAD_PATH walk, then
	// require_relative-style fallback from the current source file's
	// directory. Unlike require, load always re-evaluates (matches
	// MRI's load-vs-require distinction).
	if filepath.IsAbs(name) {
		return loadAndForce(env, name, name)
	}
	if abs, ok := resolveViaLoadPath(env, name); ok {
		return loadAndForce(env, abs, name)
	}
	if env.CurrentFile() != "" {
		// Resolve relative to the current file dir but force re-eval.
		base := filepath.Dir(env.CurrentFile())
		target := name
		if filepath.Ext(target) == "" {
			target += ".rb"
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(base, target)
		}
		abs, _ := filepath.Abs(target)
		return loadAndForce(env, abs, name)
	}
	return nil, errorf("evaluator: LoadError: cannot load such file -- %s", name)
}

// loadAndForce parses + evaluates abs even if it's already been
// loaded (matches Kernel#load semantics, distinct from require).
func loadAndForce(env *object.Environment, abs, displayName string) (object.RubyObject, error) {
	src, err := os.ReadFile(abs)
	if err != nil {
		return raiseBuiltin(env, "LoadError", "cannot load such file -- "+displayName)
	}
	prog, err := parser.ParseFile(abs, src, 0, parser.WithVersion(env.Version()))
	if err != nil {
		return nil, errorf("evaluator: load: parse %s: %s", abs, err.Error())
	}
	if _, err := Eval(prog, env); err != nil {
		return nil, err
	}
	postLoadFixup(env, abs)
	return object.TRUE, nil
}

// kernelRequire implements Kernel#require(name): the stdlib loader.
// We don't ship a ruby stdlib, so the call silently returns true for
// names we know to be stdlib (so the caller's `require 'json'` etc.
// don't blow up at load time). Anything else still falls through to
// the relative-style resolve so users can call require with paths.
//
// Programs that try to USE classes that would have been loaded
// (`Prime`, `Date`, ...) will still fail later with NameError when
// the constant is referenced -- that's the explicit signal that
// this stub isn't enough.
func kernelRequire(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	if len(args) != 1 {
		return nil, errorf("evaluator: require: wrong number of arguments (given %d, expected 1)", len(args))
	}
	name, ok := stringText(env, args[0])
	if !ok {
		return nil, errorf("evaluator: require: expected String, got %T", args[0])
	}
	// Known stdlib feature names get a no-op stub so the eval continues.
	// The list isn't exhaustive -- extend as the corpus uncovers more.
	//
	// Two groups: "loaded" (we ship some runtime support, so the require
	// returns true and the constants it would have defined are
	// available) and "absent" (we ship no runtime; the require raises
	// LoadError so callers' `begin require X; rescue LoadError; ...`
	// fallback paths engage). Names callers do `require X` unconditionally
	// belong in "loaded" -- otherwise their require would crash the load.
	switch name {
	case "prime", "date", "set", "stringio", "json", "optparse", "fileutils",
		"tempfile", "pathname", "uri", "cgi", "csv", "yaml", "open3",
		"shellwords", "time", "bigdecimal", "rational", "complex",
		"matrix", "ostruct", "delegate", "forwardable", "singleton",
		"observer", "logger", "benchmark", "digest", "digest/md5",
		"digest/sha1", "digest/sha256", "base64", "zlib", "socket",
		"net/http", "open-uri", "io/console", "strscan",
		"rbconfig", "monitor", "thread", "mutex_m", "weakref",
		"tmpdir", "etc":
		return object.TRUE, nil
	case "test/unit":
		// test/unit aliased to minitest as a shim. rake's own test
		// suite uses Test::Unit::TestCase which we treat as the
		// minitest Test class.
		// Wire Test::Unit::TestCase pointing at Minitest::Test if
		// available; otherwise fall back to plain require to load
		// minitest's runtime under the Test::Unit namespace.
		if mini, ok := env.Get("Minitest"); ok {
			if miniMod, ok := mini.(*object.Class); ok {
				if testCls, ok := miniMod.Constants["Test"].(*object.Class); ok {
					testUnit := object.NewClass("Test", nil)
					testUnit.IsModule = true
					testUnit.Constants = map[string]object.RubyObject{
						"Unit": func() *object.Class {
							u := object.NewClass("Unit", nil)
							u.IsModule = true
							u.Constants = map[string]object.RubyObject{
								"TestCase": testCls,
							}
							return u
						}(),
					}
					env.SetGlobal("Test", testUnit)
					return object.TRUE, nil
				}
			}
		}
		return object.TRUE, nil
	case "win32ole", "win32/registry":
		// "absent" group -- callers wrap in begin/rescue LoadError to
		// pick a fallback. Returning LoadError makes the fallback path
		// engage cleanly instead of letting the caller introspect a
		// constant we never defined.
		return raiseBuiltin(env, "LoadError", "cannot load such file -- "+name)
	}
	// Non-stdlib name: first try the $LOAD_PATH search (MRI semantics --
	// iterate the array, try `<entry>/<name>.rb` for each, take the
	// first that exists). Drivers that vendor a gem add the gem's lib/
	// dir to $LOAD_PATH so `require "gem/sub"` resolves from the array
	// regardless of which file inside the gem is doing the require.
	if abs, ok := resolveViaLoadPath(env, name); ok {
		return loadRubyFile(env, abs, name)
	}
	// Fall back to require_relative-style resolution from the current
	// source file's directory. Useful for the simpler shape where a
	// gem's own lib/<gem>.rb does `require 'gem/sub'` and the requirer
	// is the gem root (so the relative path lands).
	if env.CurrentFile() != "" {
		return kernelRequireRelative(env, args)
	}
	return nil, errorf("evaluator: LoadError: cannot load such file -- %s", name)
}

// resolveViaLoadPath iterates $LOAD_PATH entries and returns the
// absolute path of the first matching `<entry>/<name>.rb` (or
// `<entry>/<name>` if name already ends in .rb). Resolves $LOAD_PATH
// entries against env.CurrentFile()'s directory when relative -- MRI
// keeps the array verbatim, but goruby drivers typically push relative
// paths and rely on the working dir, so anchoring to the entry script
// makes the behaviour predictable across invocations.
func resolveViaLoadPath(env *object.Environment, name string) (string, bool) {
	lp, ok := env.Get("$LOAD_PATH")
	if !ok {
		return "", false
	}
	arr, ok := lp.(*object.Array)
	if !ok {
		return "", false
	}
	target := name
	if filepath.Ext(target) == "" {
		target += ".rb"
	}
	anchor := ""
	if cur := env.CurrentFile(); cur != "" {
		anchor = filepath.Dir(cur)
	}
	for _, e := range arr.Elements {
		dir, ok := stringText(env, e)
		if !ok {
			continue
		}
		candidate := filepath.Join(dir, target)
		if !filepath.IsAbs(candidate) && anchor != "" {
			candidate = filepath.Join(anchor, candidate)
		}
		abs, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		if _, err := os.Stat(abs); err == nil {
			return abs, true
		}
	}
	return "", false
}

// loadRubyFile parses and evaluates the file at abs in env's root,
// honouring the $LOADED_FEATURES cache. displayName is the original
// require argument, used only in error messages. Returns TRUE on first
// load, FALSE if previously loaded.
func loadRubyFile(env *object.Environment, abs, displayName string) (object.RubyObject, error) {
	if env.IsLoaded(abs) {
		return object.FALSE, nil
	}
	src, err := os.ReadFile(abs)
	if err != nil {
		return raiseBuiltin(env, "LoadError", "cannot load such file -- "+displayName)
	}
	env.MarkLoaded(abs)
	prog, err := parser.ParseFile(abs, src, 0, parser.WithVersion(env.Version()))
	if err != nil {
		return nil, errorf("evaluator: require: parse %s: %s", abs, err.Error())
	}
	if _, err := Eval(prog, env); err != nil {
		return nil, err
	}
	postLoadFixup(env, abs)
	return object.TRUE, nil
}

// postLoadFixup applies inline patches after a known gem source file
// finishes loading. Used to layer compat shims for vendored gems we
// can't modify directly (the gems dir is gitignored, auto-fetched).
func postLoadFixup(env *object.Environment, abs string) {
	if strings.HasSuffix(abs, "/rake/task.rb") {
		// Rake::Task#first_sentence uses a lookbehind regex
		// `/(?<=\w)(\.|!)[ \t]|(\.$|!)|\n/`. Go's regexp doesn't
		// support lookbehind. The strip-lookaround rewrite is too
		// permissive for "test...I think" style inputs (splits at the
		// first dot it sees instead of dot-after-\w). Replace with a
		// hand-rolled scan that approximates the MRI behaviour:
		// stop at "[!\.]" preceded by \w when followed by space/tab/EOL/EOS,
		// or at "\n" -- and elide the trailing dot/bang from the result.
		_, _ = evalString(env, `
class Rake::Task
  private
  def first_sentence(string)
    n = string.length
    i = 0
    while i < n
      ch = string[i]
      if ch == "\n"
        return string[0, i]
      elsif ch == "." || ch == "!"
        prev = i > 0 ? string[i - 1] : ""
        nxt = i + 1 < n ? string[i + 1] : ""
        if prev =~ /\w/
          if nxt == " " || nxt == "\t" || nxt == "\n" || nxt == ""
            return string[0, i]
          end
        end
      end
      i += 1
    end
    string
  end
end
`)
	}
	if strings.HasSuffix(abs, "/minitest/assertions.rb") {
		// rake's test suite calls Minitest::Assertions#capture_output;
		// minitest 5.x in the vendored copy doesn't ship it. Alias to
		// the equivalent capture_io.
		_, _ = evalString(env, `
module Minitest
  module Assertions
    alias capture_output capture_io unless method_defined?(:capture_output)
  end
end
`)
	}
}

// evalString parses + evals a small string in env.
func evalString(env *object.Environment, src string) (object.RubyObject, error) {
	prog, err := parser.ParseFile("(post-load)", []byte(src), 0, parser.WithVersion(env.Version()))
	if err != nil {
		return nil, err
	}
	return Eval(prog, env)
}

// kernelRequireRelative implements Kernel#require_relative(path): resolves
// path against the directory of the currently-executing source file,
// appending ".rb" if absent, parses and evaluates that file in the
// current root environment. Returns true on first load, false if
// already loaded (matches MRI's $LOADED_FEATURES semantics). Loaded
// classes / methods / constants persist in the root env so the caller
// sees them after the require returns.
func kernelRequireRelative(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	if len(args) != 1 {
		return nil, errorf("evaluator: require_relative: wrong number of arguments (given %d, expected 1)", len(args))
	}
	rel, ok := stringText(env, args[0])
	if !ok {
		return nil, errorf("evaluator: require_relative: expected String, got %T", args[0])
	}
	cur := env.CurrentFile()
	if cur == "" {
		return raiseBuiltin(env, "LoadError", "cannot infer basepath for require_relative")
	}
	base := filepath.Dir(cur)
	target := rel
	if !filepath.IsAbs(target) {
		target = filepath.Join(base, target)
	}
	if filepath.Ext(target) == "" {
		target += ".rb"
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return raiseBuiltin(env, "LoadError", "cannot resolve "+target+": "+err.Error())
	}
	if env.IsLoaded(abs) {
		return object.FALSE, nil
	}
	src, err := os.ReadFile(abs)
	if err != nil {
		return raiseBuiltin(env, "LoadError", "cannot load such file -- "+rel)
	}
	env.MarkLoaded(abs)
	prog, err := parser.ParseFile(abs, src, 0, parser.WithVersion(env.Version()))
	if err != nil {
		return nil, errorf("evaluator: require_relative: parse %s: %s", abs, err.Error())
	}
	if _, err := Eval(prog, env); err != nil {
		return nil, err
	}
	return object.TRUE, nil
}

// kernelEval implements Kernel#eval(str): parses str under the
// current ruby version and evaluates it in env. Useful for the host
// of esolang interpreters that build ruby source dynamically.
func kernelEval(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	if len(args) < 1 {
		return nil, errorf("evaluator: eval: wrong number of arguments (given %d, expected 1..)", len(args))
	}
	src, ok := stringText(env, args[0])
	if !ok {
		return nil, errorf("evaluator: eval: expected String, got %T", args[0])
	}
	prog, err := parser.ParseFile("(eval)", []byte(src), 0, parser.WithVersion(env.Version()))
	if err != nil {
		return nil, errorf("evaluator: eval: parse: %s", err.Error())
	}
	return Eval(prog, env)
}

// kernelLoop implements Kernel#loop { ... } -- runs the block forever
// until `break` short-circuits it. Returns the break value (or nil).
func kernelLoop(env *object.Environment, blk *ast.BlockExpression) (object.RubyObject, error) {
	for {
		v, stop, err := iterStep(func(a []object.RubyObject) (object.RubyObject, error) {
			return invokeBlock(env, blk, a)
		}, nil)
		if err != nil {
			return nil, err
		}
		if stop {
			return v, nil
		}
	}
}

// kernelWarn implements Kernel#warn: writes to $stderr (or env.Stderr
// when unset) with trailing newline, like puts but routed to the
// error stream. Honors $stderr reassignment (capture_io's stderr-half).
func kernelWarn(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	// Honor $stderr if reassigned to an object that responds to puts.
	if v, ok := env.Get("$stderr"); ok && v != nil {
		if inst, ok := v.(*object.Instance); ok {
			if _, found := inst.C.LookupMethod("puts"); found {
				if _, err := callMethod(env, inst, "puts", args); err != nil {
					return nil, err
				}
				return object.NIL, nil
			}
		}
	}
	w := env.Stderr()
	for _, a := range args {
		s := putsString(env, a)
		_, _ = w.Write([]byte(s))
		if len(s) == 0 || s[len(s)-1] != '\n' {
			_, _ = w.Write([]byte{'\n'})
		}
	}
	return object.NIL, nil
}

// kernelPuts implements Kernel#puts: writes each argument followed by a
// newline (unless the value already ends in one). With zero args, writes
// a single newline. Arrays are unrolled element-by-element.
func kernelPuts(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	// Honor $stdout if reassigned to an object that responds to puts
	// (minitest's capture_io path; common test-setup idiom).
	if v, ok := env.Get("$stdout"); ok && v != nil {
		if inst, ok := v.(*object.Instance); ok {
			if _, found := inst.C.LookupMethod("puts"); found {
				if _, err := callMethod(env, inst, "puts", args); err != nil {
					return nil, err
				}
				return object.NIL, nil
			}
		}
	}
	w := env.Stdout()
	if len(args) == 0 {
		_, _ = w.Write([]byte{'\n'})
		return object.NIL, nil
	}
	for _, a := range args {
		if arr, ok := a.(*object.Array); ok {
			if _, err := kernelPuts(env, arr.Elements); err != nil {
				return nil, err
			}
			continue
		}
		s := putsString(env, a)
		_, _ = w.Write([]byte(s))
		if len(s) == 0 || s[len(s)-1] != '\n' {
			_, _ = w.Write([]byte{'\n'})
		}
	}
	return object.NIL, nil
}

// kernelInteger implements Kernel#Integer(v, base=10) for the common
// String / Integer / Float inputs.
func kernelInteger(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	if len(args) < 1 || len(args) > 2 {
		return nil, errorf("evaluator: Integer: wrong number of arguments (%d)", len(args))
	}
	base := 10
	if len(args) == 2 {
		bi, ok := args[1].(*object.Integer)
		if !ok {
			return nil, errorf("evaluator: Integer: base must be Integer")
		}
		base = int(bi.Value)
	}
	switch v := args[0].(type) {
	case *object.Integer:
		return v, nil
	case *object.Float:
		return object.NewInteger(int64(v.Value)), nil
	case *object.String:
		s := strings.TrimSpace(string(v.Buf))
		s = strings.ReplaceAll(s, "_", "")
		// Accept the standard ruby 0x/0o/0b/0d prefixes when base is
		// auto-detected (i.e. caller didn't specify and string carries
		// one).
		if len(args) == 1 {
			if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
				s = s[2:]
				base = 16
			} else if strings.HasPrefix(s, "0b") || strings.HasPrefix(s, "0B") {
				s = s[2:]
				base = 2
			} else if strings.HasPrefix(s, "0o") || strings.HasPrefix(s, "0O") {
				s = s[2:]
				base = 8
			}
		}
		n, err := strconv.ParseInt(s, base, 64)
		if err != nil {
			return raiseBuiltin(env, "ArgumentError", "invalid value for Integer(): \""+string(v.Buf)+"\"")
		}
		return object.NewInteger(n), nil
	}
	return nil, errorf("evaluator: Integer: can't convert %T", args[0])
}

// kernelFloat implements Kernel#Float(v).
func kernelFloat(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	if len(args) != 1 {
		return nil, errorf("evaluator: Float: wrong number of arguments (%d)", len(args))
	}
	switch v := args[0].(type) {
	case *object.Float:
		return v, nil
	case *object.Integer:
		return object.NewFloat(float64(v.Value)), nil
	case *object.String:
		s := strings.TrimSpace(string(v.Buf))
		s = strings.ReplaceAll(s, "_", "")
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return raiseBuiltin(env, "ArgumentError", "invalid value for Float(): \""+string(v.Buf)+"\"")
		}
		return object.NewFloat(f), nil
	}
	return nil, errorf("evaluator: Float: can't convert %T", args[0])
}

// kernelString implements Kernel#String(v) -- routes through to_s.
func kernelString(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	if len(args) != 1 {
		return nil, errorf("evaluator: String: wrong number of arguments (%d)", len(args))
	}
	return object.NewString(putsString(env, args[0])), nil
}

// kernelArray implements Kernel#Array(v) -- v if it's already an Array,
// [v] otherwise (nil becomes []).
func kernelArray(args []object.RubyObject) (object.RubyObject, error) {
	if len(args) != 1 {
		return nil, errorf("evaluator: Array: wrong number of arguments (%d)", len(args))
	}
	switch v := args[0].(type) {
	case *object.Array:
		return v, nil
	case *object.Nil:
		return object.NewArray(), nil
	}
	return object.NewArray(args[0]), nil
}

// kernelSprintf implements Kernel#sprintf / format: first arg is the
// format string, rest are the values to interpolate.
func kernelSprintf(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	if len(args) == 0 {
		return nil, errorf("evaluator: ArgumentError: sprintf needs a format string")
	}
	fmtStr, ok := stringText(env, args[0])
	if !ok {
		return nil, errorf("evaluator: sprintf: format must be String")
	}
	s, err := sprintfFormat(env, fmtStr, args[1:])
	if err != nil {
		return nil, err
	}
	return object.NewString(s), nil
}

// kernelPrintf implements Kernel#printf: same as sprintf but writes
// to stdout instead of returning a String.
func kernelPrintf(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	v, err := kernelSprintf(env, args)
	if err != nil {
		return nil, err
	}
	s := v.(*object.String)
	if writeViaStdoutRedirect(env, s) {
		return object.NIL, nil
	}
	_, _ = env.Stdout().Write(s.Buf)
	return object.NIL, nil
}

// writeViaStdoutRedirect checks if $stdout has been reassigned to an
// Instance responding to #write (minitest's capture_io path) and routes
// the write through it. Returns true if the redirect was honoured.
func writeViaStdoutRedirect(env *object.Environment, s *object.String) bool {
	v, ok := env.Get("$stdout")
	if !ok || v == nil {
		return false
	}
	inst, ok := v.(*object.Instance)
	if !ok {
		return false
	}
	if _, found := inst.C.LookupMethod("write"); !found {
		return false
	}
	_, _ = callMethod(env, inst, "write", []object.RubyObject{s})
	return true
}

// kernelPrint implements Kernel#print: writes each argument's to_s
// rendering with no trailing newline and no separator.
func kernelPrint(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	var sb strings.Builder
	for _, a := range args {
		sb.WriteString(putsString(env, a))
	}
	out := object.NewString(sb.String())
	if writeViaStdoutRedirect(env, out) {
		return object.NIL, nil
	}
	_, _ = env.Stdout().Write(out.Buf)
	return object.NIL, nil
}

// kernelP implements Kernel#p: writes each argument's inspect form
// followed by a newline. Returns the single arg, or the args slice
// boxed as an Array for multiple args.
func kernelP(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	w := env.Stdout()
	for _, a := range args {
		_, _ = w.Write([]byte(env.Inspect(a)))
		_, _ = w.Write([]byte{'\n'})
	}
	switch len(args) {
	case 0:
		return object.NIL, nil
	case 1:
		return args[0], nil
	}
	return object.NewArray(args...), nil
}

// putsString returns the Kernel#puts rendering of a value. Differs from
// inspect in two places: nil renders as the empty string (so `puts nil`
// produces just "\n"), and strings render bare (no surrounding quotes).
// For Instances with a user-defined `to_s`, dispatches to it.
func putsString(env *object.Environment, o object.RubyObject) string {
	switch v := o.(type) {
	case *object.Nil:
		return ""
	case *object.String:
		return string(v.Buf)
	case *object.FrozenString:
		return env.Strings().Get(v.ID)
	case *object.Symbol:
		return env.Symbols().Name(v.ID)
	case *object.Instance:
		if m, found := dispatchClass(env, v).LookupMethod("to_s"); found {
			res, err := m.Call(env, v, nil, nil)
			if err == nil {
				if s, ok := stringText(env, res); ok {
					return s
				}
			}
		}
	}
	return env.Inspect(o)
}
