//go:build integration

// Smoke tests for lexer + parser + ast walker against real-world ruby source.
// Fixtures populated by `make gems` (see plan.md).
package integrationtest

import (
	"errors"
	"fmt"
	gotoken "go/token"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lczyk/assert"

	"github.com/lczyk/goruby/ast"
	"github.com/lczyk/goruby/lexer"
	"github.com/lczyk/goruby/parser"
	"github.com/lczyk/goruby/token"
)

const (
	fixturesDir        = "testdata/gems"
	langFixturesDir    = "testdata/mri-tests"
	esolangFixturesDir = "testdata/esolangs"
	lockFile           = "testdata/gems.lock"
	skipFile           = "gems.skip"
	phaseTimeout       = 5 * time.Second
)

type counters struct {
	pass, fail, skip, total atomic.Int64
}

var (
	lexCounters   counters
	parseCounters counters
	walkCounters  counters

	skips     *skipList
	skipsOnce sync.Once
	rubyFiles []string
)

func TestMain(m *testing.M) {
	if err := bootstrap(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	code := m.Run()
	printSummary()
	os.Exit(code)
}

// bootstrap verifies fixtures match gems.lock and pre-walks the file tree.
func bootstrap() error {
	wanted, err := readLockNames(lockFile)
	if err != nil {
		return fmt.Errorf("read %s: %w", lockFile, err)
	}
	for _, name := range wanted {
		dir := filepath.Join(fixturesDir, name)
		rbs, err := countRubyFiles(dir)
		if err != nil {
			return fmt.Errorf("missing fixture %q: %w; run `make gems`", name, err)
		}
		if rbs == 0 {
			return fmt.Errorf("fixture %q has no .rb files; run `make gems`", name)
		}
	}
	// Warn on extra dirs (not failing -- devs may experiment locally).
	entries, err := os.ReadDir(fixturesDir)
	if err == nil {
		want := make(map[string]bool, len(wanted))
		for _, n := range wanted {
			want[n] = true
		}
		for _, e := range entries {
			if e.IsDir() && !want[e.Name()] {
				fmt.Fprintf(os.Stderr, "warning: %s/%s is not in %s\n", fixturesDir, e.Name(), lockFile)
			}
		}
	}

	rubyFiles, err = walkRubyFiles(fixturesDir)
	if err != nil {
		return fmt.Errorf("walk fixtures: %w", err)
	}

	// Also walk language test fixtures (committed, not fetched).
	langFiles, err := walkRubyFiles(langFixturesDir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("walk lang fixtures: %w", err)
	}
	for _, f := range langFiles {
		rubyFiles = append(rubyFiles, "mri-tests/"+f)
	}

	// Also walk esolang interpreter fixtures (committed, not fetched).
	esolangFiles, err := walkRubyFiles(esolangFixturesDir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("walk esolang fixtures: %w", err)
	}
	for _, f := range esolangFiles {
		rubyFiles = append(rubyFiles, "esolangs/"+f)
	}

	skips, err = loadSkips(skipFile)
	if err != nil {
		return fmt.Errorf("load %s: %w", skipFile, err)
	}
	return nil
}

func printSummary() {
	fmt.Fprintln(os.Stderr, "integration test summary:")
	rows := []struct {
		name string
		c    *counters
	}{
		{"TestGemsLex  ", &lexCounters},
		{"TestGemsParse", &parseCounters},
		{"TestGemsWalk ", &walkCounters},
	}
	for _, extra := range extraSummaries {
		rows = append(rows, struct {
			name string
			c    *counters
		}{extra.name, extra.c})
	}
	for _, row := range rows {
		fmt.Fprintf(os.Stderr, "  %s: %d passed, %d failed, %d skipped (%d total)\n",
			row.name, row.c.pass.Load(), row.c.fail.Load(), row.c.skip.Load(), row.c.total.Load())
	}
}

// readLockNames returns the names of gems listed in gems.lock.
func readLockNames(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 1 {
			continue
		}
		names = append(names, strings.TrimSpace(fields[0]))
	}
	return names, nil
}

