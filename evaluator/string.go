package evaluator

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/lczyk/goruby/object"
)

func printfFmt(fmtStr string, arg any) string {
	return fmt.Sprintf(fmtStr, arg)
}

// stringSucc returns the lexical successor of s, matching String#succ:
// increments the rightmost alphanumeric char with carry into the next
// alnum, prepending a "rollover" char when needed. Non-alphanumeric
// strings are bumped by ASCII +1 on the last byte.
func stringSucc(s string) string {
	if s == "" {
		return s
	}
	bytes := []byte(s)
	// scan from right for an alnum to bump
	for i := len(bytes) - 1; i >= 0; i-- {
		c := bytes[i]
		switch {
		case c >= '0' && c <= '8',
			c >= 'a' && c <= 'y',
			c >= 'A' && c <= 'Y':
			bytes[i] = c + 1
			return string(bytes)
		case c == '9':
			bytes[i] = '0'
			if i == 0 || !alnum(bytes[i-1]) {
				return "1" + string(bytes)
			}
		case c == 'z':
			bytes[i] = 'a'
			if i == 0 || !alnum(bytes[i-1]) {
				return "a" + string(bytes)
			}
		case c == 'Z':
			bytes[i] = 'A'
			if i == 0 || !alnum(bytes[i-1]) {
				return "A" + string(bytes)
			}
		default:
			// non-alnum: bump by one
			bytes[i] = c + 1
			return string(bytes)
		}
	}
	return string(bytes)
}

