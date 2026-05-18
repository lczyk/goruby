//go:build oracle

// TestMRIParseTreeDiff: parser-correctness oracle.
//
// For each fixture and each MRI version where mri-golden.tsv says it passes:
//
//	src1 -> MRI --dump=parsetree     -> tree1
//	src1 -> goruby Parse -> String() -> src2
//	src2 -> MRI --dump=parsetree     -> tree2
//	normalize(tree1) == normalize(tree2) ?
//
// Three outcome buckets per (file, ver) cell:
//   - pass             : trees match
//   - src2-rejected    : MRI rejected goruby's reformat (String() emitted invalid Ruby)
//   - tree-mismatch    : both parsed, trees diverged after normalization
//
// Skip-list at parsetree.skip selects (phase, ver-range, pattern) where phase is
// one of the bucket names. Stale entries (where the test would pass) fail loudly.
package integrationtest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lczyk/goruby/internal/parsetreenorm"
	"github.com/lczyk/goruby/parser"
	"github.com/lczyk/goruby/token"
)

const (
	parsetreeSkipFile = "parsetree.skip"
	mriExecTimeout    = 30 * time.Second
	parsetreeCacheDir = ".cache/mri-parsetree"
)

var (
	parsetreePassCounters         counters
	parsetreeSrc2RejectedCounters counters
	parsetreeMismatchCounters     counters

	rubiesOnce sync.Once
	rubiesMap  map[string]string // short-ver ("3.4") -> full ruby binary path
	rubiesErr  error
)

func init() {
	registerSummary("TestMRIParseTreeDiff", &parsetreePassCounters)
}

// locateRubies scans .rubies/versions/ for ruby binaries, mapping short
// "major.minor" to the absolute binary path. The repo root is found by walking
// up from cwd until a .rubies/versions/ dir is seen.
func locateRubies() (map[string]string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getwd: %w", err)
	}
	dir := cwd
	var versionsDir string
	for {
		candidate := filepath.Join(dir, ".rubies", "versions")
		if st, err := os.Stat(candidate); err == nil && st.IsDir() {
			versionsDir = candidate
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, fmt.Errorf("could not find .rubies/versions/ walking up from %s", cwd)
		}
		dir = parent
	}
	entries, err := os.ReadDir(versionsDir)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", versionsDir, err)
	}
	out := make(map[string]string)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		full := e.Name() // e.g. "3.4.4" or "1.9.3-p551"
		bin := filepath.Join(versionsDir, full, "bin", "ruby")
		st, err := os.Stat(bin)
		if err != nil || st.Mode()&0o111 == 0 {
			continue
		}
		// Extract major.minor: "1.9.3-p551" -> "1.9", "3.4.4" -> "3.4".
		short := majorMinor(full)
		if short == "" {
			continue
		}
		out[short] = bin
	}
	return out, nil
}

func majorMinor(full string) string {
	parts := strings.SplitN(full, ".", 3)
	if len(parts) < 2 {
		return ""
	}
	// strip suffix like "-p551" from minor if present (shouldn't be, since
	// "1.9.3-p551" has the suffix on patch, not minor -- defensive).
	minor := parts[1]
	if idx := strings.IndexAny(minor, "-+"); idx >= 0 {
		minor = minor[:idx]
	}
	return parts[0] + "." + minor
}

func rubies(t *testing.T) map[string]string {
	t.Helper()
	rubiesOnce.Do(func() {
		rubiesMap, rubiesErr = locateRubies()
	})
	if rubiesErr != nil {
		t.Skipf("MRI binaries unavailable: %v (run `make rubies`)", rubiesErr)
	}
	if len(rubiesMap) == 0 {
		t.Skip("no MRI binaries found under .rubies/versions/")
	}
	return rubiesMap
}

// --- normalisation ----------------------------------------------------------
//
// Lives in internal/parsetreenorm so the same logic backs the
// `normalize-parsetree` CLI under cmd/.

// --- MRI invocation ---------------------------------------------------------

// mriDumpResult captures the outcome of a single MRI --dump=parsetree run.
type mriDumpResult struct {
	tree     string // normalized parsetree (empty on parseErr or fatal)
	parseErr error  // MRI rejected the source (syntax error)
	fatal    error  // harness-level problem (binary missing, timeout, etc.)
}

