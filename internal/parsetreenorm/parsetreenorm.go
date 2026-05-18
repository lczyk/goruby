// Package parsetreenorm normalises the text output of
// `ruby --dump=parsetree` so trees from two source variants of the same
// program can be compared structurally, ignoring cosmetic and
// position-only differences.
//
// Normalisation rule: the transformed tree must evaluate identically to
// the original. Display-only fields (node IDs, line/location ranges,
// sibling indices, redundant counters, last-sibling markers, paren markers)
// and pure no-op wrappers (NODE_BEGIN(null), single-child NODE_BLOCK) get
// stripped. Anything that could change runtime behaviour stays.
package parsetreenorm

import (
	"regexp"
	"strings"
)

// Patterns covering cosmetic / positional noise across MRI parsetree
// formats (1.9 -> 4.0). Each MRI generation emits a slightly different
// format; the union of these regexes covers all observed cases without
// requiring per-version awareness, since callers only ever diff
// intra-version.
var (
	// Warning banner some MRIs prepend to --dump=parsetree output.
	reHeader = regexp.MustCompile(`(?s)^#+\n## Do NOT.*?\n#+\n+`)
	// __FILE__ / __dir__ substitution via SourceFileNode (prism) or
	// NODE_STR with the tempfile path: different tempfiles per run so the
	// path text differs even when the source is identical.
	rePrismSourceFile = regexp.MustCompile(`(?m)^.*\+-- filepath: ".*parsetree-[0-9]+\.rb".*$\n?`)
	reNdLitTempPath   = regexp.MustCompile(`(?m)^.*\+- nd_lit: ".*parsetree-[0-9]+\.rb".*$\n?`)
	// The tempfile path can also appear as a substring inside a larger
	// string literal (e.g. `__FILE__` interpolated, eval source banners
	// like `"(eval at /tmp/.../parsetree-NNN.rb:"`). Replace just the path
	// fragment with <TEMPFILE> to keep the rest of the literal intact.
	// Path may contain `/` but never `"` or `\n` -- so [^"\n]* up to the
	// `parsetree-NNN.rb` suffix matches the full path component safely.
	reTempPathSubstr = regexp.MustCompile(`/[^"\n]*parsetree-[0-9]+\.rb`)
)

// Normalize returns s with cosmetic / positional noise stripped and
// semantics-preserving no-op wrappers collapsed. Idempotent: Normalize ==
// Normalize . Normalize.
//
// Does NOT handle __LINE__ divergence -- callers that have the original
// source should use NormalizeWithSource instead.
func Normalize(s string) string {
	return normalize(s, nil, true)
}

// NormalizeWithSource is Normalize with extra __LINE__ awareness. src is
// the Ruby source that produced the parsetree dump. Lines containing the
// `__LINE__` pseudo-literal are detected and the NODE_LIT integer values
// at those lines are replaced with a placeholder, so two parsetrees from
// reformatted-but-equivalent sources compare equal even when their
// `__LINE__` evaluations differ.
func NormalizeWithSource(dump, src string) string {
	return normalize(dump, lineMagicLines(src), true)
}

// NormalizeRaw is Normalize without the final flat-form reparse. Output
// keeps the original indented tree shape (with cosmetic / positional
// noise and no-op wrappers stripped, but no path-prefixed flat lines).
// Useful when downstream consumers want to inspect / further process the
// tree structure directly.
func NormalizeRaw(s string) string {
	return normalize(s, nil, false)
}

// NormalizeWithSourceRaw is NormalizeWithSource without the flat-form
// reparse. See NormalizeRaw.
func NormalizeWithSourceRaw(dump, src string) string {
	return normalize(dump, lineMagicLines(src), false)
}

func normalize(s string, magic map[int]bool, flat bool) string {
	// Fixed-point loop: individual passes are not all mutually idempotent
	// when run once (e.g. unwrapSingleChildBlocks can expose new lines
	// starting with `#` that reLeadingHash would then strip). Iterating
	// guarantees the text-level pass converges before flattening.
	// normalizeOnce returns changed=false when no pass mutated s, avoiding
	// the O(n) equality compare on multi-MB dumps.
	for {
		next, changed := normalizeOnce(s, magic)
		if !changed {
			s = next
			break
		}
		s = next
	}
	if !flat {
		return s
	}
	// Final pass: reparse the indented tree and re-emit in flat path form,
	// one fact per line. Path-prefixed lines are diff-stable -- adding /
	// removing one node changes only contiguous lines instead of cascading
	// horizontal indent shifts across whole subtrees.
	return toFlat(s)
}

