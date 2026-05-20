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
	if len(n.Parts) > 0 {
		return nil, errorf("evaluator: interpolated regex not yet supported")
	}
	src := n.Value
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
