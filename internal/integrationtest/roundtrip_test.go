//go:build integration

package integrationtest

import (
	"fmt"
	gotoken "go/token"
	"os"
	"strings"
	"testing"

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

const roundtripSkipFile = "roundtrip.skip"

var (
	roundtripASTCounters counters
	roundtripSrcCounters counters
)

func init() {
	registerSummary("TestRoundtripAST  ", &roundtripASTCounters)
	registerSummary("TestRoundtripSrc  ", &roundtripSrcCounters)
}

func TestRoundtrip(t *testing.T) {
	versions, rows := loadGoldenTSV(t)
	skips, err := loadGoldenSkips(roundtripSkipFile, "ast", "src")
	if err != nil {
		t.Fatalf("load %s: %v", roundtripSkipFile, err)
	}

	for _, row := range rows {
		localPath := strings.TrimPrefix(row.file, goldenPrefix)

		src, err := os.ReadFile(localPath)
		if err != nil {
			t.Fatalf("read %s: %v", localPath, err)
		}

		for _, verStr := range versions {
			if !row.results[verStr] {
				continue
			}
			ver := token.MustParseVersion(verStr)

			name := fmt.Sprintf("%s/ruby_%s", row.file, verStr)
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				fset1 := gotoken.NewFileSet()
				prog1, parseErr := parser.ParseFile(fset1, row.file, src, parser.AllErrors, parser.WithVersion(ver))
				if parseErr != nil {
					t.Skipf("initial parse failed: %v", parseErr)
				}

				src2, stringPanic := safeString(prog1)

				var prog2 *ast.Program
				var reParseErr error
				if stringPanic == "" {
					fset2 := gotoken.NewFileSet()
					prog2, reParseErr = parser.ParseFile(fset2, row.file+"<roundtrip>", []byte(src2), parser.AllErrors, parser.WithVersion(ver))
				}

				t.Run("ast", func(t *testing.T) {
					t.Cleanup(func() { recordOutcome(t, &roundtripASTCounters) })

					if e := skips.match("ast", row.file, ver); e != nil {
						t.Skipf("skip: %s", e.reason)
					}
					if stringPanic != "" {
						t.Fatalf("String() panicked: %s", stringPanic)
					}
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

				t.Run("src", func(t *testing.T) {
					t.Cleanup(func() { recordOutcome(t, &roundtripSrcCounters) })

					if e := skips.match("src", row.file, ver); e != nil {
						t.Skipf("skip: %s", e.reason)
					}
					if stringPanic != "" {
						t.Skipf("String() panicked: %s", stringPanic)
					}
					if string(src) != src2 {
						t.Errorf("source mismatch after roundtrip")
					}
				})
			})
		}
	}
}
