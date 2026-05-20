package object

import (
	"strings"

	"github.com/lczyk/goruby/token"
)

// Array represents a ruby Array. Scanned: Elements is a slice of
// interfaces, each carrying a type+data pointer pair.
type Array struct {
	Elements []RubyObject
}

// NewArray returns an Array containing the given elements (in order).
// The caller retains no ownership of the slice -- a fresh backing array
// is allocated.
func NewArray(elements ...RubyObject) *Array {
	out := make([]RubyObject, len(elements))
	copy(out, elements)
	return &Array{Elements: out}
}

func (a *Array) Type() Type       { return ARRAY_OBJ }
func (a *Array) Class() RubyClass { return ArrayClass }

// Inspect formats the array under latest-version conventions. For
// version-aware output, callers should use the free Inspect(obj, v) or
// env.Inspect(obj).
func (a *Array) Inspect() string {
	return a.inspectAt(inspectCtx{v: token.LatestVersion()})
}

func (a *Array) inspectAt(ctx inspectCtx) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, e := range a.Elements {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(inspectAt(ctx, e))
	}
	b.WriteByte(']')
	return b.String()
}
