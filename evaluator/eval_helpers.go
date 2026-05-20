package evaluator

import (
	"regexp"
	"strings"

	"github.com/lczyk/goruby/object"
)

// stringGsubHash implements `s.gsub(pattern, hash)`: for each match,
// look the matched text up in hash. Missing keys fall through to the
// hash's default block (Hash.new { |h,k| ... }), then its plain default
// value, then "" per MRI.
func stringGsubHash(env *object.Environment, s string, pat object.RubyObject, h *object.Hash) (object.RubyObject, error) {
	var blockErr error
	replace := func(match string) string {
		if blockErr != nil {
			return ""
		}
		for _, e := range h.Entries {
			if k, ok := e.Key.(*object.String); ok && k.Value() == match {
				if rs, ok := stringText(env, e.Value); ok {
					return rs
				}
			}
		}
		if h.DefaultBlock != nil {
			if p, ok := h.DefaultBlock.(*object.Proc); ok {
				v, err := invokeProc(env, p, []object.RubyObject{h, object.NewString(match)})
				if err != nil {
					blockErr = err
					return ""
				}
				if rs, ok := stringText(env, v); ok {
					return rs
				}
			}
		}
		if h.Default != nil {
			if rs, ok := stringText(env, h.Default); ok {
				return rs
			}
		}
		return ""
	}
	if re, ok := pat.(*object.Regex); ok {
		out := re.RE.ReplaceAllStringFunc(s, replace)
		if blockErr != nil {
			return nil, blockErr
		}
		return object.NewString(out), nil
	}
	p, ok := stringText(env, pat)
	if !ok {
		return nil, errorf("evaluator: String#gsub: pattern must be String/Regexp")
	}
	if p == "" {
		return object.NewString(s), nil
	}
	out := strings.ReplaceAll(s, p, replace(p))
	if blockErr != nil {
		return nil, blockErr
	}
	return object.NewString(out), nil
}

// stringGsubBlock implements `s.gsub(pattern) { |m| ... }`: each match
// is passed to the block and replaced with the block's String return.
func stringGsubBlock(env *object.Environment, s string, pat object.RubyObject, invoke blockCallback) (object.RubyObject, error) {
	var blockErr error
	repl := func(match string) string {
		if blockErr != nil {
			return match
		}
		v, err := invoke([]object.RubyObject{object.NewString(match)})
		if err != nil {
			blockErr = err
			return match
		}
		if rs, ok := stringText(env, v); ok {
			return rs
		}
		return ""
	}
	out := stringPatternReplace(env, s, pat, repl)
	return out, blockErr
}

// stringSubBlock implements `s.sub(pattern) { |m| ... }`: replaces the
// first match only.
func stringSubBlock(env *object.Environment, s string, pat object.RubyObject, invoke blockCallback) (object.RubyObject, error) {
	if re, ok := pat.(*object.Regex); ok {
		loc := re.RE.FindStringIndex(s)
		if loc == nil {
			return object.NewString(s), nil
		}
		v, err := invoke([]object.RubyObject{object.NewString(s[loc[0]:loc[1]])})
		if err != nil {
			return nil, err
		}
		rs, _ := stringText(env, v)
		return object.NewString(s[:loc[0]] + rs + s[loc[1]:]), nil
	}
	p, ok := stringText(env, pat)
	if !ok {
		return nil, errorf("evaluator: String#sub: pattern must be String/Regexp")
	}
	if p == "" {
		return object.NewString(s), nil
	}
	idx := strings.Index(s, p)
	if idx < 0 {
		return object.NewString(s), nil
	}
	v, err := invoke([]object.RubyObject{object.NewString(p)})
	if err != nil {
		return nil, err
	}
	rs, _ := stringText(env, v)
	return object.NewString(s[:idx] + rs + s[idx+len(p):]), nil
}

// expandTrRanges expands `A-Z`-style range specifiers inside a
// String#tr argument into the explicit rune list. Bare `-` at the
// start or end is treated literally. Used by String#tr to match MRI's
// semantics where `'A-Za-z'` means the 52 alphabetic runes.
func expandTrRanges(s string) []rune {
	runes := []rune(s)
	out := []rune{}
	for i := 0; i < len(runes); i++ {
		// Range only valid when `-` is not the first or last char and
		// the bounding runes form a valid forward range.
		if runes[i] == '-' && i > 0 && i+1 < len(runes) && runes[i-1] <= runes[i+1] {
			start := runes[i-1] + 1
			end := runes[i+1]
			for r := start; r <= end; r++ {
				out = append(out, r)
			}
			i++ // skip the upper bound, already consumed
			continue
		}
		out = append(out, runes[i])
	}
	return out
}

// stringPatternReplace runs repl across every match of pat in s (regex
// or literal). Used by the gsub block path; the hash path has its own
// shortcut because the per-match cost is a hash lookup, not a callback.
func stringPatternReplace(env *object.Environment, s string, pat object.RubyObject, repl func(string) string) *object.String {
	if re, ok := pat.(*object.Regex); ok {
		return object.NewString(re.RE.ReplaceAllStringFunc(s, repl))
	}
	if p, ok := stringText(env, pat); ok && p != "" {
		// Literal string pattern: behave like gsub does on a fixed string.
		// Build a non-overlapping regex by escaping p.
		re := regexp.MustCompile(regexp.QuoteMeta(p))
		return object.NewString(re.ReplaceAllStringFunc(s, repl))
	}
	return object.NewString(s)
}
