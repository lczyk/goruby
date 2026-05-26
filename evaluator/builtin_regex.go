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
	// Ruby ^/$ are line anchors by default; Go's regexp uses ^/$ for
	// the start/end of input unless (?m) is set. Always prefix so
	// Ruby semantics hold. Mri's "multiline" flag (/m) means dot
	// matches \n, which Go encodes as (?s) -- separate concept.
	flagsToInject := "m"
	if flags != "" {
		flagsToInject += flags
	}
	pattern = "(?" + flagsToInject + ")" + src
	re, err := regexp.Compile(pattern)
	if err != nil {
		// Go's regexp engine doesn't support look-around (?<=...),
		// (?<!...), (?=...), (?!...). When a literal regex uses them
		// (real-world gems hit this; rake/task.rb's first_sentence is
		// the canary), strip the look-around groups and retry. The
		// resulting regex matches more loosely than MRI's, but
		// permits the surrounding code to keep working rather than
		// erroring out at parse time.
		if alt := stripLookaround(pattern); alt != pattern {
			if re2, err2 := regexp.Compile(alt); err2 == nil {
				return object.NewRegex(re2, src, n.Options), nil
			}
		}
		return raiseBuiltin(env, "RegexpError", err.Error())
	}
	return object.NewRegex(re, src, n.Options), nil
}

// stripLookaround removes (?<=...), (?<!...), (?=...), (?!...) groups
// from a pattern. Each is replaced by an empty match; the engine
// then matches without the assertion. Crude but sufficient for the
// `unsupported Perl syntax` regex parser errors that block real-
// world Ruby corpus.
func stripLookaround(p string) string {
	out := p
	for _, prefix := range []string{"(?<=", "(?<!", "(?=", "(?!"} {
		for {
			idx := strings.Index(out, prefix)
			if idx < 0 {
				break
			}
			depth := 1
			end := -1
			for j := idx + len(prefix); j < len(out); j++ {
				switch out[j] {
				case '(':
					depth++
				case ')':
					depth--
					if depth == 0 {
						end = j
					}
				case '\\':
					j++
				}
				if end >= 0 {
					break
				}
			}
			if end < 0 {
				return out
			}
			out = out[:idx] + out[end+1:]
		}
	}
	return out
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
// the first match, or nil. Accepts either side as the regex. Side
// effect: populates $~, $1..$9 with captures from the match (or clears
// them on miss) so subsequent reads see the most recent groups.
func regexMatch(env *object.Environment, left, right object.RubyObject) object.RubyObject {
	var re *object.Regex
	var s string
	if r, ok := left.(*object.Regex); ok {
		re = r
		if t, ok := stringText(env, right); ok {
			s = t
		} else {
			setMatchGlobals(env, nil)
			return object.NIL
		}
	} else if r, ok := right.(*object.Regex); ok {
		re = r
		if t, ok := stringText(env, left); ok {
			s = t
		} else {
			setMatchGlobals(env, nil)
			return object.NIL
		}
	} else {
		setMatchGlobals(env, nil)
		return object.NIL
	}
	idx := re.RE.FindStringSubmatchIndex(s)
	if idx == nil {
		setMatchGlobals(env, nil)
		return object.NIL
	}
	setMatchGlobals(env, submatchesFromIndices(s, idx))
	return object.NewInteger(int64(idx[0]))
}

// submatchesFromIndices builds a []string from FindStringSubmatchIndex
// output. A capture that didn't participate has start == -1; preserve
// that as an empty string-not-set marker -- callers use nil to mean
// "absent". The caller setMatchGlobals layered: it treats a nil-typed
// entry as missing. Encode "missing" as the sentinel "\x00MISS" so we
// can distinguish from a legitimately-empty captured "".
func submatchesFromIndices(s string, idx []int) []string {
	out := make([]string, len(idx)/2)
	for i := 0; i < len(out); i++ {
		st := idx[2*i]
		en := idx[2*i+1]
		if st < 0 {
			out[i] = "\x00MISS"
			continue
		}
		out[i] = s[st:en]
	}
	return out
}

// setMatchGlobals writes regex submatches into $~, $1..$9 globals so
// subsequent reads see the most recent captures. nil clears them.
// MRI also exposes $&, $`, $' but those aren't yet consumed by the
// rake/minitest corpus.
func setMatchGlobals(env *object.Environment, groups []string) {
	if groups == nil {
		env.SetGlobal("$~", object.NIL)
		for i := 1; i <= 9; i++ {
			env.SetGlobal("$"+string(rune('0'+i)), object.NIL)
		}
		return
	}
	if len(groups) > 0 {
		env.SetGlobal("$~", object.NewString(groups[0]))
	}
	for i := 1; i <= 9; i++ {
		var v object.RubyObject = object.NIL
		if i < len(groups) && groups[i] != "\x00MISS" {
			v = object.NewString(groups[i])
		}
		env.SetGlobal("$"+string(rune('0'+i)), v)
	}
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
		// Regexp.new(source[, opts]) compiles source into a Regex.
		// Mirrors MRI's regex constructor; without it,
		// `Regexp.new(...)` falls through Class.new and builds an
		// opaque Instance that fails when methods like #match are
		// dispatched. rake/task_manager.rb's create_rule path needs
		// the proper Regex shape.
		c.ClassMethods["new"] = &object.UserMethod{Name: "new", Body: nativeFn{Fn: regexpNew}}
		c.ClassMethods["compile"] = c.ClassMethods["new"]
		// Regexp.last_match returns the MatchData from the most recent
		// =~ in the current thread. Goruby doesn't track per-thread
		// state for this; return nil. Sufficient for callers that
		// invoke it for its side-effect-free return (minitest's
		// assert_match passes it through unchanged).
		c.ClassMethods["last_match"] = &object.BuiltinMethod{
			Name: "last_match",
			Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
				return object.NIL, nil
			},
		}
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
func regexpNew(_ *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
	if len(args) < 1 {
		return nil, errorf("evaluator: Regexp.new: expected 1..2 args, got %d", len(args))
	}
	var src string
	switch v := args[0].(type) {
	case *object.String:
		src = v.Value()
	case *object.Regex:
		// Regexp.new(/foo/) -- copy the source through.
		return v, nil
	default:
		return nil, errorf("evaluator: Regexp.new: expected String/Regexp, got %T", args[0])
	}
	re, err := regexp.Compile(src)
	if err != nil {
		return nil, errorf("evaluator: Regexp.new: %s", err.Error())
	}
	return object.NewRegex(re, src, ""), nil
}

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