func normalizeOnce(s string, magic map[int]bool) (string, bool) {
	orig := s
	// Each pass is gated by a cheap substring check -- the regex engine is
	// ~60% of normalisation CPU and a full match-attempt on a multi-MB dump
	// dwarfs a strings.Contains scan. Triggers are necessary substrings of
	// any match the regex could find; absence means the regex can't match.
	if strings.Contains(s, "## Do NOT") {
		s = reHeader.ReplaceAllString(s, "")
	}
	if strings.HasPrefix(s, "#") || strings.Contains(s, "\n#") {
		s = stripLeadingHash(s)
	}
	// __LINE__ masking depends on (line: N) / (location: (L,C)-...) info
	// that the strip passes below remove, so it must run first.
	if len(magic) > 0 {
		s = maskLineMagic(s, magic)
	}
	// Outer gates are cheap SIMD-accelerated `strings.Contains` calls.
	// Skipping the helper's function-call + internal Contains is faster
	// than entering a no-op helper, especially on the idempotent path.
	if strings.Contains(s, "(location: ") {
		s = stripPrismLocation(s)
	}
	if strings.Contains(s, "(id: ") {
		s = stripIDLineLocation(s)
	}
	if strings.Contains(s, "(line: ") {
		s = stripLineLocation(s)
	}
	if strings.Contains(s, "_loc:") {
		s = stripPrismTokenLoc(s)
	}
	if strings.Contains(s, "*") {
		s = stripTrailingStars(s)
	}
	if strings.Contains(s, "nd_alen:") {
		s = stripNdAlen(s)
	}
	if strings.Contains(s, "+- nd_") {
		s = stripSiblingIndex(s)
	}
	if strings.Contains(s, "parsetree-") {
		if strings.Contains(s, "+-- filepath:") {
			s = rePrismSourceFile.ReplaceAllString(s, "")
		}
		if strings.Contains(s, "+- nd_lit:") {
			s = reNdLitTempPath.ReplaceAllString(s, "")
		}
		// Final pass: in-string substitutions that the line-strippers above
		// don't catch (path embedded in a larger literal).
		s = reTempPathSubstr.ReplaceAllString(s, "<TEMPFILE>")
	}
	if strings.Contains(s, "NODE_BEGIN") {
		s = stripNullBeginChildren(s)
	}
	if strings.Contains(s, "NODE_BLOCK") {
		s = unwrapSingleChildBlocks(s)
	}
	return s, s != orig
}

// stripLeadingHash removes leading `#`+ chars (and one optional following
// space) from each line. Equivalent to the regex `(?m)^#+ ?` but ~10x
// faster on multi-MB dumps -- single linear scan, single allocation when
// any line is modified, zero allocation when no line starts with `#`.
func stripLeadingHash(s string) string {
	// First pass: detect if any change is needed.
	needsWork := false
	if len(s) > 0 && s[0] == '#' {
		needsWork = true
	} else {
		for i := 0; i < len(s)-1; i++ {
			if s[i] == '\n' && s[i+1] == '#' {
				needsWork = true
				break
			}
		}
	}
	if !needsWork {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		// At start of a line. Strip any run of `#` and one optional space.
		if s[i] == '#' {
			for i < len(s) && s[i] == '#' {
				i++
			}
			if i < len(s) && s[i] == ' ' {
				i++
			}
		}
		// Copy through to and including next newline as one chunk.
		nl := strings.IndexByte(s[i:], '\n')
		if nl < 0 {
			b.WriteString(s[i:])
			break
		}
		b.WriteString(s[i : i+nl+1])
		i += nl + 1
	}
	return b.String()
}

