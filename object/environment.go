package object

import (
	"io"
	"os"

	"github.com/lczyk/goruby/token"
)

// envInlineCap is the number of bindings each Environment holds
// inline before promoting to a map. Chosen to cover typical method
// frames (1-4 params + block-locals) without overshooting; bigger
// frames pay the map cost only when they actually need it.
const envInlineCap = 6

// envEntry pairs a binding name with its value for inline storage.
type envEntry struct {
	name  string
	value RubyObject
}

// Environment holds the runtime state available during evaluation:
// variable bindings, the symbol/string pools, the active ruby version,
// and stdout for Kernel#puts / Kernel#p.
//
// Environments chain via Outer to model block/method scopes. Pools and
// stdout live on the root and are reached through the chain.
//
// Local bindings use a small inline array first (linear scan, no map
// alloc) and only promote to a real map once the binding count
// exceeds envInlineCap. Most call frames -- method calls with a
// handful of params, blocks with one or two block-locals -- stay in
// the inline path and avoid per-frame map allocation.
type Environment struct {
	inline  [envInlineCap]envEntry
	inlineN int
	store   map[string]RubyObject // overflow once inline fills up
	outer   *Environment

	// Root-only fields. Non-root environments delegate to Outer.
	syms    *SymbolPool
	strings *StringPool
	stdout  io.Writer
	stderr  io.Writer
	stdin   io.Reader
	stdinBR any // *bufio.Reader cached so successive gets() share buffer state
	argfBR  any // *bufio.Reader cached so successive ARGF.gets advance through same stream
	version token.RubyVersion
	methods map[string]RubyObject

	// currentFile is the absolute path of the source file presently
	// being evaluated. Updated on entry/exit by evalProgram (top-level)
	// and by Kernel#require_relative (nested loads). dirname(currentFile)
	// is the base for resolving relative loads. Empty for hand-built ASTs
	// and ad-hoc Eval calls without a backing file.
	currentFile string
	// loadedFiles records absolute paths already loaded by
	// require_relative / require, so re-requiring is a no-op (matches
	// MRI's $LOADED_FEATURES semantics, scoped to this interpreter).
	loadedFiles map[string]bool

	// MethodFrame marks an environment that backs a method call. Block
	// scoping stops walking outward at the nearest method frame so a
	// block can't reach into the surrounding caller's locals through a
	// method body in between.
	MethodFrame bool

	// CurrentBlock is the block (if any) passed to the method that
	// owns this frame; `yield` resolves to it. Stored as `any` so the
	// object package needn't depend on the evaluator's block type.
	CurrentBlock any

	// Self is the ruby `self` value visible in this frame. nil means
	// "inherit from outer"; the root frame leaves it unset and the
	// evaluator falls back to the main object.
	Self RubyObject

	// CurrentClass is the open class body being executed (set inside
	// `class Foo ... end`). nil at top level.
	CurrentClass *Class

	// CurrentMethodName / CurrentMethodArgs are set on a method's call
	// frame so `super` can find the next-up implementation and (for
	// implicit-args super) reuse the original argument list.
	CurrentMethodName string
	CurrentMethodArgs []RubyObject

	// CurrentKwargs holds keyword args supplied by the caller, made
	// available to bindParams when the callee declares IsKeyword
	// parameters. Stashed on the call frame because plumbing kwargs
	// through every dispatcher signature would balloon the API.
	CurrentKwargs map[string]RubyObject
}

// NewMainEnvironment returns a fresh root environment with default stdout
// (os.Stdout), fresh pools, and a latest-version target. The root is
// expected to accumulate many bindings (constants, top-level
// methods), so its store map is allocated up front to skip the
// inline-then-promote work the transient frames benefit from.
func NewMainEnvironment(opts ...EnvOption) *Environment {
	e := &Environment{
		store:   make(map[string]RubyObject),
		syms:    NewSymbolPool(),
		strings: NewStringPool(),
		stdout:  os.Stdout,
		stderr:  os.Stderr,
		stdin:   os.Stdin,
		methods: make(map[string]RubyObject),
	}
	for _, o := range opts {
		o(e)
	}
	return e
}

// NewEnclosedEnvironment returns a child environment whose lookups fall
// through to outer if not found locally. The store map stays nil until
// the first Set so block-only scopes that just read enclosing locals
// (the common case in tight iteration) skip the map alloc entirely.
func NewEnclosedEnvironment(outer *Environment) *Environment {
	return &Environment{outer: outer}
}

