//go:build integration || oracle

package integrationtest

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/goruby/ast"
	"github.com/lczyk/goruby/parser"
	"github.com/lczyk/goruby/token"
)

func safeString(prog *ast.Program) (result string, panicMsg string) {
	defer func() {
		if r := recover(); r != nil {
			panicMsg = fmt.Sprintf("%v", r)
		}
	}()
	return prog.String(), ""
}

var roundtripCounters counters

func init() {
	registerSummary("TestRoundtrip", &roundtripCounters)
}

// TestRoundtrip parses each golden fixture, re-prints the AST, re-parses
// the printed form, and asserts the second print is byte-identical to the
// first (AST-stable roundtrip). Source-stable roundtrip (input == first
// print) is no longer tested -- TestMRIParseTreeDiff covers parse-tree
// equivalence directly against MRI and supersedes the source-text check.
func TestRoundtrip(t *testing.T) {
	versions, rows := loadGoldenTSV(t)

	for _, row := range rows {
		localPath := strings.TrimPrefix(row.file, goldenPrefix)

		src, err := os.ReadFile(localPath)
		assert.NoError(t, err, "read %s", localPath)

		for _, verStr := range versions {
			if !row.results[verStr] {
				continue
			}
			ver := token.MustParseVersion(verStr)

			name := fmt.Sprintf("%s/ruby_%s", row.file, verStr)
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				t.Cleanup(func() { recordOutcome(t, &roundtripCounters) })

				prog1, parseErr := parser.ParseFile(row.file, src, parser.AllErrors, parser.WithVersion(ver))
				if parseErr != nil {
					t.Skipf("initial parse failed: %v", parseErr)
				}

				src2, stringPanic := safeString(prog1)
				if stringPanic != "" {
					t.Fatalf("String() panicked: %s", stringPanic)
				}

				prog2, reParseErr := parser.ParseFile(row.file+"<roundtrip>", []byte(src2), parser.AllErrors, parser.WithVersion(ver))
				if reParseErr != nil {
					t.Fatalf("re-parse failed:\n--- regenerated source ---\n%s\n--- error ---\n%v", src2, reParseErr)
				}

				src3, reStringPanic := safeString(prog2)
				if reStringPanic != "" {
					t.Fatalf("re-String() panicked: %s", reStringPanic)
				}
				if src2 != src3 {
					t.Errorf("AST roundtrip not stable:\n--- first String() ---\n%s\n--- second String() ---\n%s", src2, src3)
				}
			})
		}
	}
}
