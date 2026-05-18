package parsetreenorm

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeIdempotent(t *testing.T) {
	cases := []string{
		"",
		"@ NODE_LIT\n+- nd_lit: 42\n",
		sampleDump3x,
	}
	for i, in := range cases {
		once := Normalize(in)
		twice := Normalize(once)
		if once != twice {
			t.Errorf("case %d: not idempotent\n--- once ---\n%s\n--- twice ---\n%s", i, once, twice)
		}
	}
}

func TestStripsHeader(t *testing.T) {
	in := "###########################################################\n" +
		"## Do NOT use this node dump for any purpose other than  ##\n" +
		"## debug and research.  Compatibility is not guaranteed. ##\n" +
		"###########################################################\n\n" +
		"# @ NODE_LIT\n# +- nd_lit: 42\n"
	out := Normalize(in)
	if strings.Contains(out, "Do NOT use") {
		t.Errorf("header banner not stripped:\n%s", out)
	}
	if strings.Contains(out, "# @") {
		t.Errorf("leading hash not stripped:\n%s", out)
	}
}

func TestStripsLineAndID(t *testing.T) {
	in := "# @ NODE_LIT (id: 5, line: 12, location: (1,0)-(1,2))\n# +- nd_lit: 42\n"
	out := Normalize(in)
	if strings.Contains(out, "line:") || strings.Contains(out, "id:") {
		t.Errorf("line/id not stripped:\n%s", out)
	}
}

func TestStripsSiblingIndex(t *testing.T) {
	in := "@ NODE_BLOCK\n+- nd_head (1):\n|   @ NODE_LIT\n+- nd_head (2):\n    @ NODE_LIT\n"
	out := Normalize(in)
	if strings.Contains(out, "(1)") || strings.Contains(out, "(2)") {
		t.Errorf("sibling index not stripped:\n%s", out)
	}
}

func TestStripsNullBegin(t *testing.T) {
	in := "@ NODE_BLOCK\n" +
		"+- nd_head:\n" +
		"|   @ NODE_BEGIN\n" +
		"|   +- nd_body:\n" +
		"|       (null node)\n" +
		"+- nd_head:\n" +
		"    @ NODE_LIT\n" +
		"    +- nd_lit: 42\n"
	out := Normalize(in)
	// After stripping NODE_BEGIN(null) the wrapping NODE_BLOCK has one
	// remaining child and gets unwrapped too.
	if strings.Contains(out, "NODE_BEGIN") {
		t.Errorf("NODE_BEGIN(null) not stripped:\n%s", out)
	}
	if strings.Contains(out, "NODE_BLOCK") {
		t.Errorf("single-child NODE_BLOCK not unwrapped:\n%s", out)
	}
	if !strings.Contains(out, "NODE_LIT") || !strings.Contains(out, "nd_lit = 42") {
		t.Errorf("body content lost:\n%s", out)
	}
}

func TestMaskLineMagic(t *testing.T) {
	src := "x = 1\n__LINE__\n"
	// NODE_LIT at line 2 (where __LINE__ lives), value 2 -> mask.
	dump := "# @ NODE_LIT (line: 2)\n# +- nd_lit: 2\n"
	out := NormalizeWithSource(dump, src)
	if !strings.Contains(out, "<__LINE__>") {
		t.Errorf("__LINE__ at matching line not masked:\n%s", out)
	}
	if strings.Contains(out, "nd_lit: 2") {
		t.Errorf("original value still present:\n%s", out)
	}
}

func TestMaskLineMagicNoFalsePositive(t *testing.T) {
	src := "x = 1\nputs 42\n"
	// NODE_LIT at line 2, value 2. Line 2 has no __LINE__ -> do NOT mask.
	dump := "# @ NODE_LIT (line: 2)\n# +- nd_lit: 2\n"
	out := NormalizeWithSource(dump, src)
	if strings.Contains(out, "<__LINE__>") {
		t.Errorf("non-__LINE__ value wrongly masked:\n%s", out)
	}
}

func TestMaskLineMagicValueMismatch(t *testing.T) {
	// __LINE__ on src line 2 but the NODE_LIT at line 2 has a different
	// value -- e.g. literal `99` on the same line. Don't mask.
	src := "x = 1\n__LINE__; puts 99\n"
	dump := "# @ NODE_LIT (line: 2)\n# +- nd_lit: 99\n"
	out := NormalizeWithSource(dump, src)
	if strings.Contains(out, "<__LINE__>") {
		t.Errorf("value-mismatched NODE_LIT wrongly masked:\n%s", out)
	}
}