// stripTrailingStars removes trailing `*` markers at end-of-line in two
// cases (per original reTrailingStar + reNodeNameStar regexes):
//   - `)*` -> `)`  (last-sibling marker, 2.6+)
//   - `@ NODE_<UPPER0-9_>+*` -> drop the `*` (paren marker)
//
// Hand-rolled byte scan: walk lines, only inspect those ending in `*`.
// Returns s unchanged when no qualifying line exists.
func stripTrailingStars(s string) string {
	// Quick reject: no `*` followed by newline / EOF.
	if !hasTrailingStar(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		// Find end-of-line.
		nl := strings.IndexByte(s[i:], '\n')
		var lineEnd int
		if nl < 0 {
			lineEnd = len(s)
		} else {
			lineEnd = i + nl
		}
		line := s[i:lineEnd]
		if len(line) > 0 && line[len(line)-1] == '*' {
			trimmed := trimTrailingStarLine(line)
			b.WriteString(trimmed)
		} else {
			b.WriteString(line)
		}
		if nl < 0 {
			break
		}
		b.WriteByte('\n')
		i = lineEnd + 1
	}
	return b.String()
}

// hasTrailingStar reports whether s contains a `*` immediately followed by
// `\n` or at end-of-input -- the only positions stripTrailingStars touches.
func hasTrailingStar(s string) bool {
	// Jump from `*` to `*` via IndexByte (SIMD-accelerated) instead of
	// scanning every byte. Cheap reject path for inputs lacking `*`.
	i := 0
	for i < len(s) {
		j := strings.IndexByte(s[i:], '*')
		if j < 0 {
			return false
		}
		j += i
		if j+1 == len(s) || s[j+1] == '\n' {
			return true
		}
		i = j + 1
	}
	return false
}

// trimTrailingStarLine returns line w/out its trailing `*` if it ends in
// `)*` or `@ NODE_<UPPER0-9_>+*`. Otherwise returns line unchanged.
func trimTrailingStarLine(line string) string {
	if len(line) < 2 {
		return line
	}
	// Case 1: `)*`
	if line[len(line)-2] == ')' {
		return line[:len(line)-1]
	}
	// Case 2: `@ NODE_<UPPER0-9_>+*`. Walk backwards from `*` over the name.
	i := len(line) - 2
	end := i
	for i >= 0 {
		c := line[i]
		if (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' {
			i--
			continue
		}
		break
	}
	// Need at least one name char.
	if end-i < 1 {
		return line
	}
	nameStart := i + 1
	// Must be preceded by `@ NODE_` (NODE_ is part of the name pattern, so
	// nameStart points to `N` of `NODE_X`). The regex requires `NODE_[A-Z0-9_]+`
	// -- at least one char AFTER `NODE_`, so the substring before `*` must be
	// strictly longer than `NODE_`.
	if !strings.HasPrefix(line[nameStart:], "NODE_") {
		return line
	}
	if len(line)-1-nameStart <= len("NODE_") {
		return line
	}
	// Need `@ ` immediately before nameStart.
	if nameStart < 2 || line[nameStart-2] != '@' || line[nameStart-1] != ' ' {
		return line
	}
	return line[:len(line)-1]
}

// stripIDLineLocation replaces matches of
//   ` (id: <digits>, line: <digits>[, (location|code_range): (D,D)-(D,D)])`
// with the empty string. Equivalent to the reLineIDLocation regex, but
// hand-rolled to skip the regex engine. Returns s unchanged when no match
// is found, avoiding the allocation entirely.
func stripIDLineLocation(s string) string {
	const trigger = " (id: "
	if !strings.Contains(s, trigger) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		j := strings.Index(s[i:], trigger)
		if j < 0 {
			b.WriteString(s[i:])
			break
		}
		j += i
		end, ok := matchIDLineLocation(s, j)
		if !ok {
			// Trigger present but pattern didn't match -- copy up to and past
			// the trigger's first char, retry from there.
			b.WriteString(s[i : j+1])
			i = j + 1
			continue
		}
		b.WriteString(s[i:j])
		i = end
	}
	return b.String()
}

// matchIDLineLocation tries to match the reLineIDLocation pattern starting
// at s[start] (which must point at the leading space). Returns the end
// offset (exclusive) and ok.
func matchIDLineLocation(s string, start int) (int, bool) {
	// " (id: "
	i := start + len(" (id: ")
	if i > len(s) {
		return 0, false
	}
	// digits
	d, ok := scanDigits(s, i)
	if !ok {
		return 0, false
	}
	i = d
	// ", line: "
	const lineTag = ", line: "
	if !strings.HasPrefix(s[i:], lineTag) {
		return 0, false
	}
	i += len(lineTag)
	d, ok = scanDigits(s, i)
	if !ok {
		return 0, false
	}
	i = d
	// optional ", (location|code_range): (N,N)-(N,N)"
	if strings.HasPrefix(s[i:], ", location: ") {
		i += len(", location: ")
		if i, ok = scanParenPair(s, i); !ok {
			return 0, false
		}
	} else if strings.HasPrefix(s[i:], ", code_range: ") {
		i += len(", code_range: ")
		if i, ok = scanParenPair(s, i); !ok {
			return 0, false
		}
	}
	// closing ")"
	if i >= len(s) || s[i] != ')' {
		return 0, false
	}
	return i + 1, true
}

