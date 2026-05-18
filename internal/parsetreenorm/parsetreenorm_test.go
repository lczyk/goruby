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

func TestStripsPrismTokenLoc(t *testing.T) {
	in := "@ ProgramNode\n+-- name_loc: nil\n+-- value_loc: (1,0)-(1,5) = \"hello\"\n"
	out := Normalize(in)
	if strings.Contains(out, "_loc:") {
		t.Errorf("_loc lines not stripped:\n%s", out)
	}
}

func TestStripsNdAlen(t *testing.T) {
	in := "@ NODE_ARRAY\n+- nd_alen: 99\n+- nd_head:\n    @ NODE_LIT\n    +- nd_lit: 1\n"
	out := Normalize(in)
	if strings.Contains(out, "nd_alen") {
		t.Errorf("nd_alen line not stripped:\n%s", out)
	}
}

func TestStripsTempPathVariants(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"prism-filepath", "@ ProgramNode\n+-- filepath: \"/tmp/foo/parsetree-12345.rb\"\n"},
		{"nd-lit-path", "@ NODE_STR\n+- nd_lit: \"/var/folders/x/parsetree-99.rb\"\n"},
		{"embedded-substr", "@ NODE_STR\n+- nd_lit: \"(eval at /tmp/parsetree-7.rb:1)\"\n"},
	}
	for _, c := range cases {
		out := Normalize(c.in)
		if strings.Contains(out, "/tmp/") || strings.Contains(out, "/var/folders/") {
			t.Errorf("%s: tempfile path leaked:\n%s", c.name, out)
		}
	}
	// Embedded-substr case should keep the surrounding literal.
	embedded := Normalize("@ NODE_STR\n+- nd_lit: \"(eval at /tmp/parsetree-7.rb:1)\"\n")
	if !strings.Contains(embedded, "<TEMPFILE>") {
		t.Errorf("expected <TEMPFILE> placeholder:\n%s", embedded)
	}
}

func TestPrismInlineChildField(t *testing.T) {
	// Prism inlines child nodes as `+-- @ ChildNode` -- not a list field.
	in := "@ ProgramNode\n+-- @ DefNode\n    +-- name: :foo\n"
	out := Normalize(in)
	if !strings.Contains(out, "DefNode") {
		t.Errorf("inline child node lost:\n%s", out)
	}
}

func TestSplitFieldNameValueNoSpace(t *testing.T) {
	// `name:value` (no space after colon) -- rare leaf form.
	name, val, ok := splitFieldNameValue("foo:bar")
	if !ok || name != "foo" || val != "bar" {
		t.Errorf("got (%q, %q, %v); want (foo, bar, true)", name, val, ok)
	}
}

func TestSplitFieldNameValueNoColon(t *testing.T) {
	name, val, ok := splitFieldNameValue("just-a-name")
	if ok || name != "just-a-name" || val != "" {
		t.Errorf("got (%q, %q, %v); want (just-a-name, '', false)", name, val, ok)
	}
}

func TestParseNodeKindRejects(t *testing.T) {
	cases := []string{
		"no-at-prefix",
		"@ ",          // empty after `@ `
		"@ 123lower",  // bad first char
		"@ NODE_",     // too short (just prefix)
	}
	for _, c := range cases {
		if kind, ok := parseNodeKind(c); ok {
			t.Errorf("parseNodeKind(%q) = (%q, true); want false", c, kind)
		}
	}
}

func TestParseNodeKindPrismForm(t *testing.T) {
	if kind, ok := parseNodeKind("@ ProgramNode (location: ...)"); !ok || kind != "ProgramNode" {
		t.Errorf("got (%q, %v); want (ProgramNode, true)", kind, ok)
	}
}

func TestConsumeFieldBodyMissingBody(t *testing.T) {
	// Subtree field with no body line at expected depth -- leave field empty.
	in := "@ NODE_SCOPE\n+- nd_body:\n@ NODE_ORPHAN\n"
	out := Normalize(in)
	if out == "" {
		t.Errorf("output empty for malformed body")
	}
}