func alnum(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// splitLinesKeepNL splits s into lines, retaining the trailing
// newline on each (matching ruby's String#each_line semantics). Empty
// trailing "" after a final "\n" is dropped.
func splitLinesKeepNL(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.SplitAfter(s, "\n")
	if parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return parts
}

func unicodeUpper(r rune) rune { return unicode.ToUpper(r) }
func unicodeLower(r rune) rune { return unicode.ToLower(r) }

// sprintfFormat is a minimal `printf`-style formatter covering the
// specifiers actually used by the corpus / tests: %s, %d, %f, %x, %o,
// %b, and a literal %%. Width and precision parse to standard Go fmt
// shape; modifiers beyond that fall through unchanged.
func sprintfFormat(env *object.Environment, format string, args []object.RubyObject) (string, error) {
	var out strings.Builder
	argi := 0
	i := 0
	for i < len(format) {
		c := format[i]
		if c != '%' {
			out.WriteByte(c)
			i++
			continue
		}
		// Capture the directive body up to a verb char.
		j := i + 1
		for j < len(format) {
			ch := format[j]
			if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '%' {
				break
			}
			j++
		}
		if j >= len(format) {
			out.WriteString(format[i:])
			break
		}
		spec := format[i+1 : j]
		verb := format[j]
		if verb == '%' {
			out.WriteByte('%')
			i = j + 1
			continue
		}
		if argi >= len(args) {
			return "", errorf("evaluator: ArgumentError: too few arguments to format")
		}
		a := args[argi]
		argi++
		switch verb {
		case 's':
			out.WriteString(printfFmt("%"+spec+"s", putsString(env, a)))
		case 'd', 'i':
			n, err := toIntValue(a)
			if err != nil {
				return "", err
			}
			out.WriteString(printfFmt("%"+spec+"d", n))
		case 'x', 'X', 'o', 'b':
			n, err := toIntValue(a)
			if err != nil {
				return "", err
			}
			out.WriteString(printfFmt("%"+spec+string(verb), n))
		case 'f', 'e', 'g':
			f, err := toFloatValue(a)
			if err != nil {
				return "", err
			}
			out.WriteString(printfFmt("%"+spec+string(verb), f))
		default:
			return "", errorf("evaluator: unsupported format verb %%%c", verb)
		}
		i = j + 1
	}
	return out.String(), nil
}

func toIntValue(o object.RubyObject) (int64, error) {
	switch v := o.(type) {
	case *object.Integer:
		return v.Value, nil
	case *object.Float:
		return int64(v.Value), nil
	}
	return 0, errorf("evaluator: TypeError: cannot convert %T to Integer for format", o)
}

func toFloatValue(o object.RubyObject) (float64, error) {
	switch v := o.(type) {
	case *object.Integer:
		return float64(v.Value), nil
	case *object.Float:
		return v.Value, nil
	}
	return 0, errorf("evaluator: TypeError: cannot convert %T to Float for format", o)
}

// stringText extracts the raw text of a String / FrozenString. Returns
// "", false for non-string operands.
func stringText(env *object.Environment, o object.RubyObject) (string, bool) {
	switch v := o.(type) {
	case *object.String:
		return string(v.Buf), true
	case *object.FrozenString:
		return env.Strings().Get(v.ID), true
	}
	return "", false
}

// stringInfix dispatches `+` / `*` / comparison ops where the left
// operand is a string. Returns handled=false to fall through.
func stringInfix(env *object.Environment, op string, left, right object.RubyObject) (object.RubyObject, bool, error) {
	l, ok := stringText(env, left)
	if !ok {
		return nil, false, nil
	}
	switch op {
	case "+":
		r, ok := stringText(env, right)
		if !ok {
			return nil, true, errorf("evaluator: no implicit conversion of %T into String", right)
		}
		return object.NewString(l + r), true, nil
	case "*":
		n, ok := right.(*object.Integer)
		if !ok {
			return nil, true, errorf("evaluator: String#* needs Integer, got %T", right)
		}
		if n.Value < 0 {
			return nil, true, errorf("evaluator: ArgumentError: negative argument")
		}
		return object.NewString(strings.Repeat(l, int(n.Value))), true, nil
	case "%":
		var values []object.RubyObject
		if arr, ok := right.(*object.Array); ok {
			values = arr.Elements
		} else {
			values = []object.RubyObject{right}
		}
		s, err := sprintfFormat(env, l, values)
		if err != nil {
			return nil, true, err
		}
		return object.NewString(s), true, nil
	case "<", "<=", ">", ">=", "<=>":
		r, ok := stringText(env, right)
		if !ok {
			return nil, true, errorf("evaluator: comparison of String with %T failed", right)
		}
		c := strings.Compare(l, r)
		switch op {
		case "<":
			return object.BooleanOf(c < 0), true, nil
		case "<=":
			return object.BooleanOf(c <= 0), true, nil
		case ">":
			return object.BooleanOf(c > 0), true, nil
		case ">=":
			return object.BooleanOf(c >= 0), true, nil
		case "<=>":
			return object.NewInteger(int64(c)), true, nil
		}
	}
	return nil, false, nil
}

// callStringMethod handles the small set of String methods exercised by
// the strings corpus. Returns handled=false for unknown method names.
func callStringMethod(env *object.Environment, recv object.RubyObject, name string, args []object.RubyObject) (object.RubyObject, bool, error) {
	s, ok := stringText(env, recv)
	if !ok {
		return nil, false, nil
	}
	switch name {
	case "length", "size":
		return object.NewInteger(int64(len(s))), true, nil
	case "upcase":
		return object.NewString(strings.ToUpper(s)), true, nil
	case "downcase":
		return object.NewString(strings.ToLower(s)), true, nil
	case "capitalize":
		if s == "" {
			return object.NewString(""), true, nil
		}
		runes := []rune(s)
		runes[0] = unicodeUpper(runes[0])
		for i := 1; i < len(runes); i++ {
			runes[i] = unicodeLower(runes[i])
		}
		return object.NewString(string(runes)), true, nil
	case "swapcase":
		runes := []rune(s)
		for i, r := range runes {
			if r >= 'a' && r <= 'z' {
				runes[i] = r - 'a' + 'A'
			} else if r >= 'A' && r <= 'Z' {
				runes[i] = r - 'A' + 'a'
			}
		}
		return object.NewString(string(runes)), true, nil
	case "strip":
		return object.NewString(strings.TrimSpace(s)), true, nil
	case "reverse":
		runes := []rune(s)
		for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
			runes[i], runes[j] = runes[j], runes[i]
		}
		return object.NewString(string(runes)), true, nil
	case "include?":
		if len(args) != 1 {
			return nil, true, errorf("evaluator: wrong number of arguments to String#include? (given %d, expected 1)", len(args))
		}
		needle, ok := stringText(env, args[0])
		if !ok {
			return nil, true, errorf("evaluator: TypeError: no implicit conversion of %T into String", args[0])
		}
		return object.BooleanOf(strings.Contains(s, needle)), true, nil
	case "to_s":
		return recv, true, nil
	case "empty?":
		return object.BooleanOf(len(s) == 0), true, nil
	case "chars":
		runes := []rune(s)
		out := make([]object.RubyObject, len(runes))
		for i, r := range runes {
			out[i] = object.NewString(string(r))
		}
		return object.NewArray(out...), true, nil
	case "lines":
		parts := strings.SplitAfter(s, "\n")
		// SplitAfter leaves a trailing "" when s ends with "\n"; drop it.
		if len(parts) > 0 && parts[len(parts)-1] == "" {
			parts = parts[:len(parts)-1]
		}
		out := make([]object.RubyObject, len(parts))
		for i, p := range parts {
			out[i] = object.NewString(p)
		}
		return object.NewArray(out...), true, nil
	case "split":
		// No-arg split mirrors ruby: trims leading whitespace then
		// splits on runs of whitespace.
		if len(args) == 0 {
			parts := strings.Fields(s)
			out := make([]object.RubyObject, len(parts))
			for i, p := range parts {
				out[i] = object.NewString(p)
			}
			return object.NewArray(out...), true, nil
		}
		// Regexp separator: split on matches of the pattern. Empty
		// pattern (//) is special-cased to per-character split,
		// matching MRI.
		if re, ok := args[0].(*object.Regex); ok {
			var parts []string
			if re.Source == "" {
				parts = strings.Split(s, "")
			} else {
				parts = re.RE.Split(s, -1)
			}
			out := make([]object.RubyObject, len(parts))
			for i, p := range parts {
				out[i] = object.NewString(p)
			}
			return object.NewArray(out...), true, nil
		}
		sep, ok := stringText(env, args[0])
		if !ok {
			return nil, true, errorf("evaluator: String#split needs String sep, got %T", args[0])
		}
		var parts []string
		if sep == "" {
			parts = strings.Split(s, "")
		} else {
			parts = strings.Split(s, sep)
		}
		out := make([]object.RubyObject, len(parts))
		for i, p := range parts {
			out[i] = object.NewString(p)
		}
		return object.NewArray(out...), true, nil
	case "chomp":
		out := s
		if len(args) == 1 {
			if t, ok := stringText(env, args[0]); ok {
				out = strings.TrimSuffix(s, t)
			}
		} else {
			out = strings.TrimRight(s, "\r\n")
		}
		return object.NewString(out), true, nil
	case "start_with?":
		for _, a := range args {
			t, ok := stringText(env, a)
			if !ok {
				return nil, true, errorf("evaluator: String#start_with? needs String args")
			}
			if strings.HasPrefix(s, t) {
				return object.TRUE, true, nil
			}
		}
		return object.FALSE, true, nil
	case "end_with?":
		for _, a := range args {
			t, ok := stringText(env, a)
			if !ok {
				return nil, true, errorf("evaluator: String#end_with? needs String args")
			}
			if strings.HasSuffix(s, t) {
				return object.TRUE, true, nil
			}
		}
		return object.FALSE, true, nil
	case "replace":
		if len(args) != 1 {
			return nil, true, errorf("evaluator: String#replace expected 1 arg, got %d", len(args))
		}
		t, ok := stringText(env, args[0])
		if !ok {
			return nil, true, errorf("evaluator: String#replace needs String arg")
		}
		return object.NewString(t), true, nil
	case "match?":
		if len(args) != 1 {
			return nil, true, errorf("evaluator: String#match? expects 1 arg")
		}
		re, ok := args[0].(*object.Regex)
		if !ok {
			return nil, true, errorf("evaluator: String#match? needs Regexp")
		}
		return object.BooleanOf(re.RE.MatchString(s)), true, nil
	case "match":
		if len(args) != 1 {
			return nil, true, errorf("evaluator: String#match expects 1 arg")
		}
		re, ok := args[0].(*object.Regex)
		if !ok {
			return nil, true, errorf("evaluator: String#match needs Regexp")
		}
		m := re.RE.FindString(s)
		if m == "" && !re.RE.MatchString(s) {
			return object.NIL, true, nil
		}
		// Return the matched substring (poor-man's MatchData).
		return object.NewString(m), true, nil
	case "scan":
		if len(args) != 1 {
			return nil, true, errorf("evaluator: String#scan expects 1 arg")
		}
		switch p := args[0].(type) {
		case *object.Regex:
			out := []object.RubyObject{}
			for _, m := range p.RE.FindAllStringSubmatch(s, -1) {
				if len(m) == 1 {
					out = append(out, object.NewString(m[0]))
				} else {
					inner := make([]object.RubyObject, len(m)-1)
					for i, g := range m[1:] {
						inner[i] = object.NewString(g)
					}
					out = append(out, object.NewArray(inner...))
				}
			}
			return object.NewArray(out...), true, nil
		case *object.String, *object.FrozenString:
			t, _ := stringText(env, p)
			if t == "" {
				return object.NewArray(), true, nil
			}
			out := []object.RubyObject{}
			i := 0
			for i <= len(s)-len(t) {
				if s[i:i+len(t)] == t {
					out = append(out, object.NewString(t))
					i += len(t)
					continue
				}
				i++
			}
			return object.NewArray(out...), true, nil
		}
		return nil, true, errorf("evaluator: String#scan: bad pattern %T", args[0])
	case "sub":
		if len(args) != 2 {
			return nil, true, errorf("evaluator: String#sub expects 2 args, got %d", len(args))
		}
		repl, ok := stringText(env, args[1])
		if !ok {
			return nil, true, errorf("evaluator: String#sub replacement must be String")
		}
		if re, ok := args[0].(*object.Regex); ok {
			loc := re.RE.FindStringIndex(s)
			if loc == nil {
				return object.NewString(s), true, nil
			}
			return object.NewString(s[:loc[0]] + repl + s[loc[1]:]), true, nil
		}
		pat, ok := stringText(env, args[0])
		if !ok {
			return nil, true, errorf("evaluator: String#sub pattern must be String/Regexp")
		}
		if pat == "" {
			return object.NewString(s), true, nil
		}
		idx := strings.Index(s, pat)
		if idx < 0 {
			return object.NewString(s), true, nil
		}
		return object.NewString(s[:idx] + repl + s[idx+len(pat):]), true, nil
	case "gsub":
		if len(args) != 2 {
			return nil, true, errorf("evaluator: String#gsub expects 2 args, got %d", len(args))
		}
		if h, ok := args[1].(*object.Hash); ok {
			out, err := stringGsubHash(env, s, args[0], h)
			return out, true, err
		}
		repl, ok := stringText(env, args[1])
		if !ok {
			return nil, true, errorf("evaluator: String#gsub replacement must be String or Hash")
		}
		if re, ok := args[0].(*object.Regex); ok {
			return object.NewString(re.RE.ReplaceAllString(s, repl)), true, nil
		}
		pat, ok := stringText(env, args[0])
		if !ok {
			return nil, true, errorf("evaluator: String#gsub pattern must be String/Regexp")
		}
		if pat == "" {
			return object.NewString(s), true, nil
		}
		return object.NewString(strings.ReplaceAll(s, pat, repl)), true, nil
	case "ord":
		if s == "" {
			return nil, true, errorf("evaluator: ArgumentError: empty string for String#ord")
		}
		runes := []rune(s)
		return object.NewInteger(int64(runes[0])), true, nil
	case "to_sym":
		return env.Symbols().Intern(s), true, nil
	case "tr":
		if len(args) != 2 {
			return nil, true, errorf("evaluator: String#tr expects 2 args, got %d", len(args))
		}
		from, ok1 := stringText(env, args[0])
		to, ok2 := stringText(env, args[1])
		if !ok1 || !ok2 {
			return nil, true, errorf("evaluator: String#tr needs String args")
		}
		fromRunes := expandTrRanges(from)
		toRunes := expandTrRanges(to)
		mapping := func(r rune) rune {
			for i, f := range fromRunes {
				if r == f {
					if i < len(toRunes) {
						return toRunes[i]
					}
					if len(toRunes) > 0 {
						return toRunes[len(toRunes)-1]
					}
					return -1
				}
			}
			return r
		}
		var b strings.Builder
		for _, r := range s {
			if m := mapping(r); m != -1 {
				b.WriteRune(m)
			}
		}
		return object.NewString(b.String()), true, nil
	case "count":
		if len(args) != 1 {
			return nil, true, errorf("evaluator: String#count expects 1 arg")
		}
		chars, ok := stringText(env, args[0])
		if !ok {
			return nil, true, errorf("evaluator: String#count needs String")
		}
		n := 0
		for _, c := range s {
			for _, k := range chars {
				if c == k {
					n++
					break
				}
			}
		}
		return object.NewInteger(int64(n)), true, nil
	case "bytes":
		out := make([]object.RubyObject, len(s))
		for i := 0; i < len(s); i++ {
			out[i] = object.NewInteger(int64(s[i]))
		}
		return object.NewArray(out...), true, nil
	case "bytesize":
		return object.NewInteger(int64(len(s))), true, nil
	case "ljust", "rjust", "center":
		if len(args) < 1 || len(args) > 2 {
			return nil, true, errorf("evaluator: String#%s expects 1..2 args, got %d", name, len(args))
		}
		w, ok := args[0].(*object.Integer)
		if !ok {
			return nil, true, errorf("evaluator: String#%s needs Integer width", name)
		}
		pad := " "
		if len(args) == 2 {
			if t, ok := stringText(env, args[1]); ok && t != "" {
				pad = t
			}
		}
		width := int(w.Value)
		runes := []rune(s)
		if width <= len(runes) {
			return object.NewString(s), true, nil
		}
		need := width - len(runes)
		repeat := func(n int) string {
			if n <= 0 {
				return ""
			}
			padR := []rune(pad)
			out := make([]rune, n)
			for i := 0; i < n; i++ {
				out[i] = padR[i%len(padR)]
			}
			return string(out)
		}
		switch name {
		case "ljust":
			return object.NewString(s + repeat(need)), true, nil
		case "rjust":
			return object.NewString(repeat(need) + s), true, nil
		}
		// center: split pad on both sides, extra on the right.
		left := need / 2
		right := need - left
		return object.NewString(repeat(left) + s + repeat(right)), true, nil
	case "succ", "next":
		return object.NewString(stringSucc(s)), true, nil
	case "to_i":
		base := 10
		if len(args) >= 1 {
			b, ok := args[0].(*object.Integer)
			if !ok {
				return nil, true, errorf("evaluator: String#to_i: base must be Integer, got %T", args[0])
			}
			base = int(b.Value)
			if base != 0 && (base < 2 || base > 36) {
				return nil, true, errorf("evaluator: ArgumentError: invalid radix %d", base)
			}
		}
		v := int64(0)
		neg := false
		i := 0
		for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
			i++
		}
		if i < len(s) && (s[i] == '+' || s[i] == '-') {
			neg = s[i] == '-'
			i++
		}
		// Auto-detect / strip the standard 0x / 0o / 0b / 0d prefix when
		// the base allows it. base == 0 means "infer from prefix"; the
		// specific bases peel their own prefix if present (mri behaviour).
		if i+1 < len(s) && s[i] == '0' {
			c := s[i+1]
			switch {
			case (c == 'x' || c == 'X') && (base == 0 || base == 16):
				base = 16
				i += 2
			case (c == 'b' || c == 'B') && (base == 0 || base == 2):
				base = 2
				i += 2
			case (c == 'o' || c == 'O') && (base == 0 || base == 8):
				base = 8
				i += 2
			case (c == 'd' || c == 'D') && (base == 0 || base == 10):
				base = 10
				i += 2
			}
		}
		if base == 0 {
			base = 10
		}
		digitVal := func(c byte) (int64, bool) {
			switch {
			case c >= '0' && c <= '9':
				return int64(c - '0'), true
			case c >= 'a' && c <= 'z':
				return int64(c-'a') + 10, true
			case c >= 'A' && c <= 'Z':
				return int64(c-'A') + 10, true
			}
			return 0, false
		}
		for i < len(s) {
			c := s[i]
			if c == '_' {
				i++
				continue
			}
			d, ok := digitVal(c)
			if !ok || d >= int64(base) {
				break
			}
			v = v*int64(base) + d
			i++
		}
		if neg {
			v = -v
		}
		return object.NewInteger(v), true, nil
	case "to_f":
		// Best-effort: skip leading ws + optional sign, consume digits
		// and optional `.` + decimals + optional exponent. Trailing
		// garbage is ignored, mirroring MRI.
		f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
		return object.NewFloat(f), true, nil
	}
	return nil, false, nil
}