func countRubyFiles(dir string) (int, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return 0, err
	}
	if !info.IsDir() {
		return 0, fmt.Errorf("%s is not a directory", dir)
	}
	count := 0
	err = filepath.WalkDir(dir, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".rb") {
			count++
		}
		return nil
	})
	return count, err
}

// walkRubyFiles returns paths relative to fixturesDir.
func walkRubyFiles(root string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".rb") {
			rel, _ := filepath.Rel(root, path)
			paths = append(paths, rel)
		}
		return nil
	})
	return paths, err
}

// --- skip list --------------------------------------------------------------

type skipEntry struct {
	phase   string
	pattern string
	reason  string
}

type skipList struct {
	entries []skipEntry
}

func loadSkips(path string) (*skipList, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &skipList{}, nil
	}
	if err != nil {
		return nil, err
	}
	sl := &skipList{}
	for lineNo, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// `<phase> <pattern>[: reason]`
		spaceIdx := strings.IndexAny(line, " \t")
		if spaceIdx < 0 {
			return nil, fmt.Errorf("%s:%d: malformed entry %q", path, lineNo+1, line)
		}
		phase := line[:spaceIdx]
		switch phase {
		case "lex", "parse", "walk":
		default:
			return nil, fmt.Errorf("%s:%d: unknown phase %q", path, lineNo+1, phase)
		}
		rest := strings.TrimSpace(line[spaceIdx+1:])
		var pattern, reason string
		if colon := strings.Index(rest, ":"); colon >= 0 {
			pattern = strings.TrimSpace(rest[:colon])
			reason = strings.TrimSpace(rest[colon+1:])
		} else {
			pattern = rest
		}
		sl.entries = append(sl.entries, skipEntry{phase: phase, pattern: pattern, reason: reason})
	}
	return sl, nil
}

// match returns the entry that skips (phase, relpath), or nil.
func (s *skipList) match(phase, relpath string) *skipEntry {
	if s == nil {
		return nil
	}
	for i := range s.entries {
		e := &s.entries[i]
		if e.phase != phase {
			continue
		}
		if matchPattern(e.pattern, relpath) {
			return e
		}
	}
	return nil
}

// matchesAnyOfPattern returns true if any path in paths matches pattern.
// Used for stale-glob detection: a glob is stale iff any matched file would pass.
func filesMatching(pattern string, paths []string) []string {
	var out []string
	for _, p := range paths {
		if matchPattern(pattern, p) {
			out = append(out, p)
		}
	}
	return out
}

// matchPattern matches `<segment>/.../<segment>` where any segment may be `*`.
// Single `*` only -- no `**`. No partial-segment matching.
func matchPattern(pattern, p string) bool {
	if !strings.ContainsRune(pattern, '*') {
		return pattern == p
	}
	patSegs := strings.Split(pattern, "/")
	pathSegs := strings.Split(p, "/")
	if len(patSegs) != len(pathSegs) {
		return false
	}
	for i, ps := range patSegs {
		if ps == "*" {
			continue
		}
		if ps != pathSegs[i] {
			return false
		}
	}
	return true
}

// --- timeout / panic handling ----------------------------------------------

// runWithTimeout executes fn in a goroutine; returns its error, or a timeout
// error after phaseTimeout. Panics inside fn are forwarded as errors.
// On timeout the goroutine leaks (parser may be hung).
func runWithTimeout(fn func() error) error {
	type result struct{ err error }
	done := make(chan result, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- result{err: fmt.Errorf("panic: %v\n%s", r, debug.Stack())}
			}
		}()
		done <- result{err: fn()}
	}()
	select {
	case r := <-done:
		return r.err
	case <-time.After(phaseTimeout):
		return fmt.Errorf("timeout after %s", phaseTimeout)
	}
}

// --- phase implementations -------------------------------------------------