func TestKeepsMultiChildBlock(t *testing.T) {
	in := "@ NODE_BLOCK\n" +
		"+- nd_head:\n" +
		"|   @ NODE_LIT\n" +
		"|   +- nd_lit: 1\n" +
		"+- nd_head:\n" +
		"    @ NODE_LIT\n" +
		"    +- nd_lit: 2\n"
	out := Normalize(in)
	if !strings.Contains(out, "NODE_BLOCK") {
		t.Errorf("multi-child NODE_BLOCK should NOT be unwrapped:\n%s", out)
	}
}

func TestMaskLineMagicPrismFormat(t *testing.T) {
	src := "x = 1\n__LINE__\n"
	dump := "@ NODE_LIT (location: (2,0)-(2,8))\n+- nd_lit: 2\n"
	out := NormalizeWithSource(dump, src)
	if !strings.Contains(out, "<__LINE__>") {
		t.Errorf("Prism-format __LINE__ not masked:\n%s", out)
	}
}

func TestMaskLineMagicPreIDFormat(t *testing.T) {
	src := "x = 1\n__LINE__\n"
	dump := "@ NODE_LIT (id: 5, line: 2, location: (2,0)-(2,8))\n+- nd_lit: 2\n"
	out := NormalizeWithSource(dump, src)
	if !strings.Contains(out, "<__LINE__>") {
		t.Errorf("(id, line, location) format __LINE__ not masked:\n%s", out)
	}
}

func TestMaskLineMagicNoNdLitFollows(t *testing.T) {
	// NODE_LIT header but no nd_lit field within window -- bail out cleanly.
	src := "__LINE__\n"
	dump := "@ NODE_LIT (line: 1)\n" +
		"# noise\n# noise\n# noise\n+- nd_lit: 1\n"
	out := NormalizeWithSource(dump, src)
	if strings.Contains(out, "<__LINE__>") {
		t.Errorf("masked despite nd_lit beyond lookahead window:\n%s", out)
	}
}

func TestUnwrapBlockWithNonHeadField(t *testing.T) {
	// NODE_BLOCK with a non-nd_head child field -- don't unwrap, just keep.
	in := "@ NODE_BLOCK\n" +
		"+- nd_unexpected:\n" +
		"    @ NODE_LIT\n"
	out := Normalize(in)
	if !strings.Contains(out, "NODE_BLOCK") {
		t.Errorf("unexpected-field NODE_BLOCK wrongly unwrapped:\n%s", out)
	}
}

func TestStripNullBeginLastSibling(t *testing.T) {
	// NODE_BEGIN(null) as the LAST sibling (uses ` ` connector not `|`).
	in := "@ NODE_BLOCK\n" +
		"+- nd_head:\n" +
		"|   @ NODE_LIT\n" +
		"|   +- nd_lit: 1\n" +
		"+- nd_head:\n" +
		"    @ NODE_BEGIN\n" +
		"    +- nd_body:\n" +
		"        (null node)\n"
	out := Normalize(in)
	if strings.Contains(out, "NODE_BEGIN") {
		t.Errorf("last-sibling NODE_BEGIN(null) not stripped:\n%s", out)
	}
}

func TestPrismListField(t *testing.T) {
	// Prism `(length: N)` list field: parent has one `+-- name:` header
	// followed by N inline `+-- @ ChildNode` entries at bodyDepth.
	// Flat output should emit N separate `name[i]` lines.
	in := "@ NODE_SCOPE\n" +
		"+-- statements: (length: 2)\n" +
		"    +-- @ NODE_LIT\n" +
		"    |   +- nd_lit: 1\n" +
		"    +-- @ NODE_LIT\n" +
		"        +- nd_lit: 2\n"
	out := Normalize(in)
	if !strings.Contains(out, "statements[0]") || !strings.Contains(out, "statements[1]") {
		t.Errorf("list field not expanded to indexed entries:\n%s", out)
	}
}

func TestLineMagicLinesEmptyShortcut(t *testing.T) {
	if got := lineMagicLines("x = 1\nputs y\n"); got != nil {
		t.Errorf("non-nil map for source w/out __LINE__: %v", got)
	}
}

func FuzzNormalize(f *testing.F) {
	f.Add("")
	f.Add(sampleDump3x)
	f.Add("@ NODE_BLOCK\n+- nd_head:\n|   @ NODE_BEGIN\n|   +- nd_body:\n|       (null node)\n")
	f.Add("@ NODE_LIT (line: 5)\n+- nd_lit: 5\n")
	f.Fuzz(func(t *testing.T, dump string) {
		// Should never panic, and must be idempotent.
		out1 := Normalize(dump)
		out2 := Normalize(out1)
		if out1 != out2 {
			t.Fatalf("not idempotent:\n--- once ---\n%q\n--- twice ---\n%q", out1, out2)
		}
	})
}

