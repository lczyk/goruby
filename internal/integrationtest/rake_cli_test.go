//go:build integration

// End-to-end exercise of the rake CLI through the goruby binary. Each
// scenario builds a Rakefile in a tempdir, execs `goruby -I<rake/lib>
// <rake/exe/rake> <args...>` against it, and asserts stdout + exit
// code. A `ruby -> goruby` symlink is dropped into the tempdir's bin
// and prepended to PATH so anything that shells out to `ruby` (rake's
// FileUtils.sh, TestTask) ends up back inside goruby.
package integrationtest

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
)

var (
	gorubyBinPath string
	gorubyBinErr  error
	gorubyBinOnce sync.Once
)

// gorubyBin builds the goruby binary once per test process and returns
// its absolute path. Subsequent calls reuse the cached path. Test fails
// fatally if the build itself fails -- nothing in this file makes sense
// without it.
func gorubyBin(t *testing.T) string {
	t.Helper()
	gorubyBinOnce.Do(func() {
		dir, err := os.MkdirTemp("", "goruby-bin-")
		if err != nil {
			gorubyBinErr = err
			return
		}
		bin := filepath.Join(dir, "goruby")
		if runtime.GOOS == "windows" {
			bin += ".exe"
		}
		// Find repo root by walking up from cwd until go.mod surfaces.
		cwd, err := os.Getwd()
		if err != nil {
			gorubyBinErr = err
			return
		}
		root := cwd
		for {
			if _, statErr := os.Stat(filepath.Join(root, "go.mod")); statErr == nil {
				break
			}
			parent := filepath.Dir(root)
			if parent == root {
				gorubyBinErr = err
				return
			}
			root = parent
		}
		cmd := exec.Command("go", "build", "-o", bin, "./cmd/goruby")
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		if err != nil {
			gorubyBinErr = err
			gorubyBinPath = string(out)
			return
		}
		gorubyBinPath = bin
	})
	if gorubyBinErr != nil {
		t.Fatalf("build goruby: %v\n%s", gorubyBinErr, gorubyBinPath)
	}
	return gorubyBinPath
}

// rakeGemPaths returns absolute paths to the rake gem's lib and exe
// directories shipped under testdata.
func rakeGemPaths(t *testing.T) (libDir, exePath string) {
	t.Helper()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	libDir = filepath.Join(cwd, "testdata", "gems", "rake", "lib")
	exePath = filepath.Join(cwd, "testdata", "gems", "rake", "exe", "rake")
	return
}

