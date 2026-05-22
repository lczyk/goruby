package stdlib

import (
	"strings"

	"github.com/lczyk/goruby/evaluator/builtinapi"
	"github.com/lczyk/goruby/object"
)

// optHandler is one .on(...) registration. prefixes lists every flag
// form the .on call accepted (e.g. ["-v", "--verbose"]); valueful=true
// when one of those forms declares an argument ("-n NAME" / "--name=NAME").
// block is whatever env.CurrentBlock looked like at the .on call site;
// parse! later fires it via builtinapi.InvokeBlockValue.
type optHandler struct {
	prefixes []string
	valueful bool
	block    any
}

// BootstrapOptionParser installs a minimal stub of ruby's stdlib
// OptionParser. Covers the surface the corpus actually hits today:
//
//	parser = OptionParser.new { |p| ... }
//	p.banner = "..."
//	p.on(*spec) { |val| ... }   -- register handler
//	p.parse!(argv)              -- consume registered options from argv,
//	                              invoking each handler's block;
//	                              non-option tokens are left in argv
//
// OptionParser::InvalidOption is installed as an exception subclass so
// rescue clauses referencing it resolve.
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
	c.Methods["on"] = &object.BuiltinMethod{Name: "on", Fn: optionOn}
	c.Methods["on_tail"] = c.Methods["on"]
	c.Methods["on_head"] = c.Methods["on"]
	c.Methods["parse!"] = &object.BuiltinMethod{Name: "parse!", Fn: optionParseBang}
	c.Methods["parse"] = c.Methods["parse!"]

	env.SetGlobal("OptionParser", c)
	return c
}

func optionParserNew(c *object.Class) func(*object.Environment, []object.RubyObject) (object.RubyObject, error) {
	return func(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
		inst := &object.Instance{C: c, Ivars: map[string]object.RubyObject{}}
		if _, _, err := builtinapi.InvokeCurrentBlock(env, []object.RubyObject{inst}); err != nil {
			return nil, err
		}
		return inst, nil
	}
}

// handlersOf returns the (possibly empty) handler slice stashed on the
// parser instance via the unexported @__handlers__ ivar slot.
func handlersOf(inst *object.Instance) []optHandler {
	v, ok := inst.Ivars["@__handlers__"]
	if !ok {
		return nil
	}
	box, ok := v.(*handlerBox)
	if !ok {
		return nil
	}
	return box.handlers
}

// handlerBox wraps the handler slice as a RubyObject so it can sit in
// the Ivars map. Never user-visible.
type handlerBox struct {
	handlers []optHandler
}

func (h *handlerBox) Type() object.Type       { return object.OBJECT_OBJ }
func (h *handlerBox) Class() object.RubyClass { return nil }
func (h *handlerBox) Inspect() string         { return "#<optparse handler-box>" }

func setHandlers(inst *object.Instance, h []optHandler) {
	box, ok := inst.Ivars["@__handlers__"].(*handlerBox)
	if !ok {
		box = &handlerBox{}
		inst.Ivars["@__handlers__"] = box
	}
	box.handlers = h
}

// optionOn registers a handler. Spec strings are scanned for "-X" and
// "--xxx" prefixes; if any spec mentions an argument (whitespace or
// '=' followed by a placeholder), the handler is marked valueful.
func optionOn(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
	inst, ok := recv.(*object.Instance)
	if !ok {
		return recv, nil
	}
	h := optHandler{block: block}
	for _, a := range args {
		s, ok := builtinapi.StringText(env, a)
		if !ok {
			continue
		}
		s = strings.TrimSpace(s)
		if !strings.HasPrefix(s, "-") {
			continue
		}
		// Strip an argument placeholder ("-n NAME" / "--name=NAME") so
		// the stored prefix is just the flag itself; mark valueful.
		flag := s
		if i := strings.IndexAny(s, " \t="); i >= 0 {
			flag = s[:i]
			h.valueful = true
		}
		h.prefixes = append(h.prefixes, flag)
	}
	hs := handlersOf(inst)
	hs = append(hs, h)
	setHandlers(inst, hs)
	return recv, nil
}

// optionParseBang walks argv removing each token that matches a
// registered handler. valueful handlers consume either an attached
// value ("--name=alice", "-nalice") or the next argv element ("-n alice").
// Non-option tokens (no '-' prefix) and unknown options are left in
// place.
func optionParseBang(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
	if len(args) < 1 {
		return object.NIL, nil
	}
	arr, ok := args[0].(*object.Array)
	if !ok {
		return object.NIL, nil
	}
	inst, ok := recv.(*object.Instance)
	if !ok {
		return arr, nil
	}
	handlers := handlersOf(inst)

	out := arr.Elements[:0]
	i := 0
	for i < len(arr.Elements) {
		tok, ok := builtinapi.StringText(env, arr.Elements[i])
		if !ok || !strings.HasPrefix(tok, "-") {
			out = append(out, arr.Elements[i])
			i++
			continue
		}
		// Find a matching handler. For valueful handlers, the token may
		// be just the flag (value comes from next argv element) or
		// flag=value / -fvalue (value attached).
		matched := false
		for _, h := range handlers {
			for _, p := range h.prefixes {
				if tok == p {
					// Plain match.
					if h.valueful && i+1 < len(arr.Elements) {
						if val, ok := builtinapi.StringText(env, arr.Elements[i+1]); ok {
							if _, err := builtinapi.InvokeBlockValue(env, h.block, []object.RubyObject{object.NewString(val)}); err != nil {
								return nil, err
							}
							i += 2
							matched = true
							break
						}
					}
					if _, err := builtinapi.InvokeBlockValue(env, h.block, nil); err != nil {
						return nil, err
					}
					i++
					matched = true
					break
				}
				if h.valueful && strings.HasPrefix(tok, p+"=") {
					val := tok[len(p)+1:]
					if _, err := builtinapi.InvokeBlockValue(env, h.block, []object.RubyObject{object.NewString(val)}); err != nil {
						return nil, err
					}
					i++
					matched = true
					break
				}
				if h.valueful && strings.HasPrefix(tok, p) && len(tok) > len(p) && !strings.HasPrefix(p, "--") {
					// Short-form attached value: -nalice
					val := tok[len(p):]
					if _, err := builtinapi.InvokeBlockValue(env, h.block, []object.RubyObject{object.NewString(val)}); err != nil {
						return nil, err
					}
					i++
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}
		if !matched {
			out = append(out, arr.Elements[i])
			i++
		}
	}
	arr.Elements = out
	return arr, nil
}
