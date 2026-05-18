package parsetreenorm

import (
	"regexp"
	"strings"
	"testing"
)

// Oracle regexes preserved here for differential fuzz only. The production
// byte-scan helpers (stripLeadingHash / stripIDLineLocation /
// stripSiblingIndex / stripTrailingStars) replaced these for performance;
// any output divergence between scanner and oracle is a bug.
var (
	oracleLeadingHash    = regexp.MustCompile(`(?m)^#+ ?`)
	oracleIDLineLocation = regexp.MustCompile(` \(id: \d+, line: \d+(?:, (?:location|code_range): \([^)]+\)-\([^)]+\))?\)`)
	oracleSiblingIndex   = regexp.MustCompile(`(\+- nd_[a-z_]+) \(\d+\):`)
	oracleTrailingStar   = regexp.MustCompile(`(?m)\)\*$`)
	oracleNodeNameStar   = regexp.MustCompile(`(?m)(@ NODE_[A-Z0-9_]+)\*$`)
)

func oracleStripTrailingStars(s string) string {
	s = oracleTrailingStar.ReplaceAllString(s, ")")
	s = oracleNodeNameStar.ReplaceAllString(s, "$1")
	return s
}

// --- differential fuzz: scanner vs regex oracle -----------------------------

func FuzzStripLeadingHashOracle(f *testing.F) {
	for _, s := range commonSeeds {
		f.Add(s)
	}
	f.Add("# hello\n## world\n")
	f.Add("#\n#\n##\n")
	f.Add("no hash\nplain\n")
	f.Add("#trailing-nonl")
	f.Fuzz(func(t *testing.T, s string) {
		got := stripLeadingHash(s)
		want := oracleLeadingHash.ReplaceAllString(s, "")
		if got != want {
			t.Fatalf("stripLeadingHash diverges for %q\nscan:  %q\nregex: %q", s, got, want)
		}
	})
}

func FuzzStripIDLineLocationOracle(f *testing.F) {
	for _, s := range commonSeeds {
		f.Add(s)
	}
	f.Add(" (id: 1, line: 2)")
	f.Add(" (id: 5, line: 12, location: (1,0)-(1,2))")
	f.Add(" (id: 9, line: 9, code_range: (1,0)-(2,3))")
	f.Add(" (id: not-digits)")
	f.Add(" (id: 1, line: 2, location: ((nested))-(x))")
	f.Fuzz(func(t *testing.T, s string) {
		got := stripIDLineLocation(s)
		want := oracleIDLineLocation.ReplaceAllString(s, "")
		if got != want {
			t.Fatalf("stripIDLineLocation diverges for %q\nscan:  %q\nregex: %q", s, got, want)
		}
	})
}

func FuzzStripSiblingIndexOracle(f *testing.F) {
	for _, s := range commonSeeds {
		f.Add(s)
	}
	f.Add("+- nd_head (1):\n")
	f.Add("+- nd_body (42):\n")
	f.Add("+- nd_foo (x):\n")  // non-digits inside parens
	f.Add("+- nd_head:")       // missing ` (N):` suffix
	f.Add("+- nd_head (1)")    // missing trailing colon
	f.Add("  +- nd_a_b_c (7):\n+- nd_x (0):\n")
	f.Fuzz(func(t *testing.T, s string) {
		got := stripSiblingIndex(s)
		want := oracleSiblingIndex.ReplaceAllString(s, "$1:")
		if got != want {
			t.Fatalf("stripSiblingIndex diverges for %q\nscan:  %q\nregex: %q", s, got, want)
		}
	})
}

func FuzzStripTrailingStarsOracle(f *testing.F) {
	for _, s := range commonSeeds {
		f.Add(s)
	}
	f.Add("@ NODE_X*\n")
	f.Add("@ NODE_FOO_BAR42*\n")
	f.Add("something)*\n")
	f.Add("not stripped *\n")  // bare `*` after space
	f.Add("@ NODE_X)*\n")      // both markers on same line
	f.Add("@ NODE_X*")         // no trailing newline
	f.Add("@ Node*\n")         // not NODE_<UPPER>, leave alone
	f.Fuzz(func(t *testing.T, s string) {
		got := stripTrailingStars(s)
		want := oracleStripTrailingStars(s)
		if got != want {
			t.Fatalf("stripTrailingStars diverges for %q\nscan:  %q\nregex: %q", s, got, want)
		}
	})
}

// commonSeeds is a small pool of realistic dump fragments reused across
// every oracle-fuzz target so the corpus shares discovered interesting
// shapes between targets.
var commonSeeds = []string{
	"",
	"\n",
	"# @ NODE_LIT (id: 5, line: 2, location: (1,0)-(1,2))\n# +- nd_lit: 42\n",
	"@ NODE_BLOCK\n+- nd_head (1):\n|   @ NODE_LIT\n",
	"@ ProgramNode (location: (1,0)-(2,3))\n+-- value_loc: (1,0)-(1,5) = \"x\"\n",
	"@ NODE_DEFN (id: 1, line: 1, location: (1,0)-(4,3))*\n",
}

// --- enriched seeds + output-invariants on top-level Normalize fuzz ---------

// FuzzNormalizeWellFormed extends the existing idempotency fuzz with
// shape-preserving post-conditions: after Normalize, no line should start
// with `#` (leading-hash pass converged) and no line should end with `)*`
// (trailing-star pass converged). Both invariants hold unconditionally
// once the fixed-point loop terminates -- a violation means a strip
// pass's gate or scanner failed to converge.
func FuzzNormalizeWellFormed(f *testing.F) {
	// Format variants -- one seed per major dump generation.
	f.Add(sampleDump3x)
	f.Add(samplePrism34)
	f.Add(sample2x)
	// Specific shapes worth exploring.
	f.Add("@ NODE_BLOCK\n+- nd_head:\n|   @ NODE_LIT\n+- nd_head:\n    @ NODE_LIT\n") // multi-child BLOCK
	f.Add("@ NODE_BLOCK\n+- nd_head:\n    @ NODE_BEGIN\n    +- nd_body:\n        (null node)\n") // last-sibling NODE_BEGIN
	f.Add("@ NODE_STR\n+- nd_lit: \"/tmp/parsetree-7.rb\"\n") // tempfile path
	f.Add("@ NODE_ARRAY\n+- nd_alen: 99\n+- nd_head (1):\n|   @ NODE_LIT\n") // nd_alen + sibling-index
	f.Add("")

	f.Fuzz(func(t *testing.T, dump string) {
		out := Normalize(dump)
		// Idempotency (mirrors FuzzNormalize for redundancy).
		if out2 := Normalize(out); out != out2 {
			t.Fatalf("not idempotent:\n--- once ---\n%q\n--- twice ---\n%q", out, out2)
		}
		// Post-conditions: every line must satisfy these after the
		// fixed-point loop converges.
		for ln := range strings.SplitSeq(out, "\n") {
			if len(ln) > 0 && ln[0] == '#' {
				t.Fatalf("line still starts with `#` after Normalize: %q\nfull:\n%s", ln, out)
			}
			if strings.HasSuffix(ln, ")*") {
				t.Fatalf("line still ends with `)*` after Normalize: %q\nfull:\n%s", ln, out)
			}
		}
	})
}
