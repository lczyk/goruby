package evaluator

import (
	"github.com/lczyk/goruby/object"
)

// bootstrapEncodingClass installs a minimal stub of ruby's Encoding
// class. The corpus needs:
//   - Encoding::UTF_8 (and a handful of other names) as a sentinel
//     constant -- callers compare for identity or pass it as an arg.
//   - Encoding.default_internal= / default_external= as no-op setters
//     so source files that pin the encoding at load time don't error.
//   - Encoding.list / Encoding.name_list -- not yet exposed; add when
//     hit.
//
// We don't actually track string encodings -- everything is treated as
// raw bytes / UTF-8. The stub exists so code paths that interact with
// Encoding don't raise NameError on the constant lookup.
func bootstrapEncodingClass(env *object.Environment) *object.Class {
	if existing, ok := env.Get("Encoding"); ok {
		if c, ok := existing.(*object.Class); ok {
			return c
		}
	}
	c := object.NewClass("Encoding", nil)
	// Sentinel encoding values: each is a distinct Instance of Encoding
	// so identity / class checks behave plausibly. The Name ivar lets
	// `enc.name` and inspect report the encoding's label.
	encNames := []string{
		"UTF_8",
		"ASCII_8BIT",
		"BINARY",
		"US_ASCII",
		"ASCII",
	}
	for _, name := range encNames {
		inst := &object.Instance{
			C:     c,
			Ivars: map[string]object.RubyObject{"@name": object.NewString(encodingDisplayName(name))},
		}
		c.Constants[name] = inst
	}
	// No-op setters for the global default encodings.
	c.ClassMethods["default_internal="] = &object.UserMethod{Name: "default_internal=", Body: nativeFn{fn: encodingNoop}}
	c.ClassMethods["default_external="] = &object.UserMethod{Name: "default_external=", Body: nativeFn{fn: encodingNoop}}
	c.ClassMethods["default_internal"] = &object.UserMethod{Name: "default_internal", Body: nativeFn{fn: encodingReturnUTF8(c)}}
	c.ClassMethods["default_external"] = &object.UserMethod{Name: "default_external", Body: nativeFn{fn: encodingReturnUTF8(c)}}
	c.ClassMethods["find"] = &object.UserMethod{Name: "find", Body: nativeFn{fn: encodingReturnUTF8(c)}}
	// Encoding.list -> [enc, enc, ...] (one per distinct sentinel).
	// We dedupe by display name so e.g. ASCII_8BIT / BINARY don't
	// double up.
	c.ClassMethods["list"] = &object.UserMethod{Name: "list", Body: nativeFn{fn: func(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
		seen := map[string]bool{}
		var out []object.RubyObject
		for _, n := range encNames {
			d := encodingDisplayName(n)
			if seen[d] {
				continue
			}
			seen[d] = true
			out = append(out, c.Constants[n])
		}
		return object.NewArray(out...), nil
	}}}
	c.ClassMethods["name_list"] = &object.UserMethod{Name: "name_list", Body: nativeFn{fn: func(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
		seen := map[string]bool{}
		var out []object.RubyObject
		for _, n := range encNames {
			d := encodingDisplayName(n)
			if seen[d] {
				continue
			}
			seen[d] = true
			out = append(out, object.NewString(d))
		}
		return object.NewArray(out...), nil
	}}}
	c.ClassMethods["compatible?"] = &object.UserMethod{Name: "compatible?", Body: nativeFn{fn: func(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
		// Stub: assume any two strings/encodings are compatible under
		// UTF-8. Returns the UTF-8 sentinel. MRI returns nil for genuine
		// mismatches (e.g. invalid bytes in one of them) -- we don't
		// track per-string encodings, so always-compatible is the most
		// useful default.
		if len(args) < 2 {
			return object.NIL, nil
		}
		return c.Constants["UTF_8"], nil
	}}}
	// Instance methods on Encoding values: name, to_s, inspect.
	c.Methods["name"] = &object.BuiltinMethod{Name: "name", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		if inst, ok := recv.(*object.Instance); ok {
			if n, ok := inst.Ivars["@name"]; ok {
				return n, nil
			}
		}
		return object.NewString("UTF-8"), nil
	}}
	c.Methods["to_s"] = c.Methods["name"]
	c.Methods["inspect"] = &object.BuiltinMethod{Name: "inspect", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		name := "UTF-8"
		if inst, ok := recv.(*object.Instance); ok {
			if n, ok := inst.Ivars["@name"]; ok {
				if s, ok := stringText(env, n); ok {
					name = s
				}
			}
		}
		return object.NewString("#<Encoding:" + name + ">"), nil
	}}
	env.SetGlobal("Encoding", c)
	return c
}

func encodingDisplayName(snake string) string {
	switch snake {
	case "UTF_8":
		return "UTF-8"
	case "ASCII_8BIT", "BINARY":
		return "ASCII-8BIT"
	case "US_ASCII", "ASCII":
		return "US-ASCII"
	}
	return snake
}

func encodingNoop(_ *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	if len(args) >= 1 {
		return args[0], nil
	}
	return object.NIL, nil
}

func encodingReturnUTF8(c *object.Class) func(*object.Environment, []object.RubyObject) (object.RubyObject, error) {
	return func(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
		if v, ok := c.Constants["UTF_8"]; ok {
			return v, nil
		}
		return object.NIL, nil
	}
}
