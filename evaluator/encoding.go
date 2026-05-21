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
	for _, name := range []string{
		"UTF_8",
		"ASCII_8BIT",
		"BINARY",
		"US_ASCII",
		"ASCII",
	} {
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