func TestConsumeFieldBodyBadNodeKind(t *testing.T) {
	// Body line starts with `@ ` but kind doesn't parse -- skip.
	in := "@ NODE_SCOPE\n+- nd_body:\n    @ 0bad\n"
	_ = Normalize(in) // should not panic
}

func TestTrimTrailingStarLineRejects(t *testing.T) {
	// `*` not preceded by `)` or NODE_X -- leave alone.
	in := "some random *\n"
	out := stripTrailingStars(in)
	if out != in {
		t.Errorf("stripped non-qualifying `*`: %q", out)
	}
}

func TestStripTrailingStarsFastPath(t *testing.T) {
	in := "no stars at end of line\nplain text\n"
	out := stripTrailingStars(in)
	if out != in {
		t.Errorf("fast-path mutated input: %q", out)
	}
}

func TestStripLeadingHashFastPath(t *testing.T) {
	in := "no hash here\nplain line\n"
	out := stripLeadingHash(in)
	if out != in {
		t.Errorf("fast-path mutated input: %q", out)
	}
}

func TestStripIDLineLocationFastPath(t *testing.T) {
	in := "@ NODE_LIT\n+- nd_lit: 1\n"
	out := stripIDLineLocation(in)
	if out != in {
		t.Errorf("fast-path mutated input: %q", out)
	}
}

func TestStripIDLineLocationCodeRange(t *testing.T) {
	in := "@ NODE_LIT (id: 5, line: 2, code_range: (2,0)-(2,8))\n+- nd_lit: 2\n"
	out := stripIDLineLocation(in)
	if strings.Contains(out, "code_range") || strings.Contains(out, "id:") {
		t.Errorf("code_range form not stripped: %q", out)
	}
}

func TestStripIDLineLocationMalformed(t *testing.T) {
	// `(id:` substring without the full pattern -- copy through, no panic.
	in := "@ NODE_LIT (id: not-digits)\n"
	out := stripIDLineLocation(in)
	if !strings.Contains(out, "(id: not-digits)") {
		t.Errorf("malformed input mangled: %q", out)
	}
}

func TestStripSiblingIndexFastPath(t *testing.T) {
	in := "@ NODE_LIT\n+- nd_lit: 1\n"
	out := stripSiblingIndex(in)
	if out != in {
		t.Errorf("fast-path mutated input: %q", out)
	}
}

func TestStripSiblingIndexMalformed(t *testing.T) {
	// `+- nd_xxx` w/out the `(N):` suffix -- copy through.
	in := "+- nd_head: leaf-value\n"
	out := stripSiblingIndex(in)
	if out != in {
		t.Errorf("non-matching marker mangled: %q", out)
	}
}

