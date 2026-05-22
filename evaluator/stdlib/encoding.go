package stdlib

import (
	"github.com/lczyk/goruby/evaluator/builtinapi"
	"github.com/lczyk/goruby/object"
)

// BootstrapEncodingClass installs a minimal stub of ruby's Encoding
// class. The corpus needs:
//   - Encoding::UTF_8 (and a handful of other names) as a sentinel
//     constant -- callers compare for identity or pass it as an arg.
//   - Encoding.default_internal= / default_external= as no-op setters
//     so source files that pin the encoding at load time don't error.
//   - Encoding.list / Encoding.name_list / Encoding.compatible? as
//     class-method probes.
//
// We don't track string encodings -- everything is treated as raw
// bytes / UTF-8. The stub exists so code paths that interact with
// Encoding don't raise NameError on the constant lookup.
func BootstrapEncodingClass(env *object.Environment) *object.Class {
	if existing, ok := env.Get("Encoding"); ok {
		if c, ok := existing.(*object.Class); ok {
			return c
		}
	}
	c := object.NewClass("Encoding", nil)
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
	c.ClassMethods["default_internal="] = &object.UserMethod{Name: "default_internal=", Body: builtinapi.NativeFn{Fn: encodingNoop}}
	c.ClassMethods["default_external="] = &object.UserMethod{Name: "default_external=", Body: builtinapi.NativeFn{Fn: encodingNoop}}
	c.ClassMethods["default_internal"] = &object.UserMethod{Name: "default_internal", Body: builtinapi.NativeFn{Fn: encodingReturnUTF8(c)}}
	c.ClassMethods["default_external"] = &object.UserMethod{Name: "default_external", Body: builtinapi.NativeFn{Fn: encodingReturnUTF8(c)}}
	c.ClassMethods["find"] = &object.UserMethod{Name: "find", Body: builtinapi.NativeFn{Fn: encodingReturnUTF8(c)}}
	c.ClassMethods["list"] = &object.UserMethod{Name: "list", Body: builtinapi.NativeFn{Fn: func(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
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
	c.ClassMethods["name_list"] = &object.UserMethod{Name: "name_list", Body: builtinapi.NativeFn{Fn: func(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
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
	c.ClassMethods["compatible?"] = &object.UserMethod{Name: "compatible?", Body: builtinapi.NativeFn{Fn: func(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
		if len(args) < 2 {
			return object.NIL, nil
		}
		return c.Constants["UTF_8"], nil
	}}}
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
				if s, ok := builtinapi.StringText(env, n); ok {
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