// scanDigits advances past a run of one or more ASCII digits starting at i.
// Returns (next-index, true) on success.
func scanDigits(s string, i int) (int, bool) {
	start := i
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	return i, i > start
}

// scanParenPair advances past `(<no-rparen>)-(<no-rparen>)` starting at i.
// Mirrors the `\([^)]+\)-\([^)]+\)` regex fragment. Each `[^)]+` requires
// at least one non-`)` char, so empty `()` does NOT match.
func scanParenPair(s string, i int) (int, bool) {
	if i >= len(s) || s[i] != '(' {
		return 0, false
	}
	i++
	end := strings.IndexByte(s[i:], ')')
	if end < 1 {
		return 0, false
	}
	i += end + 1
	if i >= len(s) || s[i] != '-' {
		return 0, false
	}
	i++
	if i >= len(s) || s[i] != '(' {
		return 0, false
	}
	i++
	end = strings.IndexByte(s[i:], ')')
	if end < 1 {
		return 0, false
	}
	return i + end + 1, true
}

// stripSiblingIndex removes ` (N)` sibling-index suffixes between a
// `+- nd_<name>` field marker and the trailing `:`. Equivalent to the
// reSiblingIndex regex `(\+- nd_[a-z_]+) \(\d+\):` -> `$1:`.
func stripSiblingIndex(s string) string {
	const trigger = "+- nd_"
	if !strings.Contains(s, trigger) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		j := strings.Index(s[i:], trigger)
		if j < 0 {
			b.WriteString(s[i:])
			break
		}
		j += i
		// Scan field name: [a-z_]+ (require at least one char to match
		// the regex `[a-z_]+` -- empty name means no match).
		nameStart := j + len(trigger)
		k := nameStart
		for k < len(s) && ((s[k] >= 'a' && s[k] <= 'z') || s[k] == '_') {
			k++
		}
		if k == nameStart {
			b.WriteString(s[i : j+1])
			i = j + 1
			continue
		}
		// Expect " (<digits>):"
		if !strings.HasPrefix(s[k:], " (") {
			b.WriteString(s[i : j+1])
			i = j + 1
			continue
		}
		d, ok := scanDigits(s, k+2)
		if !ok || d >= len(s) || s[d] != ')' || d+1 >= len(s) || s[d+1] != ':' {
			b.WriteString(s[i : j+1])
			i = j + 1
			continue
		}
		// Match: copy up through the field name, skip ` (N)`, resume at `:`.
		b.WriteString(s[i:k])
		i = d + 1 // position at the `:`
	}
	return b.String()
}

// stripPrismLocation removes ` (location: (D,D)-(D,D))` substrings.
// Equivalent to the regex ` \(location: \([^)]+\)-\([^)]+\)\)`. Each
// `[^)]+` requires >= 1 char (no empty `()`).
func stripPrismLocation(s string) string {
	const trigger = " (location: "
	if !strings.Contains(s, trigger) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		j := strings.Index(s[i:], trigger)
		if j < 0 {
			b.WriteString(s[i:])
			break
		}
		j += i
		end, ok := scanParenPair(s, j+len(trigger))
		if !ok || end >= len(s) || s[end] != ')' {
			b.WriteString(s[i : j+1])
			i = j + 1
			continue
		}
		b.WriteString(s[i:j])
		i = end + 1
	}
	return b.String()
}

// stripLineLocation removes ` (line: D)` and
// ` (line: D, (location|code_range): (D,D)-(D,D))` substrings.
// Equivalent to the regex ` \(line: \d+(?:, (?:location|code_range): \([^)]+\)-\([^)]+\))?\)`.
func stripLineLocation(s string) string {
	const trigger = " (line: "
	if !strings.Contains(s, trigger) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		j := strings.Index(s[i:], trigger)
		if j < 0 {
			b.WriteString(s[i:])
			break
		}
		j += i
		end, ok := matchLineLocation(s, j)
		if !ok {
			b.WriteString(s[i : j+1])
			i = j + 1
			continue
		}
		b.WriteString(s[i:j])
		i = end
	}
	return b.String()
}