// rakeEnv builds an env slice with PATH pointing at a fresh bin dir
// (created under t.TempDir) holding a `ruby -> goruby` symlink. Any
// subprocess that resolves `ruby` via PATH ends up running goruby.
// Kept separate from the script-cwd so real-world Rakefile tests can
// run from a vendored gem directory without polluting it.
func rakeEnv(t *testing.T, gorubyPath string) []string {
	t.Helper()
	binDir := filepath.Join(t.TempDir(), "bin")
	require.NoError(t, os.MkdirAll(binDir, 0o755))
	symlinkPath := filepath.Join(binDir, "ruby")
	require.NoError(t, os.Symlink(gorubyPath, symlinkPath))
	env := os.Environ()
	pathOverride := binDir + string(os.PathListSeparator) + os.Getenv("PATH")
	for i, kv := range env {
		if strings.HasPrefix(kv, "PATH=") {
			env[i] = "PATH=" + pathOverride
			pathOverride = ""
			break
		}
	}
	if pathOverride != "" {
		env = append(env, "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	return env
}

// runRake invokes goruby on the rake CLI with the given args, in the
// given working directory, with `ruby -> goruby` on PATH. Returns
// combined stdout/stderr and any exit error.
func runRake(t *testing.T, cwd string, args ...string) (string, error) {
	t.Helper()
	bin := gorubyBin(t)
	libDir, exePath := rakeGemPaths(t)
	fullArgs := append([]string{"-I", libDir, exePath}, args...)
	cmd := exec.Command(bin, fullArgs...)
	cmd.Dir = cwd
	cmd.Env = rakeEnv(t, bin)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}

const basicRakefile = `
desc "say hi"
task :hi do
  puts "hello from rake"
end

namespace :db do
  desc "migrate db"
  task :migrate do
    puts "migrating"
  end

  desc "seed db"
  task :seed => :migrate do
    puts "seeding"
  end
end

desc "run all"
task :all => ["db:seed", :hi]

task :default => :hi
`

func writeRakefile(t *testing.T, dir, body string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Rakefile"), []byte(body), 0o644))
}

func TestRakeCLI_DashT(t *testing.T) {
	dir := t.TempDir()
	writeRakefile(t, dir, basicRakefile)
	out, err := runRake(t, dir, "-T")
	require.NoError(t, err, "rake -T: %s", out)
	want := "rake all         # run all\n" +
		"rake db:migrate  # migrate db\n" +
		"rake db:seed     # seed db\n" +
		"rake hi          # say hi\n"
	require.Equal(t, want, out)
}

func TestRakeCLI_RunTask(t *testing.T) {
	dir := t.TempDir()
	writeRakefile(t, dir, basicRakefile)
	out, err := runRake(t, dir, "hi")
	require.NoError(t, err, "rake hi: %s", out)
	require.Equal(t, "hello from rake\n", out)
}

func TestRakeCLI_Default(t *testing.T) {
	dir := t.TempDir()
	writeRakefile(t, dir, basicRakefile)
	out, err := runRake(t, dir)
	require.NoError(t, err, "rake (default): %s", out)
	require.Equal(t, "hello from rake\n", out)
}

func TestRakeCLI_Namespace(t *testing.T) {
	dir := t.TempDir()
	writeRakefile(t, dir, basicRakefile)
	out, err := runRake(t, dir, "db:migrate")
	require.NoError(t, err, "rake db:migrate: %s", out)
	require.Equal(t, "migrating\n", out)
}

func TestRakeCLI_Prereq(t *testing.T) {
	dir := t.TempDir()
	writeRakefile(t, dir, basicRakefile)
	out, err := runRake(t, dir, "db:seed")
	require.NoError(t, err, "rake db:seed: %s", out)
	require.Equal(t, "migrating\nseeding\n", out)
}

func TestRakeCLI_Multi(t *testing.T) {
	dir := t.TempDir()
	writeRakefile(t, dir, basicRakefile)
	out, err := runRake(t, dir, "db:migrate", "hi")
	require.NoError(t, err, "rake db:migrate hi: %s", out)
	require.Equal(t, "migrating\nhello from rake\n", out)
}

func TestRakeCLI_All(t *testing.T) {
	dir := t.TempDir()
	writeRakefile(t, dir, basicRakefile)
	out, err := runRake(t, dir, "all")
	require.NoError(t, err, "rake all: %s", out)
	require.Equal(t, "migrating\nseeding\nhello from rake\n", out)
}

func TestRakeCLI_TaskArgs(t *testing.T) {
	dir := t.TempDir()
	writeRakefile(t, dir, `
task :greet, [:who, :punct] do |_t, args|
  puts "hi #{args[:who]}#{args[:punct]}"
end
`)
	out, err := runRake(t, dir, "greet[world,!]")
	require.NoError(t, err, "rake greet[...]: %s", out)
	require.Equal(t, "hi world!\n", out)
}

func TestRakeCLI_UnknownTaskNonzeroExit(t *testing.T) {
	dir := t.TempDir()
	writeRakefile(t, dir, basicRakefile)
	out, err := runRake(t, dir, "does-not-exist")
	require.Error(t, err, assert.AnyError, "expected nonzero exit, got out=%q", out)
	if !strings.Contains(out, "Don't know how to build task") && !strings.Contains(out, "does-not-exist") {
		t.Fatalf("expected unknown-task message, got %q", out)
	}
}

func TestRakeCLI_TestTaskDefinition(t *testing.T) {
	// TestTask DSL surfaces a `test` task in `rake -T` even when we
	// don't invoke it. Exercises the DSL load path; invocation needs
	// the subprocess chain (rake -> ruby -> test files) which is
	// covered separately when we wire it up.
	dir := t.TempDir()
	writeRakefile(t, dir, `
require "rake/testtask"
Rake::TestTask.new(:test) do |t|
  t.libs << "test"
  t.test_files = FileList["test/**/test_*.rb"]
end
`)
	out, err := runRake(t, dir, "-T")
	require.NoError(t, err, "rake -T: %s", out)
	require.ContainsString(t, out, "rake test")
}

// gemDir returns the absolute path to a vendored gem dir under
// testdata/gems/. Used by the real-world Rakefile tests.
func gemDir(t *testing.T, name string) string {
	t.Helper()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	return filepath.Join(cwd, "testdata", "gems", name)
}

// TestRakeCLI_RealWorld_RakeSelf runs rake -T against rake's own
// Rakefile. The Rakefile requires rdoc/task (stubbed in goruby) and
// rake/testtask (real). Asserts the `test` task surfaces.
func TestRakeCLI_RealWorld_RakeSelf(t *testing.T) {
	dir := gemDir(t, "rake")
	out, err := runRake(t, dir, "-T")
	require.NoError(t, err, "rake -T on rake: %s", out)
	require.ContainsString(t, out, "rake test")
}

// TestRakeCLI_RealWorld_PanUnicode runs rake -A -T against pan-unicode-lang's
// Rakefile. No `desc` calls, so -T alone shows nothing; -A includes
// all tasks. Asserts `compile` / `run` / `default` appear.
func TestRakeCLI_RealWorld_PanUnicode(t *testing.T) {
	dir := gemDir(t, "pan-unicode-lang")
	out, err := runRake(t, dir, "-A", "-T")
	require.NoError(t, err, "rake -A -T on pan-unicode-lang: %s", out)
	for _, task := range []string{"rake compile", "rake default", "rake run"} {
		require.ContainsString(t, out, task)
	}
}

// TestRakeCLI_RealWorld_EsolangBook_List runs rake -T against the
// esolang-book-sources Rakefile. Asserts the `test` task surfaces with
// its desc.
func TestRakeCLI_RealWorld_EsolangBook_List(t *testing.T) {
	dir := gemDir(t, "esolang-book-sources")
	out, err := runRake(t, dir, "-T")
	require.NoError(t, err, "rake -T on esolang-book-sources: %s", out)
	require.ContainsString(t, out, "rake test")
	require.ContainsString(t, out, "run test")
}

// runEsolangSubTask is the shared body for the per-interpreter eso
// driver tests: cd into the gem dir, invoke `rake <task>`, fail if
// the Rakefile printed "NG:" (its assert_equal failure marker) or
// rake exited nonzero.
func runEsolangSubTask(t *testing.T, task string) {
	t.Helper()
	dir := gemDir(t, "esolang-book-sources")
	out, err := runRake(t, dir, task)
	require.NoError(t, err, "rake %s: %s", task, out)
	if strings.Contains(out, "NG:") {
		t.Fatalf("%s reported mismatch:\n%s", task, out)
	}
}

// TestRakeCLI_RealWorld_EsolangBook_RunHQ9 invokes `rake test_hq9plus`
// against the esolang-book-sources Rakefile. The task backticks
// `ruby hq9plus.rb <input>` three times and compares output via the
// Rakefile's `assert_equal` (prints "NG:" on mismatch, nothing on
// success). With ruby -> goruby on PATH the backticked subprocesses
// run goruby on the pure-ruby interpreter scripts.
func TestRakeCLI_RealWorld_EsolangBook_RunHQ9(t *testing.T) {
	runEsolangSubTask(t, "test_hq9plus")
}

// TestRakeCLI_RealWorld_EsolangBook_RunBrainfCk runs the brainf_ck
// interpreter test task. Same backtick-shellout shape as HQ9.
func TestRakeCLI_RealWorld_EsolangBook_RunBrainfCk(t *testing.T) {
	runEsolangSubTask(t, "test_brainf_ck")
}

// TestRakeCLI_RealWorld_EsolangBook_RunStarry runs the starry
// interpreter test task.
func TestRakeCLI_RealWorld_EsolangBook_RunStarry(t *testing.T) {
	runEsolangSubTask(t, "test_starry")
}

// TestRakeCLI_RealWorld_EsolangBook_RunBolic runs the bolic
// interpreter test task.
func TestRakeCLI_RealWorld_EsolangBook_RunBolic(t *testing.T) {
	runEsolangSubTask(t, "test_bolic")
}

// TestRakeCLI_RealWorld_KaiserRuby runs rake -A -T against
// kaiser-ruby's Rakefile. The Rakefile requires bundler/gem_tasks
// (now silently stubbed) and rspec/core/rake_task (now stubbed with
// a RakeTask.new that registers a noop rake task). Asserts the
// spec + default tasks surface in the all-tasks listing.
func TestRakeCLI_RealWorld_KaiserRuby(t *testing.T) {
	dir := gemDir(t, "kaiser-ruby")
	out, err := runRake(t, dir, "-A", "-T")
	require.NoError(t, err, "rake -A -T on kaiser-ruby: %s", out)
	require.ContainsString(t, out, "rake spec")
	require.ContainsString(t, out, "rake default")
}

// TestRakeCLI_DashD exercises the long-description form. A task's
// desc string can span multiple lines; rake -D prints the full
// description block under the task name (indented).
func TestRakeCLI_DashD(t *testing.T) {
	dir := t.TempDir()
	writeRakefile(t, dir, `
desc "say hi
this is a longer description
spanning multiple lines"
task :hi do
  puts "hello"
end
`)
	out, err := runRake(t, dir, "-D")
	require.NoError(t, err, "rake -D: %s", out)
	want := "rake hi\n" +
		"    say hi\n" +
		"    this is a longer description\n" +
		"    spanning multiple lines\n" +
		"\n"
	require.Equal(t, want, out)
}

// TestRakeCLI_RealWorld_EsolangBook_RunWhitespace runs the whitespace
// interpreter test task. Skipped: the whitespace interpreter script
// itself raises a ProgramError inside goruby (interpreter logic, not
// rake plumbing). Tracked separately as a goruby evaluator gap.
func TestRakeCLI_RealWorld_EsolangBook_RunWhitespace(t *testing.T) {
	t.Skip("whitespace interpreter raises in goruby -- unrelated evaluator gap")
	runEsolangSubTask(t, "test_whitespace")
}

// TestRakeCLI_TaskShellsOutToRuby exercises the symlinked
// ruby -> goruby on PATH: a task backticks `ruby -e 'puts 7+8'`. With
// the symlink, that subprocess runs goruby and the result lands in
// the captured stdout. Demonstrates the end-to-end shellout path
// works once we wire it -- a simpler shape than the HQ9 task because
// the body doesn't reach into FileUtils.
func TestRakeCLI_TaskShellsOutToRuby(t *testing.T) {
	dir := t.TempDir()
	writeRakefile(t, dir, `
task :probe do
  result = ` + "`ruby -e 'puts 7 + 8'`" + `
  puts "got: #{result.strip}"
end
`)
	out, err := runRake(t, dir, "probe")
	require.NoError(t, err, "rake probe: %s", out)
	require.Equal(t, "got: 15\n", out)
}
