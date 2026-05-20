package object

import (
	"io"
	"os"

	"github.com/lczyk/goruby/token"
)

// Environment holds the runtime state available during evaluation:
// variable bindings, the symbol/string pools, the active ruby version,
// and stdout for Kernel#puts / Kernel#p.
//
// Environments chain via Outer to model block/method scopes. Pools and
// stdout live on the root and are reached through the chain.
type Environment struct {
	store map[string]RubyObject
	outer *Environment

	// Root-only fields. Non-root environments delegate to Outer.
	syms    *SymbolPool
	strings *StringPool
	stdout  io.Writer
	version token.RubyVersion
	methods map[string]RubyObject

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
// (os.Stdout), fresh pools, and a latest-version target.
func NewMainEnvironment(opts ...EnvOption) *Environment {
	e := &Environment{
		store:   make(map[string]RubyObject),
		syms:    NewSymbolPool(),
		strings: NewStringPool(),
		stdout:  os.Stdout,
		methods: make(map[string]RubyObject),
	}
	for _, o := range opts {
		o(e)
	}
	return e
}

// NewEnclosedEnvironment returns a child environment whose lookups fall
// through to outer if not found locally.
func NewEnclosedEnvironment(outer *Environment) *Environment {
	return &Environment{
		store: make(map[string]RubyObject),
		outer: outer,
	}
}

// EnvOption configures a root Environment at construction time.
type EnvOption func(*Environment)

// WithVersion sets the ruby version the evaluator targets when run under
// this environment. Zero value means "latest".
func WithVersion(v token.RubyVersion) EnvOption {
	return func(e *Environment) { e.version = v }
}

// WithStdout overrides the writer Kernel#puts / Kernel#p send output to.
// Useful in tests for capturing output.
func WithStdout(w io.Writer) EnvOption {
	return func(e *Environment) { e.stdout = w }
}

// Get returns the binding for name, walking up the outer chain.
func (e *Environment) Get(name string) (RubyObject, bool) {
	if v, ok := e.store[name]; ok {
		return v, true
	}
	if e.outer != nil {
		return e.outer.Get(name)
	}
	return nil, false
}

// Set binds name to value in the local scope.
func (e *Environment) Set(name string, value RubyObject) RubyObject {
	e.store[name] = value
	return value
}

// SetGlobal binds name to value on the root environment regardless of
// the current scope. Used for ruby globals ($foo) and other constructs
// that must outlive any block / method scope.
func (e *Environment) SetGlobal(name string, value RubyObject) RubyObject {
	e.root().store[name] = value
	return value
}

// AssignVisible binds name in whichever enclosing scope already holds
// it; if none does, binds locally. Mirrors ruby block-scoping where a
// block writes through to an outer local var that's already defined.
// The walk stops at the nearest method frame so a block can't reach
// past its enclosing method.
func (e *Environment) AssignVisible(name string, value RubyObject) RubyObject {
	for cur := e; cur != nil; cur = cur.outer {
		if _, ok := cur.store[name]; ok {
			cur.store[name] = value
			return value
		}
		if cur.MethodFrame {
			break
		}
	}
	e.store[name] = value
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