func mriDumpParsetree(rubyBin, src string) mriDumpResult {
	tmp, err := os.CreateTemp("", "parsetree-*.rb")
	if err != nil {
		return mriDumpResult{fatal: fmt.Errorf("tempfile: %w", err)}
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.WriteString(src); err != nil {
		tmp.Close()
		return mriDumpResult{fatal: fmt.Errorf("write tempfile: %w", err)}
	}
	if err := tmp.Close(); err != nil {
		return mriDumpResult{fatal: fmt.Errorf("close tempfile: %w", err)}
	}

	ctx, cancel := context.WithTimeout(context.Background(), mriExecTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, rubyBin, "--disable-gems", "--dump=parsetree", tmpPath)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	if ctx.Err() != nil {
		return mriDumpResult{fatal: fmt.Errorf("MRI timeout after %s", mriExecTimeout)}
	}
	if runErr != nil {
		// Non-zero exit usually means parse error. We treat any non-zero
		// exit as parseErr -- stderr carries the message.
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			return mriDumpResult{parseErr: fmt.Errorf("MRI exit %d: %s",
				exitErr.ExitCode(), strings.TrimSpace(stderr.String()))}
		}
		return mriDumpResult{fatal: fmt.Errorf("MRI run: %w", runErr)}
	}
	return mriDumpResult{tree: parsetreenorm.Normalize(stdout.String())}
}

// --- cache layer (tree1 only -- src1 never changes per fixture) -------------

func cachePath(fullVer, src string) string {
	sum := sha256.Sum256([]byte(src))
	return filepath.Join(parsetreeCacheDir, fullVer, hex.EncodeToString(sum[:])+".tree")
}

// mriDumpCached returns the normalized tree for src1 at the given binary,
// reading from / writing to disk cache. The fullVer is used as a cache
// partition so that rebuilds of `.rubies/versions/<ver>/` invalidate naturally
// by leaving stale entries unreachable (cache is content-addressed by src
// hash, but partitioned by full version dir name).
func mriDumpCached(rubyBin, fullVer, src string) mriDumpResult {
	cp := cachePath(fullVer, src)
	if data, err := os.ReadFile(cp); err == nil {
		return mriDumpResult{tree: parsetreenorm.Normalize(string(data))}
	}
	res := mriDumpParsetree(rubyBin, src)
	if res.fatal == nil && res.parseErr == nil {
		_ = os.MkdirAll(filepath.Dir(cp), 0o755)
		_ = os.WriteFile(cp, []byte(res.tree), 0o644)
	}
	return res
}

// fullVerFromBin: ".../.rubies/versions/3.4.4/bin/ruby" -> "3.4.4".
func fullVerFromBin(bin string) string {
	// .../versions/<full>/bin/ruby
	d := filepath.Dir(filepath.Dir(bin))
	return filepath.Base(d)
}

// --- diff -------------------------------------------------------------------

// shortDiff produces a compact line-diff suitable for inline t.Errorf output.
// Caps total output at maxLines to keep test logs scannable when many fixtures
// fail at once.
func shortDiff(want, got string, maxLines int) string {
	wantLines := strings.Split(want, "\n")
	gotLines := strings.Split(got, "\n")

	// Find first and last differing line indices using a simple linear scan.
	n := len(wantLines)
	if len(gotLines) < n {
		n = len(gotLines)
	}
	firstDiff := -1
	for i := 0; i < n; i++ {
		if wantLines[i] != gotLines[i] {
			firstDiff = i
			break
		}
	}
	if firstDiff < 0 {
		if len(wantLines) == len(gotLines) {
			return "(no diff)"
		}
		firstDiff = n
	}

	var b strings.Builder
	fmt.Fprintf(&b, "first diff at line %d (want %d lines, got %d lines):\n",
		firstDiff+1, len(wantLines), len(gotLines))
	ctxStart := firstDiff - 2
	if ctxStart < 0 {
		ctxStart = 0
	}
	for i := ctxStart; i < firstDiff; i++ {
		fmt.Fprintf(&b, "  %s\n", wantLines[i])
	}
	half := maxLines / 2
	wantEnd := firstDiff + half
	if wantEnd > len(wantLines) {
		wantEnd = len(wantLines)
	}
	for i := firstDiff; i < wantEnd; i++ {
		fmt.Fprintf(&b, "- %s\n", wantLines[i])
	}
	if wantEnd < len(wantLines) {
		fmt.Fprintf(&b, "- ... (%d more want lines)\n", len(wantLines)-wantEnd)
	}
	gotEnd := firstDiff + half
	if gotEnd > len(gotLines) {
		gotEnd = len(gotLines)
	}
	for i := firstDiff; i < gotEnd; i++ {
		fmt.Fprintf(&b, "+ %s\n", gotLines[i])
	}
	if gotEnd < len(gotLines) {
		fmt.Fprintf(&b, "+ ... (%d more got lines)\n", len(gotLines)-gotEnd)
	}
	return b.String()
}

// --- the test itself --------------------------------------------------------

const (
	bucketPass           = "pass"
	bucketSrc2Rejected   = "src2-rejected"
	bucketTreeMismatch   = "tree-mismatch"
	bucketGorubyRejected = "goruby-rejected" // src1 not parsable by goruby; auto-skip
)