func TestMaskLineMagicNoNodeLitGate(t *testing.T) {
	// Source has __LINE__ but dump has no NODE_LIT -- gate triggers, returns dump as-is.
	src := "__LINE__\n"
	dump := "@ NODE_SCOPE\n+- nd_body:\n    (null node)\n"
	out := NormalizeWithSource(dump, src)
	if !strings.Contains(out, "NODE_SCOPE") {
		t.Errorf("dump corrupted:\n%s", out)
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

// BenchmarkNormalizePrism34 exercises the Prism (3.4+) strip passes that
// sampleDump3x doesn't trigger: `_loc:` token lines, `(length: N)` list
// fields, `+-- @ Child` inline child nodes, and standalone `(location: ...)`
// headers.
func BenchmarkNormalizePrism34(b *testing.B) {
	in := strings.Repeat(samplePrism34, 100)
	b.ResetTimer()
	for b.Loop() {
		_ = Normalize(in)
	}
}

// BenchmarkNormalize2x exercises pre-3.x strip passes missing from the
// 3.x sample: bare `(line: N)` headers, `nd_alen` leakage, and
// `+- nd_xxx (N):` sibling-index suffix.
func BenchmarkNormalize2x(b *testing.B) {
	in := strings.Repeat(sample2x, 100)
	b.ResetTimer()
	for b.Loop() {
		_ = Normalize(in)
	}
}

// BenchmarkNormalizeIdempotent measures the fast-path floor: input is
// already normalised, so every strip helper should hit its
// no-work-needed branch (zero / minimal alloc).
func BenchmarkNormalizeIdempotent(b *testing.B) {
	in := Normalize(strings.Repeat(sampleDump3x, 100))
	b.ResetTimer()
	for b.Loop() {
		_ = Normalize(in)
	}
}

// BenchmarkNormalizeRealFixture runs against a real MRI parsetree dump
// when an MRI binary is locally available. Skips cleanly otherwise.
func BenchmarkNormalizeRealFixture(b *testing.B) {
	repoRoot, ok := findRepoRootB(b)
	if !ok {
		b.Skip("repo root not found")
	}
	rubyBin, ok := findFirstRuby(filepath.Join(repoRoot, ".rubies", "versions"))
	if !ok {
		b.Skip("no MRI binary under .rubies/versions/")
	}
	fixture := filepath.Join(repoRoot, "internal", "integrationtest",
		"testdata", "mri-tests", "test_const.rb")
	src, err := os.ReadFile(fixture)
	if err != nil {
		b.Fatalf("read fixture: %v", err)
	}
	var stdout, stderr bytes.Buffer
	cmd := exec.Command(rubyBin, "--disable-gems", "--dump=parsetree", fixture)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		b.Fatalf("ruby dump failed: %v\nstderr: %s", err, stderr.String())
	}
	dump := stdout.String()
	srcStr := string(src)
	b.ResetTimer()
	b.SetBytes(int64(len(dump)))
	for b.Loop() {
		_ = NormalizeWithSource(dump, srcStr)
	}
}

// findRepoRootB is the *testing.B variant of findRepoRoot.
func findRepoRootB(b *testing.B) (string, bool) {
	b.Helper()
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

// samplePrism34 mimics a Prism (3.4+) `--dump=parsetree` excerpt:
// CamelCase node names, `(location: ...)` headers, `_loc:` per-token
// lines, `(length: N)` list field with inline `+-- @ Child` entries.
const samplePrism34 = `@ ProgramNode (location: (1,0)-(4,3))
+-- locals: []
+-- statements: (length: 2)
    +-- @ DefNode (location: (1,0)-(2,3))
    |   +-- name: :foo
    |   +-- name_loc: (1,4)-(1,7) = "foo"
    |   +-- parameters: nil
    |   +-- body:
    |       @ StatementsNode (location: (2,0)-(2,2))
    |       +-- body: (length: 1)
    |           +-- @ IntegerNode (location: (2,0)-(2,2))
    |               +-- value_loc: (2,0)-(2,2) = "42"
    +-- @ CallNode (location: (4,0)-(4,3))
        +-- name: :bar
        +-- message_loc: (4,0)-(4,3) = "bar"
        +-- arguments: nil
`

// sample2x mimics a 1.9 / 2.x MRI dump: bare `(line: N)` headers (no id,
// no location), `nd_alen` leakage on the trailing NODE_ARRAY tail, and
// `+- nd_xxx (N):` sibling-index suffix added by 3.x intermediates but
// retroactively common in older dumps that got merged through.
const sample2x = `# @ NODE_SCOPE (line: 1)
# +- nd_tbl: :a, :b
# +- nd_args:
# |   (null node)
# +- nd_body:
#     @ NODE_ARRAY (line: 2)
#     +- nd_alen: 99
#     +- nd_head (1):
#     |   @ NODE_LIT (line: 2)
#     |   +- nd_lit: 1
#     +- nd_head (2):
#         @ NODE_LIT (line: 3)
#         +- nd_lit: 2
`

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
