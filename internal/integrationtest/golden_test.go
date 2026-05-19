//go:build integration || oracle

package integrationtest

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/goruby/token"
)

const (
	goldenTSV      = "testdata/mri-golden.tsv"
	goldenSkipFile = "golden.skip"
	goldenPrefix   = "internal/integrationtest/"
)

var (
	goldenLexCounters   counters
	goldenParseCounters counters
)

func init() {
	registerSummary("TestMRIGoldenLex  ", &goldenLexCounters)
	registerSummary("TestMRIGoldenParse", &goldenParseCounters)
}

type goldenRow struct {
	file    string
	results map[string]bool // version -> pass
}

func loadGoldenTSV(t *testing.T) (versions []string, rows []goldenRow) {
	t.Helper()
	f, err := os.Open(goldenTSV)
	assert.NoError(t, err, "open %s", goldenTSV)
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if fields[0] == "file" {
			versions = fields[1:]
			continue
		}
		if len(fields) != len(versions)+1 {
			t.Fatalf("malformed golden row: %q (got %d fields, want %d)",
				fields[0], len(fields), len(versions)+1)
		}
		row := goldenRow{
			file:    fields[0],
			results: make(map[string]bool, len(versions)),
		}
		for i, ver := range versions {
			row.results[ver] = fields[i+1] == "pass"
		}
		rows = append(rows, row)
	}
	assert.NoError(t, sc.Err(), "read %s", goldenTSV)
	if len(versions) == 0 {
		t.Fatalf("no version header in %s", goldenTSV)
	}
	return versions, rows
}

// goldenSkipEntry adds a version range to the base skip entry.
type goldenSkipEntry struct {
	phase    string
	verRange versionRange
	pattern  string
	reason   string
}

type versionRange struct {
	lo  token.RubyVersion // zero = unbounded below
	hi  token.RubyVersion // zero = unbounded above
	all bool              // * matches all versions
}

func (vr versionRange) contains(v token.RubyVersion) bool {
	if vr.all {
		return true
	}
	if vr.lo.IsSet() && !v.AtLeast(vr.lo) {
		return false
	}
	if vr.hi.IsSet() && !vr.hi.AtLeast(v) {
		return false
	}
	return true
}

type goldenSkipList struct {
	entries []goldenSkipEntry
}

func loadGoldenSkips(path string, validPhases ...string) (*goldenSkipList, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &goldenSkipList{}, nil
	}
	if err != nil {
		return nil, err
	}
	sl := &goldenSkipList{}
	for lineNo, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// format: <phase> <version-range> <pattern>[: reason]
		parts := strings.SplitN(line, " ", 3)
		if len(parts) < 3 {
			return nil, fmt.Errorf("%s:%d: need at least 3 fields: %q", path, lineNo+1, line)
		}
		phase := parts[0]
		if len(validPhases) > 0 {
			valid := false
			for _, vp := range validPhases {
				if phase == vp {
					valid = true
					break
				}
			}
			if !valid {
				return nil, fmt.Errorf("%s:%d: unknown phase %q", path, lineNo+1, phase)
			}
		}
		vr, err := parseVersionRange(parts[1])
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, lineNo+1, err)
		}
		rest := strings.TrimSpace(parts[2])
		var pattern, reason string
		if colon := strings.Index(rest, ":"); colon >= 0 {
			pattern = strings.TrimSpace(rest[:colon])
			reason = strings.TrimSpace(rest[colon+1:])
		} else {
			pattern = rest
		}
		sl.entries = append(sl.entries, goldenSkipEntry{
			phase: phase, verRange: vr, pattern: pattern, reason: reason,
		})
	}
	return sl, nil
}