// matchLineLocation tries to match the reLineLocation pattern starting at
// s[start] (which must point at the leading space). Returns the end offset
// (exclusive) and ok.
func matchLineLocation(s string, start int) (int, bool) {
	i := start + len(" (line: ")
	if i > len(s) {
		return 0, false
	}
	d, ok := scanDigits(s, i)
	if !ok {
		return 0, false
	}
	i = d
	if strings.HasPrefix(s[i:], ", location: ") {
		i += len(", location: ")
		if i, ok = scanParenPair(s, i); !ok {
			return 0, false
		}
	} else if strings.HasPrefix(s[i:], ", code_range: ") {
		i += len(", code_range: ")
		if i, ok = scanParenPair(s, i); !ok {
			return 0, false
		}
	}
	if i >= len(s) || s[i] != ')' {
		return 0, false
	}
	return i + 1, true
}

// stripPrismTokenLoc drops matches of the regex
// `(?m)^.*_loc: (?:nil|\([^)]+\)-\([^)]+\) = "(?:[^"\\]|\\.)*")$\n?`. Match
// can span newlines because `[^)]` and `[^"\\]` accept `\n` in RE2 / Go's
// regexp. So we scan the whole string, not line-by-line.
func stripPrismTokenLoc(s string) string {
	if !strings.Contains(s, "_loc:") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		// `i` is at start of a line. Find the line's end (for greedy `.*`
		// rightmost-first search of `_loc: ` on this line) and try to
		// match starting from each candidate position on this line.
		nl := strings.IndexByte(s[i:], '\n')
		var lineEnd int
		if nl < 0 {
			lineEnd = len(s)
		} else {
			lineEnd = i + nl
		}
		if end, ok := tryMatchPrismTokenLoc(s, i, lineEnd); ok {
			// Match found. Skip up to end + optional trailing `\n`.
			i = end
			if i < len(s) && s[i] == '\n' {
				i++
			}
			continue
		}
		b.WriteString(s[i:lineEnd])
		if nl < 0 {
			break
		}
		b.WriteByte('\n')
		i = lineEnd + 1
	}
	return b.String()
}

// tryMatchPrismTokenLoc tries to find a match of
// `^.*_loc: (?:nil|\([^)]+\)-\([^)]+\) = "(?:[^"\\]|\\.)*")$`
// whose start is on the line s[lineStart:lineEnd]. Returns the offset
// where the match ends (`$` anchor position), exclusive of the optional
// trailing `\n`. Match body may extend past lineEnd if the string literal
// contains an unescaped newline.
func tryMatchPrismTokenLoc(s string, lineStart, lineEnd int) (int, bool) {
	tag := "_loc: "
	line := s[lineStart:lineEnd]
	// Try `_loc: ` positions on the line rightmost-first to mirror the
	// greedy `.*` preference.
	from := len(line)
	for {
		rel := strings.LastIndex(line[:from], tag)
		if rel < 0 {
			return 0, false
		}
		start := lineStart + rel + len(tag)
		if end, ok := matchPrismTokenLocSuffix(s, start); ok {
			return end, true
		}
		from = rel
		if from == 0 {
			return 0, false
		}
	}
}

// matchPrismTokenLocSuffix tries the suffix
// `(?:nil|\([^)]+\)-\([^)]+\) = "(?:[^"\\]|\\.)*")$` starting at s[start].
// Returns the end offset (where the `$` anchor must hold -- just before
// `\n` or at EOF).
func matchPrismTokenLocSuffix(s string, start int) (int, bool) {
	// Branch 1: `nil`.
	if strings.HasPrefix(s[start:], "nil") {
		end := start + 3
		if end == len(s) || s[end] == '\n' {
			return end, true
		}
	}
	// Branch 2: `\([^)]+\)-\([^)]+\) = "(?:[^"\\]|\\.)*"`.
	i, ok := scanParenPair(s, start)
	if !ok {
		return 0, false
	}
	if !strings.HasPrefix(s[i:], " = \"") {
		return 0, false
	}
	i += len(" = \"")
	for i < len(s) {
		c := s[i]
		if c == '"' {
			end := i + 1
			if end == len(s) || s[end] == '\n' {
				return end, true
			}
			return 0, false
		}
		if c == '\\' {
			if i+1 >= len(s) {
				return 0, false
			}
			i += 2
			continue
		}
		i++
	}
	return 0, false
}

