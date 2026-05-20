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
		interp := filepath.Join(esolangInterpDir, lang+".rb")
		if _, err := os.Stat(interp); err != nil {
			t.Errorf("%s: missing interpreter %s", lang, interp)
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