func parseVersionRange(s string) (versionRange, error) {
	if s == "*" {
		return versionRange{all: true}, nil
	}
	if strings.HasPrefix(s, "..") {
		hi := token.MustParseVersion(s[2:])
		return versionRange{hi: hi}, nil
	}
	if strings.HasSuffix(s, "..") {
		lo := token.MustParseVersion(s[:len(s)-2])
		return versionRange{lo: lo}, nil
	}
	if idx := strings.Index(s, ".."); idx >= 0 {
		lo := token.MustParseVersion(s[:idx])
		hi := token.MustParseVersion(s[idx+2:])
		return versionRange{lo: lo, hi: hi}, nil
	}
	v := token.MustParseVersion(s)
	return versionRange{lo: v, hi: v}, nil
}

func (sl *goldenSkipList) match(phase, relpath string, ver token.RubyVersion) *goldenSkipEntry {
	if sl == nil {
		return nil
	}
	for i := range sl.entries {
		e := &sl.entries[i]
		if e.phase != phase {
			continue
		}
		if !e.verRange.contains(ver) {
			continue
		}
		if matchPattern(e.pattern, relpath) {
			return e
		}
	}
	return nil
}

// --- test functions ---------------------------------------------------------

func TestMRIGoldenLex(t *testing.T) {
	versions, rows := loadGoldenTSV(t)
	goldenSkips, err := loadGoldenSkips(goldenSkipFile, "lex", "parse")
	assert.NoError(t, err, "load %s", goldenSkipFile)

	for _, row := range rows {
		localPath := strings.TrimPrefix(row.file, goldenPrefix)
		src, err := os.ReadFile(localPath)
		assert.NoError(t, err, "read %s", localPath)

		for _, verStr := range versions {
			mriPass := row.results[verStr]
			ver := token.MustParseVersion(verStr)

			name := fmt.Sprintf("%s/ruby_%s", row.file, verStr)
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				t.Cleanup(func() { recordOutcome(t, &goldenLexCounters) })

				if e := goldenSkips.match("lex", row.file, ver); e != nil {
					t.Skipf("skip: %s", e.reason)
				}

				lexErr := runWithTimeout(func() error {
					return runLexWithVersion(string(src), ver)
				})

				// Lex test enforces only one direction: don't reject what MRI
				// accepts. The other direction (lex passes but MRI rejects)
				// is the parser's job to catch via version-aware rules --
				// many rejections require parse-grade context (kwargs,
				// endless method, find pattern, ...) and the lexer can't
				// reliably detect them without re-doing parser work.
				if mriPass && lexErr != nil {
					t.Errorf("MRI passes at %s but lex failed: %v", verStr, lexErr)
				}
			})
		}
	}
}

func TestMRIGoldenParse(t *testing.T) {
	versions, rows := loadGoldenTSV(t)
	goldenSkips, err := loadGoldenSkips(goldenSkipFile, "lex", "parse")
	assert.NoError(t, err, "load %s", goldenSkipFile)

	for _, row := range rows {
		localPath := strings.TrimPrefix(row.file, goldenPrefix)
		src, err := os.ReadFile(localPath)
		assert.NoError(t, err, "read %s", localPath)

		for _, verStr := range versions {
			mriPass := row.results[verStr]
			ver := token.MustParseVersion(verStr)

			name := fmt.Sprintf("%s/ruby_%s", row.file, verStr)
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				t.Cleanup(func() { recordOutcome(t, &goldenParseCounters) })

				if e := goldenSkips.match("parse", row.file, ver); e != nil {
					t.Skipf("skip: %s", e.reason)
				}

				parseErr := runWithTimeout(func() error {
					return runParseWithVersion(row.file, string(src), ver)
				})

				if mriPass && parseErr != nil {
					t.Errorf("MRI passes at %s but parse failed: %v", verStr, parseErr)
				}
				if !mriPass && parseErr == nil {
					t.Errorf("MRI rejects at %s but parse passed", verStr)
				}
			})
		}
	}
}

// --- summary integration ----------------------------------------------------

var extraSummaries []struct {
	name string
	c    *counters
}

func registerSummary(name string, c *counters) {
	extraSummaries = append(extraSummaries, struct {
		name string
		c    *counters
	}{name, c})
}