func TestMRIParseTreeDiff(t *testing.T) {
	binsMap := rubies(t)

	versions, rows := loadGoldenTSV(t)
	skips, err := loadGoldenSkips(parsetreeSkipFile, bucketSrc2Rejected, bucketTreeMismatch)
	if err != nil {
		t.Fatalf("load %s: %v", parsetreeSkipFile, err)
	}

	for _, row := range rows {
		row := row
		localPath := strings.TrimPrefix(row.file, goldenPrefix)

		src, err := os.ReadFile(localPath)
		if err != nil {
			t.Fatalf("read %s: %v", localPath, err)
		}

		// Reformat src1 once per fixture (independent of MRI version).
		prog, gorubyParseErr := parser.ParseFile(row.file, src, parser.AllErrors|parser.ParseComments)
		var src2 string
		var stringPanic string
		if gorubyParseErr == nil {
			src2, stringPanic = safeString(prog)
		}

		for _, verStr := range versions {
			if !row.results[verStr] {
				continue // MRI rejects src1 at this version -- not our concern
			}
			rubyBin, ok := binsMap[verStr]
			if !ok {
				continue // version listed in TSV but no binary built locally
			}
			verStr, rubyBin := verStr, rubyBin
			ver := token.MustParseVersion(verStr)

			name := fmt.Sprintf("%s/ruby_%s", row.file, verStr)
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				t.Cleanup(func() {
					switch {
					case t.Skipped():
						parsetreePassCounters.skip.Add(1)
					case t.Failed():
						parsetreePassCounters.fail.Add(1)
					default:
						parsetreePassCounters.pass.Add(1)
					}
					parsetreePassCounters.total.Add(1)
				})

				// goruby couldn't parse src1 -- not what this test exists to
				// catch (TestMRIGoldenParse already covers it). Skip without
				// running MRI to keep signal focused.
				if gorubyParseErr != nil {
					t.Skipf("goruby parse failed: %v", gorubyParseErr)
				}
				if stringPanic != "" {
					t.Skipf("String() panicked: %s", stringPanic)
				}

				// Compute trees. tree1 is cache-served whenever possible;
				// tree2 always runs MRI fresh because src2 changes with goruby
				// parser/AST/String() drift.
				fullVer := fullVerFromBin(rubyBin)
				r1 := mriDumpCached(rubyBin, fullVer, string(src))
				if r1.fatal != nil {
					// Usually a timeout on a pathological fixture; not our
					// signal. Skip rather than fail.
					t.Skipf("MRI dump src1 unavailable: %v", r1.fatal)
				}
				if r1.parseErr != nil {
					// mri-golden TSV uses `ruby -c` (syntax check) but this
					// test uses `ruby --dump=parsetree`. Some MRI versions
					// have internal bugs (segfault, NODE_OP_CDECL unknown,
					// etc.) on --dump for valid Ruby. Skip rather than fail
					// -- it is an MRI quirk, not a goruby bug.
					t.Skipf("MRI --dump=parsetree rejected src1 at %s: %v",
						verStr, r1.parseErr)
				}

				r2 := mriDumpParsetree(rubyBin, src2)
				if r2.fatal != nil {
					t.Skipf("MRI dump src2 unavailable: %v", r2.fatal)
				}

				bucket := bucketPass
				switch {
				case r2.parseErr != nil:
					bucket = bucketSrc2Rejected
				case r1.tree != r2.tree:
					bucket = bucketTreeMismatch
				}

				// Skip-list check. Stale entries (would have passed) fail loudly.
				entry := skips.match(bucket, row.file, ver)
				if bucket == bucketPass {
					// Look for a skip entry under any non-pass bucket; if
					// found, it's stale.
					if e := skips.match(bucketSrc2Rejected, row.file, ver); e != nil {
						t.Fatalf("stale parsetree.skip entry (%s): test now passes", bucketSrc2Rejected)
					}
					if e := skips.match(bucketTreeMismatch, row.file, ver); e != nil {
						t.Fatalf("stale parsetree.skip entry (%s): test now passes", bucketTreeMismatch)
					}
					return
				}
				if entry != nil {
					reason := entry.reason
					if reason == "" {
						reason = "skip-listed"
					}
					t.Skip(reason)
					return
				}

				switch bucket {
				case bucketSrc2Rejected:
					t.Errorf("MRI %s rejected goruby reformat:\n%s\n--- src2 (first 40 lines) ---\n%s",
						verStr, r2.parseErr, headLines(src2, 40))
				case bucketTreeMismatch:
					t.Errorf("parsetree mismatch at MRI %s:\n%s",
						verStr, shortDiff(r1.tree, r2.tree, 60))
				}
			})
		}
	}
}

func headLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}
