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

// runErr is run() but reports the error string instead of asserting
// NoError, so probes can pin down a missing feature by message rather
// than failing the test on every uncovered surface.
func runErr(t *testing.T, src string) (string, string) {
	t.Helper()
	target := token.MustParseVersion("2.6")
	prog, perr := parser.ParseFile("<gap-probe>", []byte(src), 0, parser.WithVersion(target))
	if perr != nil {
		return "", "parse: " + perr.Error()
	}
	var stdout bytes.Buffer
	env := object.NewMainEnvironment(object.WithVersion(target), object.WithStdout(&stdout))
	if _, err := Eval(prog, env); err != nil {
		return stdout.String(), err.Error()
	}
	return stdout.String(), ""
}

// TestGapPrimeRequire pins the current behaviour of `require 'prime'`:
// it succeeds (stub), but Prime constant doesn't exist. A program that
// goes on to reference Prime fails with NameError. Test serves as the
// canary -- when we ship a real Prime module, swap the assertions to
// the actual factorisation.
func TestGapPrimeRequire(t *testing.T) {
	out, err := runErr(t, "require 'prime'; puts 'loaded'; Prime.prime_division(12)")
	assert.Equal(t, "loaded\n", out, "require stub returns true and lets execution continue")
	if !strings.Contains(err, "uninitialized constant Prime") {
		t.Errorf("expected uninitialized-constant NameError on Prime, got %q", err)
	}
}

// TestGapDateRequire mirrors TestGapPrimeRequire for the Date stdlib.
func TestGapDateRequire(t *testing.T) {
	out, err := runErr(t, "require 'date'; puts 'loaded'; Date.today")
	assert.Equal(t, "loaded\n", out)
	if !strings.Contains(err, "uninitialized constant Date") {
		t.Errorf("expected uninitialized-constant NameError on Date, got %q", err)
	}
}

// TestGapSortByEnumerator pins Enumerator chaining: `sort_by` without a
// block should return an Enumerator that further methods (with_index,
// each, lazy, ...) can chain on. We don't implement Enumerator yet, so
// sort_by w/out a block raises. Used by alice's stable_sort reopening.
func TestGapSortByEnumerator(t *testing.T) {
	_, err := runErr(t, "[3,1,2].sort_by.with_index { |x, i| [x, i] }")
	if err == "" {
		t.Errorf("expected error -- sort_by w/out block should return Enumerator (not implemented)")
	}
	t.Logf("current behaviour: %s", err)
}

// TestGapStringScrub pins the current scrub behaviour: with a
// replacement arg, MRI replaces invalid byte sequences with the
// replacement. Our stub returns the receiver unchanged, regardless of
// validity.  Pin so a future scrub implementation that actually
// scrubs invalid bytes can update this fixture to the real check.
func TestGapStringScrub(t *testing.T) {
	out, err := runErr(t, `puts "abc".scrub('?')`)
	assert.Equal(t, "", err)
	// Plain ASCII -- no invalid bytes, scrub is a no-op even under MRI.
	assert.Equal(t, "abc\n", out)

	// Invalid byte sequence: our stub leaves it as-is. MRI would
	// replace with the given character.
	_, err = runErr(t, `s = "\xC3".dup.force_encoding('UTF-8'); puts s.scrub('?').bytes.length`)
	t.Logf("scrub('?') on invalid bytes: out err=%q -- mri would print 1 (single '?'); we print 1 (untouched, same length by accident) or differ", err)
}

// TestGapAlicePrime drives the gap_prime.alice probe through the
// alice interpreter end-to-end. MRI prints a single byte (0x02) by
// pushing 2, decomposing via Prime.prime_division, and raw-outputting
// the factor. goruby raises NameError on Prime because we ship a
// stub Kernel#require that no-ops for `prime` but never installs the
// Prime constant. When a real Prime arrives, swap the assertion to
// "\x02" and the assertion-flip will document the gap closing.
func TestGapAlicePrime(t *testing.T) {
	repoRoot, err := filepath.Abs("..")
	assert.NoError(t, err, "repo root")
	interp := filepath.Join(repoRoot, "internal/integrationtest/testdata/gems/alice/interpreter.rb")
	interpSrc, err := os.ReadFile(interp)
	assert.NoError(t, err, "read interpreter.rb")
	probe := filepath.Join(repoRoot, "internal/integrationtest/testdata/esolang_tests/alice/gaps/gap_prime.alice")
	assert.NoError(t, mustExist(probe), "probe exists")

	target := token.MustParseVersion("2.6")
	prog, perr := parser.ParseFile(interp, interpSrc, 0, parser.WithVersion(target))
	assert.NoError(t, perr, "parse interpreter.rb")

	var stdout, stderr bytes.Buffer
	env := object.NewMainEnvironment(
		object.WithVersion(target),
		object.WithStdout(&stdout),
		object.WithStderr(&stderr),
		object.WithStdin(bytes.NewReader(nil)),
		object.WithARGV([]string{probe}),
	)
	_, err = Eval(prog, env)
	if err == nil {
		t.Errorf("expected NameError on Prime under goruby; eval returned nil. stdout=%q", stdout.String())
		return
	}
	if !strings.Contains(err.Error(), "uninitialized constant Prime") {
		t.Errorf("expected uninitialized-constant NameError on Prime, got %q", err.Error())
	}
	t.Logf("current behaviour: %v (mri prints byte 0x02)", err)
}

func mustExist(p string) error {
	_, err := os.Stat(p)
	return err
}

// TestGapEncodingTracking confirms that String#encoding returns the
// Encoding::UTF_8 sentinel regardless of any force_encoding call. We
// don't track per-string encodings, so this is a faithful pin: any
// code that branches on `str.encoding == Encoding::BINARY` would take
// the wrong branch under goruby today.
func TestGapEncodingTracking(t *testing.T) {
	out, err := runErr(t, `
		s = "abc"
		puts s.encoding.name
		s2 = s.dup.force_encoding(Encoding::ASCII_8BIT)
		puts s2.encoding.name
	`)
	assert.Equal(t, "", err)
	// Both should be UTF-8 under our stub (we always report UTF-8).
	assert.Equal(t, "UTF-8\nUTF-8\n", out,
		"per-string encoding tracking not implemented: both report UTF-8")
}
