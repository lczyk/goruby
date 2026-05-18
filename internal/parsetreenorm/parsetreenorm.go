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
	"strconv"
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
	// Leading "# " on each line (1.9 through 3.2 wrap output in comments).
	// `#+` (not `#`) keeps the strip idempotent on synthetic multi-hash
	// inputs; real MRI output always emits a single `# `.
	reLeadingHash = regexp.MustCompile(`(?m)^#+ ?`)
	// Prism (3.4+) node header: "@ NodeName (location: (L,C)-(L,C))".
	rePrismLocation = regexp.MustCompile(` \(location: \([^)]+\)-\([^)]+\)\)`)
	// Pre-Prism (3.1/3.2) "(id: N, line: N, location: ...)" form.
	reLineIDLocation = regexp.MustCompile(` \(id: \d+, line: \d+(?:, (?:location|code_range): \([^)]+\)-\([^)]+\))?\)`)
	// 1.9 ... 3.0 "(line: N)" or "(line: N, location: ...)" or "(line: N, code_range: ...)".
	reLineLocation = regexp.MustCompile(` \(line: \d+(?:, (?:location|code_range): \([^)]+\)-\([^)]+\))?\)`)
	// Prism per-token _loc lines: "+-- foo_loc: nil" or "+-- foo_loc: (L,C)-(L,C) = \"literal\"".
	// Handles escaped quotes inside the literal.
	rePrismTokenLoc = regexp.MustCompile(`(?m)^.*_loc: (?:nil|\([^)]+\)-\([^)]+\) = "(?:[^"\\]|\\.)*")$\n?`)
	// Trailing "*" marker, two uses:
	//   - "<name>)*" on entry headers flags "last sibling" (2.6+)
	//   - "NODE_<X>*" flags the expression was parenthesised in source
	// Neither carries semantic info worth diffing on. Strip both. Parens
	// around an expression evaluate identically to the bare expression.
	reTrailingStar = regexp.MustCompile(`(?m)\)\*$`)
	reNodeNameStar = regexp.MustCompile(`(?m)(@ NODE_[A-Z0-9_]+)\*$`)
	// 1.9 nd_alen leaks an uninitialised value on the tail NODE_ARRAY entry;
	// drop nd_alen entirely (redundant with sibling count anyway).
	reNdAlen = regexp.MustCompile(`(?m)^.*\bnd_alen: .*$\n?`)
	// 3.x+ adds " (N)" sibling-index suffix on chained child links
	// (e.g. "+- nd_head (1):"). Index derivable from emission order.
	reSiblingIndex = regexp.MustCompile(`(\+- nd_[a-z_]+) \(\d+\):`)
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
	return normalize(s, nil)
}

// NormalizeWithSource is Normalize with extra __LINE__ awareness. src is
// the Ruby source that produced the parsetree dump. Lines containing the
// `__LINE__` pseudo-literal are detected and the NODE_LIT integer values
// at those lines are replaced with a placeholder, so two parsetrees from
// reformatted-but-equivalent sources compare equal even when their
// `__LINE__` evaluations differ.
func NormalizeWithSource(dump, src string) string {
	return normalize(dump, lineMagicLines(src))
}

func normalize(s string, magic map[int]bool) string {
	// Fixed-point loop: individual passes are not all mutually idempotent
	// when run once (e.g. unwrapSingleChildBlocks can expose new lines
	// starting with `#` that reLeadingHash would then strip). Iterating
	// guarantees the text-level pass converges before flattening.
	for {
		next := normalizeOnce(s, magic)
		if next == s {
			break
		}
		s = next
	}
	// Final pass: reparse the indented tree and re-emit in flat path form,
	// one fact per line. Path-prefixed lines are diff-stable -- adding /
	// removing one node changes only contiguous lines instead of cascading
	// horizontal indent shifts across whole subtrees.
	return toFlat(s)
}

