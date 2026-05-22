package stdlib

import (
	"github.com/lczyk/goruby/evaluator/builtinapi"
	"github.com/lczyk/goruby/object"
)

// BootstrapOptionParser installs a minimal stub of ruby's stdlib
// OptionParser. The real OptionParser handles option specifications,
// type coercion, automatic help, etc. We only need the surface the
// corpus actually hits today:
//
//	parser = OptionParser.new { |p| ... }
//	p.banner = "..."
//	p.on(*args) { |val| ... }   -- register handler (we DON'T invoke)
//	p.parse!(argv)              -- strip leading "-"-prefix tokens
//	                              from argv (best-effort -- doesn't
//	                              parse arg shapes or call the
//	                              registered handlers)
//
// OptionParser::InvalidOption is installed as a no-op exception
// subclass so rescue clauses referencing it resolve.
func BootstrapOptionParser(env *object.Environment) *object.Class {
	if existing, ok := env.Get("OptionParser"); ok {
		if c, ok := existing.(*object.Class); ok {
			return c
		}
	}
	c := object.NewClass("OptionParser", nil)

	stdErr, _ := env.Get("StandardError")
	stdErrCls, _ := stdErr.(*object.Class)
	invalid := object.NewClass("InvalidOption", stdErrCls)
	c.Constants["InvalidOption"] = invalid

	c.ClassMethods["new"] = &object.UserMethod{
		Name: "new",
		Body: builtinapi.NativeFn{Fn: optionParserNew(c)},
	}

	c.Methods["banner="] = &object.BuiltinMethod{Name: "banner=", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		if len(args) >= 1 {
			if inst, ok := recv.(*object.Instance); ok {
				inst.Ivars["@banner"] = args[0]
			}
		}
		return object.NIL, nil
	}}
	c.Methods["banner"] = &object.BuiltinMethod{Name: "banner", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		if inst, ok := recv.(*object.Instance); ok {
			if v, ok := inst.Ivars["@banner"]; ok {
				return v, nil
			}
		}
		return object.NewString(""), nil
	}}
	c.Methods["on"] = &object.BuiltinMethod{Name: "on", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		return recv, nil
	}}
	c.Methods["on_tail"] = c.Methods["on"]
	c.Methods["on_head"] = c.Methods["on"]
	c.Methods["parse!"] = &object.BuiltinMethod{Name: "parse!", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		if len(args) >= 1 {
			if arr, ok := args[0].(*object.Array); ok {
				out := arr.Elements[:0]
				for _, e := range arr.Elements {
					if s, ok := e.(*object.String); ok && len(s.Buf) > 0 && s.Buf[0] == '-' {
						continue
					}
					out = append(out, e)
				}
				arr.Elements = out
				return arr, nil
			}
		}
		return object.NIL, nil
	}}
	c.Methods["parse"] = c.Methods["parse!"]

	env.SetGlobal("OptionParser", c)
	return c
}

// optionParserNew yields the new parser instance to the block (if any)
// and returns it. Mirrors MRI: OptionParser.new { |p| ... } returns
// the parser regardless of what the block does.
func optionParserNew(c *object.Class) func(*object.Environment, []object.RubyObject) (object.RubyObject, error) {
	return func(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
		inst := &object.Instance{C: c, Ivars: map[string]object.RubyObject{}}
		if _, _, err := builtinapi.InvokeCurrentBlock(env, []object.RubyObject{inst}); err != nil {
			return nil, err
		}
		return inst, nil
	}
}