// stripNdAlen drops whole lines containing `nd_alen:`. Equivalent to the
// regex `(?m)^.*\bnd_alen: .*$\n?`. The `\b` requires `nd_alen` to start
// at a word boundary -- a non-`[A-Za-z0-9_]` (or start of line) before `n`.
func stripNdAlen(s string) string {
	const tag = "nd_alen: "
	if !strings.Contains(s, tag) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		nl := strings.IndexByte(s[i:], '\n')
		var lineEnd int
		if nl < 0 {
			lineEnd = len(s)
		} else {
			lineEnd = i + nl
		}
		line := s[i:lineEnd]
		if lineHasWordBoundaryTag(line, tag) {
			if nl < 0 {
				break
			}
			i = lineEnd + 1
			continue
		}
		b.WriteString(line)
		if nl < 0 {
			break
		}
		b.WriteByte('\n')
		i = lineEnd + 1
	}
	return b.String()
}

// lineHasWordBoundaryTag reports whether line contains tag at a position
// preceded by a non-word char (or the start of the line) -- mirrors the
// regex `\b<tag>` semantics.
func lineHasWordBoundaryTag(line, tag string) bool {
	from := 0
	for from <= len(line) {
		j := strings.Index(line[from:], tag)
		if j < 0 {
			return false
		}
		j += from
		if j == 0 || !isWordByte(line[j-1]) {
			return true
		}
		from = j + 1
	}
	return false
}

func isWordByte(c byte) bool {
	return (c >= 'A' && c <= 'Z') ||
		(c >= 'a' && c <= 'z') ||
		(c >= '0' && c <= '9') ||
		c == '_'
}

// lineMagicLines returns the 1-indexed source lines that contain a
// `__LINE__` token. Cheap textual scan -- string literals / comments that
// contain the literal text `__LINE__` would also match, but those almost
// never overlap with a NODE_LIT at the same line so false positives are
// rare and harmless.
func lineMagicLines(src string) map[int]bool {
	if !strings.Contains(src, "__LINE__") {
		return nil
	}
	out := map[int]bool{}
	for i, l := range strings.Split(src, "\n") {
		if strings.Contains(l, "__LINE__") {
			out[i+1] = true
		}
	}
	return out
}

// maskLineMagic walks the dump and replaces nd_lit integer values that
// look like __LINE__ evaluations with a placeholder. A NODE_LIT at source
// line N whose nd_lit value is N is treated as `__LINE__` iff line N in
// src contains `__LINE__`.
func maskLineMagic(dump string, magic map[int]bool) string {
	// Gate: pattern requires `@ NODE_LIT` somewhere.
	if !strings.Contains(dump, "@ NODE_LIT") {
		return dump
	}
	const tag = "@ NODE_LIT"
	var b strings.Builder
	// Output is dump verbatim with at most a few short edits per match,
	// so initial capacity = len(dump) is a safe upper bound.
	written := 0
	for pos := 0; pos < len(dump); {
		// Find the next NODE_LIT header.
		off := strings.Index(dump[pos:], tag)
		if off < 0 {
			break
		}
		hdrStart := pos + off
		// Header occupies dump[hdrStart:hdrEnd]. Find end-of-line.
		nl := strings.IndexByte(dump[hdrStart:], '\n')
		var hdrEnd int
		if nl < 0 {
			hdrEnd = len(dump)
		} else {
			hdrEnd = hdrStart + nl
		}
		nodeLine, ok := parseNodeLitLineNo(dump[hdrStart:hdrEnd])
		if !ok || !magic[nodeLine] {
			pos = hdrEnd
			continue
		}
		// Scan up to ~3 following lines for `+- nd_lit: <int>`.
		litLineStart := hdrEnd
		if nl >= 0 {
			litLineStart++
		}
		litValueStart, litValueEnd, found := findNdLitInt(dump, litLineStart, 3)
		if !found {
			pos = hdrEnd
			continue
		}
		v, vok := atoiBytes(dump[litValueStart:litValueEnd])
		if !vok || v != nodeLine {
			pos = hdrEnd
			continue
		}
		// Match. Copy dump[written:litValueStart], emit placeholder,
		// resume at litValueEnd.
		if b.Cap() == 0 {
			b.Grow(len(dump))
		}
		b.WriteString(dump[written:litValueStart])
		b.WriteString("<__LINE__>")
		written = litValueEnd
		pos = litValueEnd
	}
	if written == 0 {
		return dump
	}
	b.WriteString(dump[written:])
	return b.String()
}

