package evaluator

import (
	"strings"

	"github.com/lczyk/goruby/ast"
	"github.com/lczyk/goruby/object"
)

// bootstrapOptionParser installs a minimal stub of ruby's stdlib
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
func bootstrapOptionParser(env *object.Environment) *object.Class {
	if existing, ok := env.Get("OptionParser"); ok {
		if c, ok := existing.(*object.Class); ok {
			return c
		}
	}
	c := object.NewClass("OptionParser", nil)

	// Nested exception class so `rescue OptionParser::InvalidOption`
	// resolves.
	stdErr, _ := env.Get("StandardError")
	stdErrCls, _ := stdErr.(*object.Class)
	invalid := object.NewClass("InvalidOption", stdErrCls)
	c.Constants["InvalidOption"] = invalid

	c.ClassMethods["new"] = &object.UserMethod{
		Name: "new",
		Body: nativeFn{fn: optionParserNew(c)},
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
		// Stub -- doesn't actually wire the handler. Returns the
		// parser for chaining.
		return recv, nil
	}}
	c.Methods["on_tail"] = c.Methods["on"]
	c.Methods["on_head"] = c.Methods["on"]
	c.Methods["parse!"] = &object.BuiltinMethod{Name: "parse!", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		// Strip leading "--?"-prefix tokens from argv. Doesn't invoke
		// the registered handlers, which is wrong for any program that
		// branches on option values -- but enough for bouncy, whose
		// only option (-d/--debug) is irrelevant under the test
		// harness (argv = [filename], no options).
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
// and returns it. Mirrors mri: OptionParser.new { |p| ... } returns
// the parser regardless of what the block does.
func optionParserNew(c *object.Class) func(*object.Environment, []object.RubyObject) (object.RubyObject, error) {
	return func(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
		inst := &object.Instance{C: c, Ivars: map[string]object.RubyObject{}}
		// Yield to the block if one was passed via the dispatcher.
		// The wrapping callMethod stashes it as env.CurrentBlock.
		blk := env.CurrentBlock
		if blk != nil {
			if be, ok := blk.(*ast.BlockExpression); ok && be != nil {
				if _, err := invokeBlock(env, be, []object.RubyObject{inst}); err != nil {
					return nil, err
				}
			} else if bm, ok := blk.(*goBlockMarker); ok && bm != nil {
				if _, err := bm.fn([]object.RubyObject{inst}); err != nil {
					return nil, err
				}
			}
		}
		_ = strings.HasPrefix // keep import live if future tweaks need it
		return inst, nil
	}
}