func FuzzNormalizeWithSource(f *testing.F) {
	f.Add("", "")
	f.Add(sampleDump3x, "x = 1\n__LINE__\n")
	f.Add("@ NODE_LIT (line: 1)\n+- nd_lit: 1\n", "__LINE__")
	f.Fuzz(func(t *testing.T, dump, src string) {
		out1 := NormalizeWithSource(dump, src)
		out2 := NormalizeWithSource(out1, src)
		if out1 != out2 {
			t.Fatalf("not idempotent for src=%q\n--- once ---\n%q\n--- twice ---\n%q", src, out1, out2)
		}
	})
}

// TestNormalizeOnRealFixture exercises the full pipeline against a real
// MRI parsetree dump if any ruby binary is locally available under
// `.rubies/versions/`. Picks the first one found and a moderate-size
// in-tree fixture. Skips cleanly when no ruby is installed.
func TestNormalizeOnRealFixture(t *testing.T) {
	repoRoot, ok := findRepoRoot(t)
	if !ok {
		t.Skip("repo root not found")
	}
	rubyBin, ok := findFirstRuby(filepath.Join(repoRoot, ".rubies", "versions"))
	if !ok {
		t.Skip("no MRI binary under .rubies/versions/")
	}
	fixture := filepath.Join(repoRoot, "internal", "integrationtest",
		"testdata", "mri-tests", "test_const.rb")
	src, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var stdout, stderr bytes.Buffer
	cmd := exec.Command(rubyBin, "--disable-gems", "--dump=parsetree", fixture)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("ruby dump failed (%s): %v\nstderr: %s", rubyBin, err, stderr.String())
	}
	out := NormalizeWithSource(stdout.String(), string(src))
	if out == "" {
		t.Fatal("empty normalised output")
	}
	// First line should be the flat-form root node identifier (e.g.
	// `NODE_SCOPE` on pre-Prism or `ProgramNode` on Prism 3.4+). No
	// indented tree marker should leak through.
	first, _, _ := strings.Cut(out, "\n")
	if strings.Contains(first, "+-") || strings.Contains(first, "@") {
		t.Errorf("output not flattened, first line still tree-shaped: %q", first)
	}
	if !strings.HasPrefix(first, "NODE_") && !strings.HasSuffix(first, "Node") {
		t.Errorf("unexpected root node name: %q", first)
	}
	// Idempotent: re-normalising should not change anything.
	out2 := NormalizeWithSource(out, string(src))
	if out != out2 {
		t.Errorf("not idempotent on real fixture")
	}
}

// findRepoRoot walks up from cwd looking for the go.mod that names this
// repository.
func findRepoRoot(t *testing.T) (string, bool) {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		return "", false
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// findFirstRuby returns the first `<rootdir>/<ver>/bin/ruby` it finds,
// sorted lexically for determinism.
func findFirstRuby(rubiesDir string) (string, bool) {
	entries, err := os.ReadDir(rubiesDir)
	if err != nil {
		return "", false
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		bin := filepath.Join(rubiesDir, e.Name(), "bin", "ruby")
		if st, err := os.Stat(bin); err == nil && st.Mode()&0o111 != 0 {
			return bin, true
		}
	}
	return "", false
}

func BenchmarkNormalize(b *testing.B) {
	in := strings.Repeat(sampleDump3x, 100)
	b.ResetTimer()
	for b.Loop() {
		_ = Normalize(in)
	}
}

func BenchmarkNormalizeWithSource(b *testing.B) {
	in := strings.Repeat(sampleDump3x, 100)
	src := strings.Repeat("__LINE__\n", 100)
	b.ResetTimer()
	for b.Loop() {
		_ = NormalizeWithSource(in, src)
	}
}

const sampleDump3x = `###########################################################
## Do NOT use this node dump for any purpose other than  ##
## debug and research.  Compatibility is not guaranteed. ##
###########################################################

# @ NODE_SCOPE (id: 24, line: 1, location: (1,0)-(4,3))
# +- nd_tbl: (empty)
# +- nd_args:
# |   (null node)
# +- nd_body:
#     @ NODE_DEFN (id: 1, line: 1, location: (1,0)-(4,3))*
#     +- nd_mid: :foo
#     +- nd_defn:
#         @ NODE_SCOPE (id: 23, line: 4, location: (1,0)-(4,3))
#         +- nd_tbl: :x
#         +- nd_args:
#         |   (null node)
#         +- nd_body:
#             @ NODE_LIT (id: 6, line: 2, location: (2,7)-(2,8))
#             +- nd_lit: 42
`
