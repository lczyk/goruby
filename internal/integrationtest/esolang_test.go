//go:build integration

package integrationtest

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/goruby/evaluator"
	"github.com/lczyk/goruby/object"
	"github.com/lczyk/goruby/parser"
)

// esolangInterpDir holds the committed esolang interpreters (real ruby
// source, fetched from esolangs.org wiki). esolangTestsDir holds one
// subdirectory per interpreter we exercise, each containing .in fixture
// programs and sibling .expected files generated against MRI.
const (
	esolangInterpDir = "testdata/esolangs"
	esolangTestsDir  = "testdata/esolang_tests"
)

// resolveEsolangInterp looks for the interpreter source under
// testdata/esolangs/<lang>.rb first; if that misses, falls back to a
// few common shapes under testdata/gems/<lang>/ for gem-distributed
// interpreters (e.g. pyramid-scheme's pyra.rb).
func resolveEsolangInterp(lang string) string {
	// Lang-specific overrides come first -- when a multi-file gem ships
	// a CLI entry point separate from the class file (interpreter.rb
	// alongside labyrinth.rb / stackcats.rb), we want the entry point
	// to win over the otherwise-matching default `gems/<lang>/<lang>.rb`.
	var cands []string
	switch lang {
	case "pyramid-scheme":
		cands = append(cands, "testdata/gems/pyramid-scheme/pyra.rb")
	case "stackcats":
		cands = append(cands, "testdata/gems/stackcats/ruby/interpreter.rb")
	case "labyrinth":
		cands = append(cands, "testdata/gems/labyrinth/interpreter.rb")
	case "alice":
		cands = append(cands, "testdata/gems/alice/interpreter.rb")
	case "hexagony":
		cands = append(cands, "testdata/gems/hexagony/interpreter.rb")
	case "wumpus":
		// gems/wumpus/interpreter.rb is just the class body --
		// wumpus.rb wraps it with the CLI shim that actually
		// reads ARGV[0] and calls Interpreter#run.
		cands = append(cands, "testdata/gems/wumpus/wumpus.rb")
	case "bouncy-lang":
		cands = append(cands, "testdata/gems/bouncy-lang/bouncy.rb")
	case "bolic":
		cands = append(cands, "testdata/gems/esolang-book-sources/bolic/bolic.rb")
	case "brainf_ck":
		cands = append(cands, "testdata/gems/esolang-book-sources/brainf_ck/brainf_ck.rb")
	case "hq9plus":
		cands = append(cands, "testdata/gems/esolang-book-sources/hq9plus/hq9plus.rb")
	case "starry":
		cands = append(cands, "testdata/gems/esolang-book-sources/starry/starry.rb")
	case "whitespace":
		cands = append(cands, "testdata/gems/esolang-book-sources/whitespace/lib/whitespace.rb")
	}
	cands = append(cands,
		filepath.Join(esolangInterpDir, lang+".rb"),
		filepath.Join("testdata/gems", lang, lang+".rb"),
	)
	for _, p := range cands {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// TestEsolangPrograms runs each <esolangTestsDir>/<lang>/*.in through
// the sibling <esolangInterpDir>/<lang>.rb interpreter under the goruby
// evaluator and compares stdout against the pre-recorded .expected.
//
// The .expected files were generated against MRI (see
// scripts/esolang-tests-oracle); this test is therefore a parity check
// that goruby's evaluator matches MRI on real third-party ruby code,
// not just our own corpus.
func TestEsolangPrograms(t *testing.T) {
	subdirs, err := os.ReadDir(esolangTestsDir)
	assert.NoError(t, err, "read %s", esolangTestsDir)

	for _, sub := range subdirs {
		if !sub.IsDir() {
			continue
		}
		lang := sub.Name()
		interp := resolveEsolangInterp(lang)
		if interp == "" {
			t.Errorf("%s: missing interpreter (looked under %s and gems)", lang, esolangInterpDir)
			continue
		}
		interpSrc, err := os.ReadFile(interp)
		assert.NoError(t, err, "read %s", interp)

		t.Run(lang, func(t *testing.T) {
			ins, err := filepath.Glob(filepath.Join(esolangTestsDir, lang, "*.in"))
			assert.NoError(t, err, "glob %s/*.in", lang)
			sort.Strings(ins)
			if len(ins) == 0 {
				t.Skipf("no .in fixtures under %s", filepath.Join(esolangTestsDir, lang))
			}
			for _, in := range ins {
				name := strings.TrimSuffix(filepath.Base(in), ".in")
				t.Run(name, func(t *testing.T) {
					want, err := os.ReadFile(strings.TrimSuffix(in, ".in") + ".expected")
					assert.NoError(t, err, "read .expected for %s", in)

					var stdout bytes.Buffer
					env := object.NewMainEnvironment(
						object.WithStdout(&stdout),
						object.WithStdin(bytes.NewReader(nil)),
						object.WithARGV([]string{in}),
					)

					prog, err := parser.ParseFile(interp, interpSrc, 0)
					assert.NoError(t, err, "parse %s", interp)

					_, err = evaluator.Eval(prog, env)
					assert.NoError(t, err, "eval %s on %s", interp, in)

					assert.Equal(t, string(want), stdout.String(),
						"%s/%s: stdout mismatch", lang, name)
				})
			}
		})
	}
}
