package evaluator

import (
	"regexp"
	"strings"

	"github.com/lczyk/goruby/ast"
	"github.com/lczyk/goruby/object"
)

// evalRegexLiteral compiles a regex literal to a Go regexp.Regexp.
// Best-effort translation of ruby flags: `i` (case-insensitive), `m`
// (dotall in ruby vs single-line in Go: handled), `x` (extended:
// approximated by stripping whitespace + comments).
func evalRegexLiteral(env *object.Environment, n *ast.RegexLiteral) (object.RubyObject, error) {
	src, err := composeRegexSource(env, n)
	if err != nil {
		return nil, err
	}
	flags := ""
	if strings.Contains(n.Options, "i") {
		flags += "i"
	}
	if strings.Contains(n.Options, "m") {
		// ruby /m flag is "dot matches newline"; in Go regex syntax
		// this is `(?s)`.
		flags += "s"
	}
	if strings.Contains(n.Options, "x") {
		// extended: drop whitespace + comments outside char classes.
		src = stripExtended(src)
	}
	pattern := src
	if flags != "" {
		pattern = "(?" + flags + ")" + src
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return raiseBuiltin(env, "RegexpError", err.Error())
	}
	return object.NewRegex(re, src, n.Options), nil
}

// composeRegexSource returns the regex source string for n. For
// non-interpolated regexes this is just n.Value. For interpolated
// regexes (n.Parts non-empty), evaluates each interpolation part and
// concatenates: literal StringContent chunks pass through verbatim
// (the regex engine handles backslash escapes); embedded expressions
// stringify via Kernel#to_s. MRI does not auto-quote interpolated
// values -- callers wanting that use Regexp.quote.
func composeRegexSource(env *object.Environment, n *ast.RegexLiteral) (string, error) {
	if len(n.Parts) == 0 {
		return n.Value, nil
	}
	var b strings.Builder
	for _, p := range n.Parts {
		switch part := p.(type) {
		case *ast.StringContent:
			b.WriteString(part.Value)
		default:
			v, err := Eval(p, env)
			if err != nil {
				return "", err
			}
			b.WriteString(toStringValue(env, v))
		}
	}
	return b.String(), nil
}

// regexMatch implements the `=~` operator: returns the byte index of
// the first match, or nil. Accepts either side as the regex.
func regexMatch(env *object.Environment, left, right object.RubyObject) object.RubyObject {
	var re *object.Regex
	var s string
	if r, ok := left.(*object.Regex); ok {
		re = r
		if t, ok := stringText(env, right); ok {
			s = t
		} else {
			return object.NIL
		}
	} else if r, ok := right.(*object.Regex); ok {
		re = r
		if t, ok := stringText(env, left); ok {
			s = t
		} else {
			return object.NIL
		}
	} else {
		return object.NIL
	}
	idx := re.RE.FindStringIndex(s)
	if idx == nil {
		return object.NIL
	}
	return object.NewInteger(int64(idx[0]))
}

// bootstrapRegexpClass installs the Regexp class so source code can
// reference it via `Regexp.union(...)` / `Regexp.escape(...)`. Idempotent.
func bootstrapRegexpClass(env *object.Environment) *object.Class {
	c := object.RegexpClass
	// ClassMethods + instance Methods land on the package-level singleton.
	// Idempotent across envs: a second bootstrap sees them present and
	// is a no-op.
	if _, ok := c.ClassMethods["union"]; !ok {
		c.ClassMethods["union"] = &object.UserMethod{Name: "union", Body: nativeFn{Fn: regexpUnion}}
		c.ClassMethods["escape"] = &object.UserMethod{Name: "escape", Body: nativeFn{Fn: regexpEscape}}
		c.ClassMethods["quote"] = &object.UserMethod{Name: "quote", Body: nativeFn{Fn: regexpEscape}}
	}
	if _, ok := c.Methods["match?"]; !ok {
		c.Methods["match?"] = &object.BuiltinMethod{Name: "match?", Fn: regexMatchQ}
		c.Methods["match"] = &object.BuiltinMethod{Name: "match", Fn: regexMatchCall}
		c.Methods["=~"] = &object.BuiltinMethod{Name: "=~", Fn: regexTildeMatch}
		c.Methods["source"] = &object.BuiltinMethod{Name: "source", Fn: regexSource}
		c.Methods["options"] = &object.BuiltinMethod{Name: "options", Fn: regexOptions}
		c.Methods["to_s"] = &object.BuiltinMethod{Name: "to_s", Fn: regexToS}
		c.Methods["inspect"] = &object.BuiltinMethod{Name: "inspect", Fn: regexToS}
	}
	env.SetGlobal("Regexp", c)
	return c
}

// regexMatchQ implements Regexp#match? -- true iff the regex matches s.
func regexMatchQ(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
	r, ok := recv.(*object.Regex)
	if !ok {
		return nil, errorf("evaluator: Regexp#match? on non-Regex %T", recv)
	}
	if len(args) < 1 {
		return nil, errorf("evaluator: Regexp#match? wrong number of arguments (given %d, expected 1..2)", len(args))
	}
	s, ok := stringText(env, args[0])
	if !ok {
		return nil, errorf("evaluator: Regexp#match? expects String, got %T", args[0])
	}
	return object.BooleanOf(r.RE.MatchString(s)), nil
}

