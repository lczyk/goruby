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
	// Leading "# " on each line (1.9 through 3.2 wrap output in comments).
	reLeadingHash = regexp.MustCompile(`(?m)^# ?`)
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
)

// Normalize returns s with cosmetic / positional noise stripped and
// semantics-preserving no-op wrappers collapsed. Idempotent: Normalize ==
// Normalize . Normalize.
func Normalize(s string) string {
	s = reHeader.ReplaceAllString(s, "")
	s = reLeadingHash.ReplaceAllString(s, "")
	s = rePrismLocation.ReplaceAllString(s, "")
	s = reLineIDLocation.ReplaceAllString(s, "")
	s = reLineLocation.ReplaceAllString(s, "")
	s = rePrismTokenLoc.ReplaceAllString(s, "")
	s = reTrailingStar.ReplaceAllString(s, ")")
	s = reNodeNameStar.ReplaceAllString(s, "$1")
	s = reNdAlen.ReplaceAllString(s, "")
	s = reSiblingIndex.ReplaceAllString(s, "$1:")
	s = rePrismSourceFile.ReplaceAllString(s, "")
	s = reNdLitTempPath.ReplaceAllString(s, "")
	s = stripNullBeginChildren(s)
	s = unwrapSingleChildBlocks(s)
	return s
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
