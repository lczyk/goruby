package evaluator

import (
	"bytes"
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

// TestPrime: require 'prime' makes Prime usable. prime_division
// produces [[prime, exponent], ...] pairs matching MRI. Bootstrap
// installs Prime eagerly so the require is technically a no-op, but
// either way the constant resolves and factorisation works.
func TestPrime(t *testing.T) {
	out, err := runErr(t, "require 'prime'; p Prime.prime_division(12)")
	assert.Equal(t, "", err)
	assert.Equal(t, "[[2, 2], [3, 1]]\n", out)
}

// TestGapDateRequire mirrors TestGapPrimeRequire for the Date stdlib.
func TestGapDateRequire(t *testing.T) {
	out, err := runErr(t, "require 'date'; puts 'loaded'; Date.today")
	assert.Equal(t, "loaded\n", out)
	if !strings.Contains(err, "uninitialized constant Date") {
		t.Errorf("expected uninitialized-constant NameError on Date, got %q", err)
	}
}

// TestSortByEnumerator: sort_by without a block returns an
// Enumerator; the chained .with_index { |elem, idx| ... } applies the
// block to each (elem, idx) pair, uses the result as the sort key,
// and returns the sorted array. Stable on ties because tuple
// comparison includes the index. Underpins alice's stable_sort and
// stable_sort_by reopenings of Enumerable.
func TestSortByEnumerator(t *testing.T) {
	out, err := runErr(t, "p [3,1,2].sort_by.with_index { |x, i| [x, i] }")
	assert.Equal(t, "", err)
	assert.Equal(t, "[1, 2, 3]\n", out)

	// Stability check: tie-broken by the index produced by with_index.
	out, err = runErr(t, "p [[5,2],[5,1],[3,9]].sort_by.with_index { |x, i| [x, i] }")
	assert.Equal(t, "", err)
	assert.Equal(t, "[[3, 9], [5, 1], [5, 2]]\n", out)
}

// TestStringScrub: scrub replaces invalid UTF-8 byte sequences with
// the given replacement (default is the unicode replacement char).
// Valid input passes through unchanged.
func TestStringScrub(t *testing.T) {
	out, err := runErr(t, `puts "abc".scrub('?')`)
	assert.Equal(t, "", err)
	assert.Equal(t, "abc\n", out)

	// Bare 0xC3 is the start of a 2-byte UTF-8 sequence but missing the
	// continuation byte -- invalid. Single '?' replaces it.
	out, err = runErr(t, `puts "\xC3".scrub('?').bytes.length`)
	assert.Equal(t, "", err)
	assert.Equal(t, "1\n", out)

	// Default replacement is the unicode replacement character (U+FFFD,
	// three bytes 0xEF 0xBF 0xBD in UTF-8). Single invalid byte gets a
	// single replacement -> length 3.
	out, err = runErr(t, `puts "\xC3".scrub.bytes.length`)
	assert.Equal(t, "", err)
	assert.Equal(t, "3\n", out)
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