// regexMatchCall implements Regexp#match -- returns the matched substring
// (group 0) or nil. MRI returns a MatchData; we approximate with the
// matched string, which is what most corpus uses inspect on.
func regexMatchCall(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
	r, ok := recv.(*object.Regex)
	if !ok {
		return nil, errorf("evaluator: Regexp#match on non-Regex %T", recv)
	}
	if len(args) < 1 {
		return nil, errorf("evaluator: Regexp#match wrong number of arguments (given %d, expected 1..2)", len(args))
	}
	s, ok := stringText(env, args[0])
	if !ok {
		return nil, errorf("evaluator: Regexp#match expects String, got %T", args[0])
	}
	if m := r.RE.FindString(s); m != "" {
		return object.NewString(m), nil
	}
	if loc := r.RE.FindStringIndex(s); loc != nil {
		return object.NewString(""), nil
	}
	return object.NIL, nil
}

// regexTildeMatch implements Regexp#=~ -- byte index of first match or nil.
func regexTildeMatch(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
	if len(args) != 1 {
		return nil, errorf("evaluator: Regexp#=~ wrong number of arguments (given %d, expected 1)", len(args))
	}
	return regexMatch(env, recv, args[0]), nil
}

func regexSource(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
	r, ok := recv.(*object.Regex)
	if !ok {
		return nil, errorf("evaluator: Regexp#source on non-Regex %T", recv)
	}
	return object.NewString(r.Source), nil
}

// regexOptions returns the bitmask of MRI Regexp option flags:
// IGNORECASE=1, EXTENDED=2, MULTILINE=4. Mirrors MRI's Regexp#options.
func regexOptions(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
	r, ok := recv.(*object.Regex)
	if !ok {
		return nil, errorf("evaluator: Regexp#options on non-Regex %T", recv)
	}
	var bits int64
	if strings.Contains(r.Options, "i") {
		bits |= 1
	}
	if strings.Contains(r.Options, "x") {
		bits |= 2
	}
	if strings.Contains(r.Options, "m") {
		bits |= 4
	}
	return object.NewInteger(bits), nil
}

func regexToS(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
	r, ok := recv.(*object.Regex)
	if !ok {
		return nil, errorf("evaluator: Regexp#to_s on non-Regex %T", recv)
	}
	return object.NewString(r.Inspect()), nil
}

// regexpUnion builds a single regex matching any of args. Strings get
// quoted (literal match), Regex objects contribute their source. A
// single Array argument is auto-splatted to mirror MRI.
func regexpUnion(_ *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	// MRI: Regexp.union([a, b]) is equivalent to Regexp.union(a, b).
	if len(args) == 1 {
		if arr, ok := args[0].(*object.Array); ok {
			args = arr.Elements
		}
	}
	if len(args) == 0 {
		// MRI returns /(?!)/ -- a regex that matches nothing. Mirror.
		re := regexp.MustCompile(`(?!)`)
		return object.NewRegex(re, `(?!)`, ""), nil
	}
	parts := make([]string, 0, len(args))
	for _, a := range args {
		switch v := a.(type) {
		case *object.Regex:
			parts = append(parts, v.Source)
		case *object.String:
			parts = append(parts, regexp.QuoteMeta(v.Value()))
		default:
			return nil, errorf("evaluator: Regexp.union: expected String/Regexp/Array, got %T", a)
		}
	}
	src := strings.Join(parts, "|")
	re, err := regexp.Compile(src)
	if err != nil {
		return nil, errorf("evaluator: Regexp.union: %s", err.Error())
	}
	return object.NewRegex(re, src, ""), nil
}

func regexpEscape(_ *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	if len(args) != 1 {
		return nil, errorf("evaluator: Regexp.escape: wrong number of arguments (given %d, expected 1)", len(args))
	}
	s, ok := args[0].(*object.String)
	if !ok {
		return nil, errorf("evaluator: Regexp.escape: expected String, got %T", args[0])
	}
	return object.NewString(regexp.QuoteMeta(s.Value())), nil
}

// stripExtended removes whitespace and `#` comments from a regex source
// outside of character classes. Approximates ruby `/x` mode.
func stripExtended(s string) string {
	var b strings.Builder
	inClass := false
	i := 0
	for i < len(s) {
		c := s[i]
		switch {
		case c == '\\' && i+1 < len(s):
			b.WriteByte(c)
			b.WriteByte(s[i+1])
			i += 2
			continue
		case c == '[':
			inClass = true
		case c == ']':
			inClass = false
		}
		if !inClass {
			if c == ' ' || c == '\t' || c == '\n' {
				i++
				continue
			}
			if c == '#' {
				for i < len(s) && s[i] != '\n' {
					i++
				}
				continue
			}
		}
		b.WriteByte(c)
		i++
	}
	return b.String()
}
