package object

import "github.com/lczyk/goruby/token"

// inspectCtx threads the version and (optionally) the environment through
// recursive inspect calls. The env carries the symbol / frozen-string
// pools needed to resolve a Symbol's or FrozenString's text. Without an
// env (env == nil) those types fall back to their compact placeholder.
type inspectCtx struct {
	v   token.RubyVersion
	env *Environment
}

// Inspect formats obj per ruby version v's conventions. Hash and Array
// inspect output changed at 3.4 (spaced `=>`, symbol-key shorthand);
// every other type is version-agnostic and falls through to obj.Inspect().
//
// This entrypoint has no env, so Symbol and FrozenString render as
// placeholders. Callers with an env should prefer env.Inspect(obj).
func Inspect(obj RubyObject, v token.RubyVersion) string {
	return inspectAt(inspectCtx{v: v}, obj)
}

func inspectWithEnv(obj RubyObject, env *Environment) string {
	return inspectAt(inspectCtx{v: env.Version(), env: env}, obj)
}

func inspectAt(ctx inspectCtx, obj RubyObject) string {
	switch o := obj.(type) {
	case *Hash:
		return o.inspectAt(ctx)
	case *Array:
		return o.inspectAt(ctx)
	case *Symbol:
		return ":" + symName(ctx, o)
	case *FrozenString:
		if ctx.env != nil {
			s := &String{Buf: []byte(ctx.env.Strings().Get(o.ID))}
			return s.Inspect()
		}
	}
	return obj.Inspect()
}

// symName returns the source name of a symbol, resolving against the
// ctx's pool when available. Without a pool, returns the synthetic
// `<sym:N>` placeholder.
func symName(ctx inspectCtx, s *Symbol) string {
	if ctx.env != nil {
		return ctx.env.Symbols().Name(s.ID)
	}
	return "<sym:" + itoa32(s.ID) + ">"
}