// EnvOption configures a root Environment at construction time.
type EnvOption func(*Environment)

// WithVersion sets the ruby version the evaluator targets when run under
// this environment. Zero value means "latest".
func WithVersion(v token.RubyVersion) EnvOption {
	return func(e *Environment) { e.version = v }
}

// WithStderr overrides the writer Kernel#abort / STDERR.puts send
// output to. Defaults to os.Stderr.
func WithStderr(w io.Writer) EnvOption {
	return func(e *Environment) { e.stderr = w }
}

// WithStdout overrides the writer Kernel#puts / Kernel#p send output to.
// Useful in tests for capturing output.
func WithStdout(w io.Writer) EnvOption {
	return func(e *Environment) { e.stdout = w }
}

// WithStdin overrides the reader Kernel#gets / STDIN.gets pull from.
// Useful in tests for feeding deterministic input.
func WithStdin(r io.Reader) EnvOption {
	return func(e *Environment) { e.stdin = r }
}

// WithARGV seeds the ARGV constant with the given string arguments.
// Mirrors MRI: command-line args after the script path land here.
func WithARGV(args []string) EnvOption {
	return func(e *Environment) {
		elems := make([]RubyObject, len(args))
		for i, a := range args {
			elems[i] = NewString(a)
		}
		e.setLocal("ARGV", &Array{Elements: elems})
	}
}

// Get returns the binding for name, walking up the outer chain.
func (e *Environment) Get(name string) (RubyObject, bool) {
	if v, ok := e.getLocal(name); ok {
		return v, true
	}
	if e.outer != nil {
		return e.outer.Get(name)
	}
	return nil, false
}

// getLocal looks up name in this env's own bindings (inline + store)
// without consulting outer.
func (e *Environment) getLocal(name string) (RubyObject, bool) {
	for i := 0; i < e.inlineN; i++ {
		if e.inline[i].name == name {
			return e.inline[i].value, true
		}
	}
	if e.store != nil {
		if v, ok := e.store[name]; ok {
			return v, true
		}
	}
	return nil, false
}

// setLocal writes name=value in this env's own bindings. Updates an
// existing entry in place; otherwise appends to the inline array, or
// promotes to the store map once inline fills up.
func (e *Environment) setLocal(name string, value RubyObject) {
	for i := 0; i < e.inlineN; i++ {
		if e.inline[i].name == name {
			e.inline[i].value = value
			return
		}
	}
	if e.store != nil {
		if _, ok := e.store[name]; ok {
			e.store[name] = value
			return
		}
	}
	if e.inlineN < envInlineCap {
		e.inline[e.inlineN] = envEntry{name: name, value: value}
		e.inlineN++
		return
	}
	if e.store == nil {
		e.store = make(map[string]RubyObject)
	}
	e.store[name] = value
}

// Set binds name to value in the local scope.
func (e *Environment) Set(name string, value RubyObject) RubyObject {
	e.setLocal(name, value)
	return value
}

// SetGlobal binds name to value on the root environment regardless of
// the current scope. Used for ruby globals ($foo) and other constructs
// that must outlive any block / method scope.
func (e *Environment) SetGlobal(name string, value RubyObject) RubyObject {
	e.root().setLocal(name, value)
	return value
}

// GetGlobal looks up name only on the root environment, bypassing the
// outer-chain walk. Used by `::Const` resolution: the leading `::`
// forces top-level lookup even when an enclosing module/class shadows
// the name in its own constant table.
func (e *Environment) GetGlobal(name string) (RubyObject, bool) {
	return e.root().getLocal(name)
}

// AssignVisible binds name in whichever enclosing scope already holds
// it; if none does, binds locally. Mirrors ruby block-scoping where a
// block writes through to an outer local var that's already defined.
// The walk stops at the nearest method frame so a block can't reach
// past its enclosing method.
func (e *Environment) AssignVisible(name string, value RubyObject) RubyObject {
	for cur := e; cur != nil; cur = cur.outer {
		if _, ok := cur.getLocal(name); ok {
			cur.setLocal(name, value)
			return value
		}
		if cur.MethodFrame {
			break
		}
	}
	e.setLocal(name, value)
	return value
}

// Version returns the ruby version active in this environment, resolving
// to the root and to the package latest if unset.
func (e *Environment) Version() token.RubyVersion {
	root := e.root()
	if root.version.IsSet() {
		return root.version
	}
	return token.LatestVersion()
}

