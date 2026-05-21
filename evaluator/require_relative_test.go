package evaluator

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/goruby/object"
	"github.com/lczyk/goruby/parser"
	"github.com/lczyk/goruby/token"
)

// writeFile drops content at dir/name and fails the test if it can't.
func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	assert.NoError(t, os.WriteFile(p, []byte(content), 0o644), "write %s", p)
	return p
}

// TestRequireRelativeLoadsSibling: parent.rb does
// `require_relative 'child'` and reads a constant defined in child.rb.
// Verifies (a) the sibling is loaded relative to parent's dir, (b) the
// loaded constants are visible to the parent after the call returns.
func TestRequireRelativeLoadsSibling(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "child.rb", "GREETING = 'hello'\n")
	parent := writeFile(t, dir, "parent.rb", "require_relative 'child'\nputs GREETING\n")

	src, err := os.ReadFile(parent)
	assert.NoError(t, err, "read parent")

	target := token.MustParseVersion("2.6")
	prog, err := parser.ParseFile(parent, src, 0, parser.WithVersion(target))
	assert.NoError(t, err, "parse parent")

	var stdout bytes.Buffer
	env := object.NewMainEnvironment(object.WithVersion(target), object.WithStdout(&stdout))
	_, err = Eval(prog, env)
	assert.NoError(t, err, "eval")
	assert.Equal(t, "hello\n", stdout.String())
}

// TestRequireRelativeIdempotent: requiring the same file twice loads
// the body once and returns true / false respectively.
func TestRequireRelativeIdempotent(t *testing.T) {
	dir := t.TempDir()
	// child increments a global on every load; if loaded twice it'd be 2.
	writeFile(t, dir, "counter.rb", "$loads = ($loads || 0) + 1\n")
	parent := writeFile(t, dir, "parent.rb",
		"a = require_relative 'counter'\nb = require_relative 'counter'\nputs a\nputs b\nputs $loads\n")

	src, err := os.ReadFile(parent)
	assert.NoError(t, err, "read parent")

	target := token.MustParseVersion("2.6")
	prog, err := parser.ParseFile(parent, src, 0, parser.WithVersion(target))
	assert.NoError(t, err, "parse parent")

	var stdout bytes.Buffer
	env := object.NewMainEnvironment(object.WithVersion(target), object.WithStdout(&stdout))
	_, err = Eval(prog, env)
	assert.NoError(t, err, "eval")
	assert.Equal(t, "true\nfalse\n1\n", stdout.String())
}

// TestRequireRelativeTransitive: a.rb -> requires b.rb -> requires c.rb.
// Each require resolves relative to its own file, not the entry point.
func TestRequireRelativeTransitive(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "lib")
	assert.NoError(t, os.Mkdir(sub, 0o755), "mkdir lib")
	writeFile(t, sub, "c.rb", "C_VAL = 3\n")
	writeFile(t, sub, "b.rb", "require_relative 'c'\nB_VAL = C_VAL + 1\n")
	parent := writeFile(t, dir, "a.rb", "require_relative 'lib/b'\nputs B_VAL\n")

	src, err := os.ReadFile(parent)
	assert.NoError(t, err, "read parent")

	target := token.MustParseVersion("2.6")
	prog, err := parser.ParseFile(parent, src, 0, parser.WithVersion(target))
	assert.NoError(t, err, "parse parent")

	var stdout bytes.Buffer
	env := object.NewMainEnvironment(object.WithVersion(target), object.WithStdout(&stdout))
	_, err = Eval(prog, env)
	assert.NoError(t, err, "eval")
	assert.Equal(t, "4\n", stdout.String())
}
