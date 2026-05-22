package evaluator

import (
	"os"
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
	case "p", "pp":
		return kernelP(env, args)
	case "print":
		return kernelPrint(env, args)
	case "printf":
		return kernelPrintf(env, args)
	case "sprintf", "format":
		return kernelSprintf(env, args)
	case "raise":
		return kernelRaise(env, args)
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
	case "rand":
		return kernelRand(env, args)
	case "srand":
		return kernelSrand(env, args)
	case "lambda":
		return nil, errorf("evaluator: Kernel#lambda without block not supported; use ->( ){ ... }")
	case "Complex":
		return stdlib.KernelComplex(env, args)
	}
	return nil, errorf("evaluator: NoMethodError: undefined method `%s' for main:Object", name)
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
func kernelExit(_ *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
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
	return nil, &exitSignal{Code: code}
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
	switch name {
	case "prime", "date", "set", "stringio", "json", "optparse", "fileutils",
		"tempfile", "pathname", "uri", "cgi", "csv", "yaml", "open3",
		"shellwords", "time", "bigdecimal", "rational", "complex",
		"matrix", "ostruct", "delegate", "forwardable", "singleton",
		"observer", "logger", "benchmark", "digest", "digest/md5",
		"digest/sha1", "digest/sha256", "base64", "zlib", "socket",
		"net/http", "open-uri", "io/console", "etc", "strscan":
		return object.TRUE, nil
	}
	// Non-stdlib name: fall back to require_relative-style resolution
	// from the current source file's directory. mri searches $LOAD_PATH,
	// which we don't model; this fallback covers the common shape where
	// a lib/<gem>.rb does `require 'gem/sub'` and the gem's lib dir was
	// added to $LOAD_PATH so the relative path finds the sibling file.
	if env.CurrentFile() != "" {
		return kernelRequireRelative(env, args)
	}
	return nil, errorf("evaluator: LoadError: cannot load such file -- %s", name)
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

// kernelPuts implements Kernel#puts: writes each argument followed by a
// newline (unless the value already ends in one). With zero args, writes
// a single newline. Arrays are unrolled element-by-element.
func kernelPuts(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
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
	_, _ = env.Stdout().Write(s.Buf)
	return object.NIL, nil
}

// kernelPrint implements Kernel#print: writes each argument's to_s
// rendering with no trailing newline and no separator.
func kernelPrint(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	w := env.Stdout()
	for _, a := range args {
		_, _ = w.Write([]byte(putsString(env, a)))
	}
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