// parseNodeLitLineNo extracts the source line number from a NODE_LIT
// header line. Mirrors the regex
// `@ NODE_LIT\b[^\n]*?(?:line: (\d+)|location: \((\d+),)`. Returns the
// line number (group 1 or group 2) and ok.
func parseNodeLitLineNo(line string) (int, bool) {
	// Find earliest of `line: ` or `location: (`. Both are extracted.
	a := strings.Index(line, "line: ")
	bIdx := strings.Index(line, "location: (")
	// Pick whichever comes first AND is present.
	if a < 0 && bIdx < 0 {
		return 0, false
	}
	if a >= 0 && (bIdx < 0 || a < bIdx) {
		// `line: <digits>`
		start := a + len("line: ")
		end, ok := scanDigits(line, start)
		if !ok {
			return 0, false
		}
		return atoiBytes(line[start:end])
	}
	// `location: (<digits>,...`
	start := bIdx + len("location: (")
	end, ok := scanDigits(line, start)
	if !ok || end >= len(line) || line[end] != ',' {
		return 0, false
	}
	return atoiBytes(line[start:end])
}

// findNdLitInt scans up to maxLines lines starting at start, looking for
// a line matching `^.*\+- nd_lit: <digits>$`. Returns the byte offsets of
// the digit run and true on success.
func findNdLitInt(s string, start, maxLines int) (int, int, bool) {
	const tag = "+- nd_lit: "
	pos := start
	for k := 0; k < maxLines && pos < len(s); k++ {
		nl := strings.IndexByte(s[pos:], '\n')
		var lineEnd int
		if nl < 0 {
			lineEnd = len(s)
		} else {
			lineEnd = pos + nl
		}
		line := s[pos:lineEnd]
		// `^.*\+- nd_lit: \d+$` -- find rightmost tag occurrence whose
		// suffix is all digits to end-of-line. With greedy `.*`, rightmost
		// preferred but earlier positions are valid fallbacks.
		from := len(line)
		for {
			rel := strings.LastIndex(line[:from], tag)
			if rel < 0 {
				break
			}
			digitStart := pos + rel + len(tag)
			digitEnd, ok := scanDigits(s, digitStart)
			if ok && digitEnd == lineEnd {
				return digitStart, digitEnd, true
			}
			from = rel
			if from == 0 {
				break
			}
		}
		if nl < 0 {
			return 0, 0, false
		}
		pos = lineEnd + 1
	}
	return 0, 0, false
}

// atoiBytes parses a decimal int from an all-digit substring. Faster than
// strconv.Atoi for tiny inputs because it skips the sign/prefix machinery.
func atoiBytes(s string) (int, bool) {
	if len(s) == 0 {
		return 0, false
	}
	n := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	return n, true
}

// stripNullBeginChildren removes nd_head entries pointing to a NODE_BEGIN
// with a (null node) body. MRI 3.x wraps single-line def bodies
// (`def f(); body; end`) as NODE_BLOCK[NODE_BEGIN(null), body]; the BEGIN
// placeholder has no rescue clauses and an empty body, so the wrapped form
// evaluates identically to the unwrapped body.
//
// Pattern (4 consecutive lines, shared indent prefix; link char to the
// child is `|` for non-last siblings, ` ` for the last):
//
//	<prefix>+- nd_head:
//	<prefix>{|| }   @ NODE_BEGIN
//	<prefix>{|| }   +- nd_body:
//	<prefix>{|| }       (null node)
func stripNullBeginChildren(s string) string {
	// Pattern needs both NODE_BEGIN and (null node) to appear -- cheap
	// double-gate before any per-line work.
	if !strings.Contains(s, "(null node)") {
		return s
	}
	var b strings.Builder
	// Walk line-by-line via IndexByte. No []string materialization.
	written := 0
	pos := 0
	for pos < len(s) {
		l1Start := pos
		nl := strings.IndexByte(s[pos:], '\n')
		var l1End int
		if nl < 0 {
			l1End = len(s)
			pos = len(s)
		} else {
			l1End = pos + nl
			pos = l1End + 1
		}
		// Need full 4-line pattern: peek the next 3 line ends.
		l2End, l3End, l4End, ok := peekThreeLineEnds(s, pos)
		if !ok {
			continue
		}
		if !nullBeginMatch(s[l1Start:l1End], s[pos:l2End],
			s[l2End+1:l3End], s[l3End+1:l4End]) {
			continue
		}
		// Match: drop lines 1-4 inclusive of line 4's trailing newline.
		if b.Cap() == 0 {
			b.Grow(len(s))
		}
		b.WriteString(s[written:l1Start])
		if l4End < len(s) {
			written = l4End + 1
		} else {
			written = l4End
		}
		pos = written
	}
	if written == 0 {
		return s
	}
	b.WriteString(s[written:])
	return b.String()
}

