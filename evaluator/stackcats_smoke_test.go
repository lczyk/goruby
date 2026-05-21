package evaluator

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/goruby/object"
	"github.com/lczyk/goruby/parser"
	"github.com/lczyk/goruby/token"
)

// TestStackcatsSmoke is a probe: load stackcats.rb (which itself does
// `require_relative 'stack'` + 'tape'), then `StackCats.run(src)` on the
// hello-world example. Verifies the multi-file require_relative path,
// IO globals, and IndexExpression multi-assignment all line up.
func TestStackcatsSmoke(t *testing.T) {
	out := runStackcats(t, "hello-world.sks", "")
	assert.Equal(t, "Hello, World!", out)
}

// runStackcats loads stackcats.rb and runs StackCats.run on the named
// example program with the provided stdin. Returns captured stdout.
func runStackcats(t *testing.T, exampleName, stdin string) string {
	t.Helper()
	repoRoot, err := filepath.Abs("..")
	assert.NoError(t, err, "repo root")
	gemDir := filepath.Join(repoRoot, "internal/integrationtest/testdata/gems/stackcats")
	exPath := filepath.Join(gemDir, "examples", exampleName)
	exSrc, err := os.ReadFile(exPath)
	assert.NoError(t, err, "read %s", exPath)
	prog := strings.TrimRight(string(exSrc), "\r\n")

	driverSrc := "require_relative 'ruby/stackcats'\n" +
		"StackCats.run(" + rubyStringLit(prog) + ")\n"
	driverPath := filepath.Join(gemDir, "_runner.rb")

	target := token.MustParseVersion("2.6")
	parsed, err := parser.ParseFile(driverPath, []byte(driverSrc), 0, parser.WithVersion(target))
	assert.NoError(t, err, "parse driver")

	var stdoutBuf, stderrBuf bytes.Buffer
	env := object.NewMainEnvironment(
		object.WithVersion(target),
		object.WithStdout(&stdoutBuf),
		object.WithStderr(&stderrBuf),
		object.WithStdin(strings.NewReader(stdin)),
	)
	if _, err := Eval(parsed, env); err != nil {
		t.Fatalf("eval %s: %v\nstdout=%q\nstderr=%q", exampleName, err, stdoutBuf.String(), stderrBuf.String())
	}
	return stdoutBuf.String()
}

func rubyStringLit(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\', '"':
			b.WriteByte('\\')
			b.WriteRune(r)
		case '\n':
			b.WriteString(`\n`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
