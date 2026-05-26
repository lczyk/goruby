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
	// optionalValue is true when the spec used `--flag=[VAL]` brackets
	// (or "--flag [VAL]" -- optional value form). MRI only consumes a
	// next-token value for required-value flags; optional-value flags
	// only accept the attached `--flag=val` form.
	optionalValue bool
	block         any
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
	// MRI's InvalidOption#message prefixes "invalid option: " to the
	// user-supplied message. Wire as a method override on the class.
	invalid.AddMethod("message", &object.BuiltinMethod{Name: "message", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		if inst, ok := recv.(*object.Instance); ok {
			if m, ok := inst.Ivars["@message"].(*object.String); ok {
				msg := m.Value()
				if !strings.HasPrefix(msg, "invalid option: ") {
					msg = "invalid option: " + msg
				}
				return object.NewString(msg), nil
			}
		}
		return object.NewString("invalid option"), nil
	}})
	invalid.AddMethod("to_s", invalid.Methods["message"])
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
	// parse (non-destructive form): MRI parses argv but leaves it
	// untouched. Clone the array first; parse! mutates the clone.
	c.Methods["parse"] = &object.BuiltinMethod{Name: "parse", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		if len(args) >= 1 {
			if arr, ok := args[0].(*object.Array); ok {
				dup := &object.Array{Elements: append([]object.RubyObject(nil), arr.Elements...)}
				return optionParseBang(env, recv, []object.RubyObject{dup}, block)
			}
		}
		return optionParseBang(env, recv, args, block)
	}}
	// Help-text builders -- rake's options() calls these throughout
	// the parser configuration. We don't render help; just accept
	// the call and return self so chaining works.
	noopSelf := &object.BuiltinMethod{Name: "(optparse-noop)", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		return recv, nil
	}}
	c.Methods["separator"] = noopSelf
	c.Methods["version="] = noopSelf
	c.Methods["program_name="] = noopSelf
	c.Methods["summary_width="] = noopSelf
	c.Methods["accept"] = noopSelf
	c.Methods["environment"] = noopSelf
	c.Methods["help"] = &object.BuiltinMethod{Name: "help", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		// MRI returns a String dump of the help; we have no help
		// table to render, so an empty String is honest.
		return object.NewString(""), nil
	}}

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
		// rake's standard_rake_options passes lambdas as a positional
		// arg (not as a block). Capture the last Proc-shaped arg as
		// the handler block.
		if p, ok := a.(*object.Proc); ok {
			h.block = p
			continue
		}
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
		// `[BRACKETED]` placeholders mark the value as optional.
		flag := s
		if i := strings.IndexAny(s, " \t="); i >= 0 {
			flag = s[:i]
			h.valueful = true
			rest := s[i:]
			// Only `=[...]` (equals + brackets) means strict optional:
			// value can only come via the attached =VAL form. Space-
			// separated `[VAL]` is also optional but MRI lets the
			// space-form consume the next token if it's not a flag.
			trimmed := strings.TrimSpace(rest)
			if strings.HasPrefix(rest, "=[") || (strings.HasPrefix(rest, "=") && strings.HasPrefix(trimmed[1:], "[")) {
				h.optionalValue = true
			}
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
					// Plain match. For required-value options (not the
					// optional-value `--name=[VAL]` form), peek ahead
					// for a value, refusing to consume a flag-prefixed
					// next token.
					if h.valueful && !h.optionalValue && i+1 < len(arr.Elements) {
						if val, ok := builtinapi.StringText(env, arr.Elements[i+1]); ok && !strings.HasPrefix(val, "-") {
							if _, err := builtinapi.InvokeBlockValue(env, h.block, []object.RubyObject{object.NewString(val)}); err != nil {
								return nil, err
							}
							i += 2
							matched = true
							break
						}
					}
					// Valueful with no value -> nil; non-valueful -> true.
					var arg object.RubyObject = object.TRUE
					if h.valueful {
						arg = object.NIL
					}
					if _, err := builtinapi.InvokeBlockValue(env, h.block, []object.RubyObject{arg}); err != nil {
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
			// MRI raises OptionParser::InvalidOption for unknown
			// flag tokens. Bare non-option tokens (no leading '-')
			// pass through.
			if strings.HasPrefix(tok, "-") && tok != "-" && tok != "--" {
				return nil, raiseInvalidOption(env, tok)
			}
			out = append(out, arr.Elements[i])
			i++
		}
	}
	arr.Elements = out
	return arr, nil
}

// raiseInvalidOption raises OptionParser::InvalidOption via the
// builtinapi hook, falling back to StandardError when the class isn't
// installed yet.
func raiseInvalidOption(env *object.Environment, flag string) error {
	// builtinapi.RaiseBuiltin takes a class name; install
	// OptionParser::InvalidOption as a const-named class so it can
	// be resolved by name from the env.
	if op, ok := env.Get("OptionParser"); ok {
		if cls, ok := op.(*object.Class); ok {
			if _, ok := cls.Constants["InvalidOption"].(*object.Class); ok {
				env.SetGlobal("OptionParser::InvalidOption", cls.Constants["InvalidOption"])
				_, err := builtinapi.RaiseBuiltin(env, "OptionParser::InvalidOption", "invalid option: "+flag)
				return err
			}
		}
	}
	_, err := builtinapi.RaiseBuiltin(env, "StandardError", "invalid option: "+flag)
	return err
}