// peekThreeLineEnds finds the end-of-line offsets for the next 3 lines
// starting at start. Returns (end of line 1, end of line 2, end of line 3,
// ok). All three lines must have terminating newlines.
func peekThreeLineEnds(s string, start int) (int, int, int, bool) {
	if start >= len(s) {
		return 0, 0, 0, false
	}
	n1 := strings.IndexByte(s[start:], '\n')
	if n1 < 0 {
		return 0, 0, 0, false
	}
	e1 := start + n1
	if e1+1 >= len(s) {
		return 0, 0, 0, false
	}
	n2 := strings.IndexByte(s[e1+1:], '\n')
	if n2 < 0 {
		return 0, 0, 0, false
	}
	e2 := e1 + 1 + n2
	if e2+1 >= len(s) {
		return 0, 0, 0, false
	}
	n3 := strings.IndexByte(s[e2+1:], '\n')
	if n3 < 0 {
		return 0, 0, 0, false
	}
	e3 := e2 + 1 + n3
	return e1, e2, e3, true
}

func nullBeginMatch(l1, l2, l3, l4 string) bool {
	const tag = "+- nd_head:"
	idx := strings.Index(l1, tag)
	if idx < 0 || l1[idx:] != tag {
		return false
	}
	prefix := l1[:idx]
	for _, lk := range []string{"|", " "} {
		if l2 == prefix+lk+"   @ NODE_BEGIN" &&
			l3 == prefix+lk+"   +- nd_body:" &&
			l4 == prefix+lk+"       (null node)" {
			return true
		}
	}
	return false
}

// unwrapSingleChildBlocks collapses NODE_BLOCK nodes containing exactly
// one nd_head child into that child directly. NODE_BLOCK is a statement
// sequence; a 1-element sequence evaluates identically to its element.
// Often arises after stripNullBeginChildren removes the NODE_BEGIN(null)
// placeholder.
//
// Multi-child NODE_BLOCKs (real statement sequences) are not touched.
func unwrapSingleChildBlocks(s string) string {
	for {
		next, changed := unwrapOneBlock(s)
		if !changed {
			return next
		}
		s = next
	}
}

func unwrapOneBlock(s string) (string, bool) {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		idx := strings.Index(l, "@ NODE_BLOCK")
		if idx < 0 || l[idx:] != "@ NODE_BLOCK" {
			continue
		}
		childCol := idx // children's `+-` aligns with the `@` column
		end := i + 1
		var childStarts []int
		for end < len(lines) {
			ln := lines[end]
			if len(ln) <= childCol || !strings.HasPrefix(ln[childCol:], "+- ") {
				if len(ln) > childCol && (ln[childCol] == '|' || ln[childCol] == ' ') {
					end++
					continue
				}
				break
			}
			tail := ln[childCol:]
			if !strings.HasPrefix(tail, "+- nd_head:") &&
				!strings.HasPrefix(tail, "+- nd_head ") {
				return s, false
			}
			childStarts = append(childStarts, end)
			end++
		}
		if len(childStarts) != 1 {
			continue
		}
		childRegionStart := childStarts[0] + 1
		childRegionEnd := end
		var dedented []string
		for j := childRegionStart; j < childRegionEnd; j++ {
			ln := lines[j]
			if len(ln) <= childCol+4 {
				dedented = append(dedented, ln)
				continue
			}
			dedented = append(dedented, ln[:childCol]+ln[childCol+4:])
		}
		out := make([]string, 0, len(lines)-2)
		out = append(out, lines[:i]...)
		out = append(out, dedented...)
		out = append(out, lines[end:]...)
		return strings.Join(out, "\n"), true
	}
	return s, false
}