func normalizeOnce(s string, magic map[int]bool) string {
	// Each pass is gated by a cheap substring check -- the regex engine is
	// ~60% of normalisation CPU and a full match-attempt on a multi-MB dump
	// dwarfs a strings.Contains scan. Triggers are necessary substrings of
	// any match the regex could find; absence means the regex can't match.
	if strings.Contains(s, "## Do NOT") {
		s = reHeader.ReplaceAllString(s, "")
	}
	if strings.HasPrefix(s, "#") || strings.Contains(s, "\n#") {
		s = reLeadingHash.ReplaceAllString(s, "")
	}
	// __LINE__ masking depends on (line: N) / (location: (L,C)-...) info
	// that the strip passes below remove, so it must run first.
	if len(magic) > 0 {
		s = maskLineMagic(s, magic)
	}
	if strings.Contains(s, "(location: ") {
		s = rePrismLocation.ReplaceAllString(s, "")
	}
	if strings.Contains(s, "(id: ") {
		s = reLineIDLocation.ReplaceAllString(s, "")
	}
	if strings.Contains(s, "(line: ") {
		s = reLineLocation.ReplaceAllString(s, "")
	}
	if strings.Contains(s, "_loc:") {
		s = rePrismTokenLoc.ReplaceAllString(s, "")
	}
	if strings.Contains(s, ")*") {
		s = reTrailingStar.ReplaceAllString(s, ")")
	}
	if strings.Contains(s, "*") {
		s = reNodeNameStar.ReplaceAllString(s, "$1")
	}
	if strings.Contains(s, "nd_alen:") {
		s = reNdAlen.ReplaceAllString(s, "")
	}
	if strings.Contains(s, "+- nd_") {
		s = reSiblingIndex.ReplaceAllString(s, "$1:")
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
	return s
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

// reNodeLitHeader extracts the source line from a NODE_LIT header, across
// all known MRI dump formats. Group 1 (pre-Prism `line: N`) or group 2
// (Prism `location: (L,...`) holds the line number.
var reNodeLitHeader = regexp.MustCompile(`@ NODE_LIT\b[^\n]*?(?:line: (\d+)|location: \((\d+),)`)

// reNdLitInt matches a `+- nd_lit: <int>` value line. Captures the int.
var reNdLitInt = regexp.MustCompile(`^(.*\+- nd_lit: )(\d+)$`)

// maskLineMagic walks the dump and replaces nd_lit integer values that
// look like __LINE__ evaluations with a placeholder. A NODE_LIT at source
// line N whose nd_lit value is N is treated as `__LINE__` iff line N in
// src contains `__LINE__`.
func maskLineMagic(dump string, magic map[int]bool) string {
	lines := strings.Split(dump, "\n")
	for i, l := range lines {
		m := reNodeLitHeader.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		var nodeLine int
		for _, g := range m[1:] {
			if g != "" {
				n, err := strconv.Atoi(g)
				if err != nil {
					continue
				}
				nodeLine = n
				break
			}
		}
		if nodeLine == 0 || !magic[nodeLine] {
			continue
		}
		// Find the next `+- nd_lit: <int>` line owned by this NODE_LIT.
		// NODE_LIT has only nd_lit as its content field, so the immediate
		// next non-empty line carrying `+- nd_lit:` is ours.
		for j := i + 1; j < len(lines) && j < i+4; j++ {
			mm := reNdLitInt.FindStringSubmatch(lines[j])
			if mm == nil {
				continue
			}
			v, err := strconv.Atoi(mm[2])
			if err != nil {
				break
			}
			if v == nodeLine {
				lines[j] = mm[1] + "<__LINE__>"
			}
			break
		}
	}
	return strings.Join(lines, "\n")
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
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for i := 0; i < len(lines); i++ {
		if i+3 < len(lines) {
			if nullBeginMatch(lines[i], lines[i+1], lines[i+2], lines[i+3]) {
				i += 3 // skip all 4 lines
				continue
			}
		}
		out = append(out, lines[i])
	}
	return strings.Join(out, "\n")
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