func runLex(src string) error {
	l := lexer.New(src)
	for l.HasNext() {
		tok := l.NextToken()
		if tok.Type == token.ILLEGAL {
			return fmt.Errorf("ILLEGAL token at pos %d: %q", tok.Pos, tok.Literal)
		}
		if tok.Type == token.EOF {
			break
		}
	}
	return nil
}

func runParse(name, src string) (*ast.Program, error) {
	fset := gotoken.NewFileSet()
	return parser.ParseFile(fset, name, []byte(src), parser.AllErrors)
}

type noopVisitor struct{}

func (noopVisitor) Visit(ast.Node) ast.Visitor { return noopVisitor{} }

func runWalk(program *ast.Program) error {
	if program == nil {
		return errors.New("nil program")
	}
	ast.Walk(noopVisitor{}, program)
	_ = program.String()
	return nil
}

// --- subtest wiring --------------------------------------------------------

func recordOutcome(t *testing.T, c *counters) {
	c.total.Add(1)
	switch {
	case t.Skipped():
		c.skip.Add(1)
	case t.Failed():
		c.fail.Add(1)
	default:
		c.pass.Add(1)
	}
}

// runPhase wires up: skip-list lookup, stale-entry check, timeout, counters.
// `phaseFn` does the work and returns (err). The test passes iff err == nil.
func runPhase(t *testing.T, phase, relpath string, c *counters, phaseFn func() error) {
	t.Helper()
	t.Cleanup(func() { recordOutcome(t, c) })

	entry := skips.match(phase, relpath)
	err := runWithTimeout(phaseFn)

	if entry != nil {
		// Skip-listed: expected to fail. Pass = stale entry.
		if err == nil {
			t.Fatalf("stale skip-list entry: %s %s", phase, relpath)
		}
		reason := entry.reason
		if reason == "" {
			reason = "skip-listed"
		}
		t.Skip(reason)
		return
	}
	if err != nil {
		t.Errorf("%s failed: %s\n%v", phase, relpath, err)
	}
}

func mustReadFile(t *testing.T, relpath string) string {
	t.Helper()
	baseDir := fixturesDir
	if strings.HasPrefix(relpath, "mri-tests/") {
		baseDir = langFixturesDir
		relpath = strings.TrimPrefix(relpath, "mri-tests/")
	} else if strings.HasPrefix(relpath, "esolangs/") {
		baseDir = esolangFixturesDir
		relpath = strings.TrimPrefix(relpath, "esolangs/")
	}
	abs := filepath.Join(baseDir, relpath)
	data, err := os.ReadFile(abs)
	assert.NoError(t, err)
	return string(data)
}

// --- tests -----------------------------------------------------------------

func TestGemsLex(t *testing.T) {
	for _, rel := range rubyFiles {
		rel := rel
		t.Run(rel, func(t *testing.T) {
			t.Parallel()
			src := mustReadFile(t, rel)
			runPhase(t, "lex", rel, &lexCounters, func() error {
				return runLex(src)
			})
		})
	}
}

func TestGemsParse(t *testing.T) {
	for _, rel := range rubyFiles {
		rel := rel
		t.Run(rel, func(t *testing.T) {
			t.Parallel()
			src := mustReadFile(t, rel)
			runPhase(t, "parse", rel, &parseCounters, func() error {
				program, err := runParse(rel, src)
				if err != nil {
					return err
				}
				if program == nil {
					return errors.New("nil program with no error")
				}
				return nil
			})
		})
	}
}

func TestGemsWalk(t *testing.T) {
	for _, rel := range rubyFiles {
		rel := rel
		t.Run(rel, func(t *testing.T) {
			t.Parallel()
			src := mustReadFile(t, rel)
			runPhase(t, "walk", rel, &walkCounters, func() error {
				program, err := runParse(rel, src)
				if err != nil {
					return fmt.Errorf("parse failed before walk: %w", err)
				}
				return runWalk(program)
			})
		})
	}
}
