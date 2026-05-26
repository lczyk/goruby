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

// TestEval__dir__: `__dir__` expands to the absolute dir of the file
// presently being evaluated. Verifies the value via a synthetic write
// to a sibling file -- so a subsequent require_relative against
// __dir__ + "/sibling" would also resolve correctly.
func TestEval__dir__(t *testing.T) {
	dir := t.TempDir()
	entry := writeFile(t, dir, "entry.rb", "puts __dir__\n")
	src, err := os.ReadFile(entry)
	assert.NoError(t, err, "read entry")

	target := token.MustParseVersion("2.6")
	prog, err := parser.ParseFile(entry, src, 0, parser.WithVersion(target))
	assert.NoError(t, err, "parse")

	var stdout bytes.Buffer
	env := object.NewMainEnvironment(object.WithVersion(target), object.WithStdout(&stdout))
	_, err = Eval(prog, env)
	assert.NoError(t, err, "eval")
	wantDir, err := filepath.Abs(dir)
	assert.NoError(t, err, "abs")
	assert.Equal(t, wantDir+"\n", stdout.String())
}

// TestRequireViaLoadPath: $LOAD_PATH.unshift then require resolves the
// target through the array, not relative to the requirer.
func TestRequireViaLoadPath(t *testing.T) {
	dir := t.TempDir()
	libs := filepath.Join(dir, "libs")
	assert.NoError(t, os.Mkdir(libs, 0o755), "mkdir libs")
	writeFile(t, libs, "greet.rb", "GREETING = 'hi from libs'\n")
	entry := writeFile(t, dir, "entry.rb",
		"$LOAD_PATH.unshift __dir__ + '/libs'\nrequire 'greet'\nputs GREETING\n")

	src, err := os.ReadFile(entry)
	assert.NoError(t, err, "read entry")

	target := token.MustParseVersion("2.6")
	prog, err := parser.ParseFile(entry, src, 0, parser.WithVersion(target))
	assert.NoError(t, err, "parse")

	var stdout bytes.Buffer
	env := object.NewMainEnvironment(object.WithVersion(target), object.WithStdout(&stdout))
	_, err = Eval(prog, env)
	assert.NoError(t, err, "eval")
	assert.Equal(t, "hi from libs\n", stdout.String())
}

// TestRequireViaLoadPathTransitive: require deep inside a library still
// resolves through $LOAD_PATH (not relative to the inner file). Mirrors
// rake's pattern where rake/ext/string.rb says `require "rake/ext/core"`
// and the lookup needs to walk back through the gem's lib/ dir.
func TestRequireViaLoadPathTransitive(t *testing.T) {
	dir := t.TempDir()
	libs := filepath.Join(dir, "libs")
	deep := filepath.Join(libs, "nest", "ext")
	assert.NoError(t, os.MkdirAll(deep, 0o755), "mkdir deep")
	writeFile(t, deep, "leaf.rb", "LEAF_VAL = 42\n")
	writeFile(t, filepath.Join(libs, "nest"), "trunk.rb",
		"require 'nest/ext/leaf'\nTRUNK_VAL = LEAF_VAL + 1\n")
	writeFile(t, libs, "nest.rb", "require 'nest/trunk'\n")
	entry := writeFile(t, dir, "entry.rb",
		"$LOAD_PATH.unshift __dir__ + '/libs'\nrequire 'nest'\nputs TRUNK_VAL\n")

	src, err := os.ReadFile(entry)
	assert.NoError(t, err, "read entry")

	target := token.MustParseVersion("2.6")
	prog, err := parser.ParseFile(entry, src, 0, parser.WithVersion(target))
	assert.NoError(t, err, "parse")

	var stdout bytes.Buffer
	env := object.NewMainEnvironment(object.WithVersion(target), object.WithStdout(&stdout))
	_, err = Eval(prog, env)
	assert.NoError(t, err, "eval")
	assert.Equal(t, "43\n", stdout.String())
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
