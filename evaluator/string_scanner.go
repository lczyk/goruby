package evaluator

import (
	"github.com/lczyk/goruby/object"
)

// bootstrapStringScanner installs a minimal subset of ruby's
// stdlib strscan::StringScanner. Covers the surface the corpus
// hits today:
//
//	StringScanner.new(s)  -> scanner over s
//	scanner.eos?          -> bool
//	scanner.scan(regex)   -> matched text or nil; advances pos
//	scanner[i]            -> capture group i from last scan
//	scanner.rest          -> unread portion
//	scanner.pos / pos=    -> byte cursor
//
// StringScanner state lives in instance ivars: @src, @pos, @matches
// (last scan's capture groups, with index 0 = whole match).
func bootstrapStringScanner(env *object.Environment) *object.Class {
	if existing, ok := env.Get("StringScanner"); ok {
		if c, ok := existing.(*object.Class); ok {
			return c
		}
	}
	c := object.NewClass("StringScanner", nil)

	c.ClassMethods["new"] = &object.UserMethod{Name: "new", Body: nativeFn{fn: scannerNew(c)}}

	c.Methods["eos?"] = &object.BuiltinMethod{Name: "eos?", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		s, pos := scannerState(recv)
		return object.BooleanOf(pos >= len(s)), nil
	}}
	c.Methods["scan"] = &object.BuiltinMethod{Name: "scan", Fn: scannerScan}
	c.Methods["[]"] = &object.BuiltinMethod{Name: "[]", Fn: scannerGroup}
	c.Methods["rest"] = &object.BuiltinMethod{Name: "rest", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		s, pos := scannerState(recv)
		if pos >= len(s) {
			return object.NewString(""), nil
		}
		return object.NewString(s[pos:]), nil
	}}
	c.Methods["pos"] = &object.BuiltinMethod{Name: "pos", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		_, pos := scannerState(recv)
		return object.NewInteger(int64(pos)), nil
	}}
	c.Methods["pos="] = &object.BuiltinMethod{Name: "pos=", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		if len(args) != 1 {
			return nil, errorf("evaluator: StringScanner#pos= expects 1 arg")
		}
		n, ok := args[0].(*object.Integer)
		if !ok {
			return nil, errorf("evaluator: StringScanner#pos= expects Integer")
		}
		if inst, ok := recv.(*object.Instance); ok {
			inst.Ivars["@pos"] = n
		}
		return args[0], nil
	}}
	c.Methods["string"] = &object.BuiltinMethod{Name: "string", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		s, _ := scannerState(recv)
		return object.NewString(s), nil
	}}

	env.SetGlobal("StringScanner", c)
	return c
}

func scannerNew(c *object.Class) func(*object.Environment, []object.RubyObject) (object.RubyObject, error) {
	return func(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
		if len(args) != 1 {
			return nil, errorf("evaluator: StringScanner.new expects 1 arg, got %d", len(args))
		}
		s, ok := stringText(env, args[0])
		if !ok {
			return nil, errorf("evaluator: StringScanner.new expects String, got %T", args[0])
		}
		return &object.Instance{
			C: c,
			Ivars: map[string]object.RubyObject{
				"@src":     object.NewString(s),
				"@pos":     object.NewInteger(0),
				"@matches": object.NewArray(),
			},
		}, nil
	}
}

func scannerState(recv object.RubyObject) (string, int) {
	inst, ok := recv.(*object.Instance)
	if !ok {
		return "", 0
	}
	var s string
	if src, ok := inst.Ivars["@src"].(*object.String); ok {
		s = string(src.Buf)
	}
	var pos int
	if p, ok := inst.Ivars["@pos"].(*object.Integer); ok {
		pos = int(p.Value)
	}
	return s, pos
}

func scannerScan(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
	if len(args) != 1 {
		return nil, errorf("evaluator: StringScanner#scan expects 1 arg")
	}
	re, ok := args[0].(*object.Regex)
	if !ok {
		return nil, errorf("evaluator: StringScanner#scan expects Regexp, got %T", args[0])
	}
	inst, ok := recv.(*object.Instance)
	if !ok {
		return nil, errorf("evaluator: StringScanner#scan on non-Instance %T", recv)
	}
	s, pos := scannerState(recv)
	if pos >= len(s) {
		return object.NIL, nil
	}
	// Anchor: scan only matches at the current position. Use
	// FindStringSubmatchIndex on the remaining slice and reject
	// any match whose start isn't 0 (i.e., not at pos).
	rest := s[pos:]
	loc := re.RE.FindStringSubmatchIndex(rest)
	if loc == nil || loc[0] != 0 {
		return object.NIL, nil
	}
	matched := rest[loc[0]:loc[1]]
	// Capture groups: [0] is whole match, [i] is group i. Store as
	// Array so [] can index.
	caps := []object.RubyObject{object.NewString(matched)}
	for g := 1; g*2+1 < len(loc); g++ {
		if loc[g*2] == -1 {
			caps = append(caps, object.NIL)
			continue
		}
		caps = append(caps, object.NewString(rest[loc[g*2]:loc[g*2+1]]))
	}
	inst.Ivars["@matches"] = object.NewArray(caps...)
	inst.Ivars["@pos"] = object.NewInteger(int64(pos + loc[1]))
	return object.NewString(matched), nil
}

func scannerGroup(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
	if len(args) != 1 {
		return nil, errorf("evaluator: StringScanner#[] expects 1 arg")
	}
	n, ok := args[0].(*object.Integer)
	if !ok {
		return nil, errorf("evaluator: StringScanner#[] expects Integer, got %T", args[0])
	}
	inst, ok := recv.(*object.Instance)
	if !ok {
		return nil, errorf("evaluator: StringScanner#[] on non-Instance %T", recv)
	}
	matches, _ := inst.Ivars["@matches"].(*object.Array)
	if matches == nil {
		return object.NIL, nil
	}
	idx := int(n.Value)
	if idx < 0 || idx >= len(matches.Elements) {
		return object.NIL, nil
	}
	return matches.Elements[idx], nil
}
