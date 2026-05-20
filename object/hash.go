package object

import (
	"strings"

	"github.com/lczyk/goruby/token"
)

// HashEntry is one key-value pair in a Hash. Ordered insertion as MRI
// preserves insertion order since 1.9.
type HashEntry struct {
	Key, Value RubyObject
}

// Hash represents a ruby Hash. Linear-scan []HashEntry layout: dense,
// preserves insertion order, fast for small N (the typical case for
// option hashes and small lookups). Once benchmarks show large hashes
// dominating, swap in a hybrid (linear small, map fallback large).
type Hash struct {
	Entries []HashEntry
	// Default is the static default value returned by `h[missing]`
	// when no default block is set; nil means "fall back to NIL".
	Default RubyObject
	// DefaultBlock is invoked as `block.call(h, missing_key)` when set
	// and the key is missing. Stored as `any` so the object package
	// stays free of the evaluator's Proc type details (it's actually
	// a *Proc).
	DefaultBlock any
}

// NewHash returns a Hash containing the given entries (in order).
func NewHash(entries ...HashEntry) *Hash {
	out := make([]HashEntry, len(entries))
	copy(out, entries)
	return &Hash{Entries: out}
}

func (h *Hash) Type() Type       { return HASH_OBJ }
func (h *Hash) Class() RubyClass { return nil }

// Inspect formats the hash under latest-version conventions. For
// version-aware output, callers should use the free Inspect(obj, v) or
// env.Inspect(obj).
func (h *Hash) Inspect() string {
	return h.inspectAt(inspectCtx{v: token.LatestVersion()})
}

// inspectAt implements the version-aware formatting. Ruby 3.4 changed
// the default inspect output:
//   - `=>` separator gains surrounding spaces: `1=>2` -> `1 => 2`.
//   - Symbol keys use shorthand: `:a=>1` -> `a: 1`.
//
// Older targets keep the compact `1=>2`, `:a=>1` form.
var ruby34 = token.MustParseVersion("3.4")

func (h *Hash) inspectAt(ctx inspectCtx) string {
	var b strings.Builder
	b.WriteByte('{')
	modern := ctx.v.AtLeast(ruby34)
	for i, e := range h.Entries {
		if i > 0 {
			b.WriteString(", ")
		}
		if modern {
			if sym, ok := e.Key.(*Symbol); ok {
				b.WriteString(symName(ctx, sym))
				b.WriteString(": ")
				b.WriteString(inspectAt(ctx, e.Value))
				continue
			}
			b.WriteString(inspectAt(ctx, e.Key))
			b.WriteString(" => ")
			b.WriteString(inspectAt(ctx, e.Value))
			continue
		}
		b.WriteString(inspectAt(ctx, e.Key))
		b.WriteString("=>")
		b.WriteString(inspectAt(ctx, e.Value))
	}
	b.WriteByte('}')
	return b.String()
}
