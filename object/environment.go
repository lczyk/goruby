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
}

// NewMainEnvironment returns a fresh root environment with default stdout
// (os.Stdout), fresh pools, and a latest-version target.
func NewMainEnvironment(opts ...EnvOption) *Environment {
	e := &Environment{
		store:   make(map[string]RubyObject),
		syms:    NewSymbolPool(),
		strings: NewStringPool(),
		stdout:  os.Stdout,
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