// Stdout returns the writer Kernel#puts / Kernel#p should target.
func (e *Environment) Stdout() io.Writer { return e.root().stdout }

// Stderr returns the writer Kernel#abort and STDERR.puts target.
func (e *Environment) Stderr() io.Writer {
	r := e.root()
	if r.stderr == nil {
		return os.Stderr
	}
	return r.stderr
}

// Stdin returns the reader Kernel#gets and STDIN methods should pull
// from. Defaults to os.Stdin; tests override via WithStdin.
func (e *Environment) Stdin() io.Reader { return e.root().stdin }

// StdinBR returns the buffered-reader slot on the root env. Used by
// the evaluator to cache a *bufio.Reader across successive `gets` so
// they share line-buffering state. Caller manages the type; env just
// holds the value.
func (e *Environment) StdinBR() any        { return e.root().stdinBR }
func (e *Environment) SetStdinBR(br any)   { e.root().stdinBR = br }
func (e *Environment) ArgfBR() any         { return e.root().argfBR }
func (e *Environment) SetArgfBR(br any)    { e.root().argfBR = br }

// CurrentFile returns the path of the source file currently being
// evaluated. Empty when no file-backed eval is on the stack.
func (e *Environment) CurrentFile() string { return e.root().currentFile }

// SetCurrentFile overwrites the active source-file path on the root
// env, returning the previous value so callers can restore it on
// frame exit (caller-managed stack discipline).
func (e *Environment) SetCurrentFile(path string) string {
	r := e.root()
	prev := r.currentFile
	r.currentFile = path
	return prev
}

// MarkLoaded records absPath in the loaded-files set and returns
// whether it was newly added. Used by require_relative to make
// repeated loads of the same file idempotent.
func (e *Environment) MarkLoaded(absPath string) bool {
	r := e.root()
	if r.loadedFiles == nil {
		r.loadedFiles = make(map[string]bool)
	}
	if r.loadedFiles[absPath] {
		return false
	}
	r.loadedFiles[absPath] = true
	return true
}

// IsLoaded reports whether absPath has already been require_relative'd
// (or require'd) in this interpreter.
func (e *Environment) IsLoaded(absPath string) bool {
	r := e.root()
	if r.loadedFiles == nil {
		return false
	}
	return r.loadedFiles[absPath]
}

// Symbols returns the env's symbol pool.
func (e *Environment) Symbols() *SymbolPool { return e.root().syms }

// Strings returns the env's frozen-string pool.
func (e *Environment) Strings() *StringPool { return e.root().strings }

// SetMethod registers a top-level method on the root environment.
func (e *Environment) SetMethod(name string, m RubyObject) {
	e.root().methods[name] = m
}

// GetMethod returns the top-level method bound to name, if any.
func (e *Environment) GetMethod(name string) (RubyObject, bool) {
	m, ok := e.root().methods[name]
	return m, ok
}

// EnclosingBlock walks outward to the nearest method frame and returns
// that frame's CurrentBlock, if any. Used by `yield` to find the block
// passed to the current method.
func (e *Environment) EnclosingBlock() any {
	for cur := e; cur != nil; cur = cur.outer {
		if cur.MethodFrame {
			return cur.CurrentBlock
		}
	}
	return nil
}

// EnclosingSelf walks outward to the nearest frame with a bound Self
// and returns it. Returns nil if none -- callers use that to mean
// top-level main.
func (e *Environment) EnclosingSelf() RubyObject {
	for cur := e; cur != nil; cur = cur.outer {
		if cur.Self != nil {
			return cur.Self
		}
	}
	return nil
}

// Outer returns the immediately enclosing scope, or nil at the root.
// Exposed so evaluator-side walks (super, etc.) can climb the chain.
func (e *Environment) Outer() *Environment { return e.outer }

// EnclosingClass walks outward to the nearest frame with a CurrentClass
// set, returning it. nil at top level.
func (e *Environment) EnclosingClass() *Class {
	for cur := e; cur != nil; cur = cur.outer {
		if cur.CurrentClass != nil {
			return cur.CurrentClass
		}
	}
	return nil
}

func (e *Environment) root() *Environment {
	for e.outer != nil {
		e = e.outer
	}
	return e
}

// Inspect renders obj per this environment's pools and version. Routes
// through the env-aware path so Symbols and FrozenStrings resolve their
// text, and Hash/Array honour the version-specific formatting rules.
func (e *Environment) Inspect(obj RubyObject) string {
	return inspectWithEnv(obj, e)
}
