package evaluator

import (
	"testing"

	"github.com/lczyk/assert"
)

// TestBignumPromotionMul verifies that 2**70 produces the exact
// big-integer result, not an int64 wrap. 2**70 > int64.max.
func TestBignumPromotionMul(t *testing.T) {
	got := run(t, "puts 2**70")
	assert.Equal(t, "1180591620717411303424\n", got)
}

// TestBignumPromotionAdd verifies addition across the int64 boundary.
// int64.max + 1 must produce 9223372036854775808 exactly.
func TestBignumPromotionAdd(t *testing.T) {
	got := run(t, "puts 9223372036854775807 + 1")
	assert.Equal(t, "9223372036854775808\n", got)
}

// TestBignumMulChain checks that repeated multiplication doesn't lose
// precision once it enters bignum territory. 10**30 has 31 digits.
func TestBignumMulChain(t *testing.T) {
	got := run(t, "puts 10**30")
	assert.Equal(t, "1000000000000000000000000000000\n", got)
}

// TestBignumDemotion: bignum arithmetic whose result fits in int64
// should land back in the inline path (so subsequent comparisons hit
// the fast branch). Hard to test directly w/out reflection; we settle
// for verifying the value renders correctly and compares == to a
// freshly-built small Integer.
func TestBignumDemotion(t *testing.T) {
	got := run(t, "x = (10**40) / (10**40 - 41); puts x; puts x == 1")
	assert.Equal(t, "1\ntrue\n", got)
}

// TestBignumCompare: ordering across the int64 boundary works both
// directions and rubyEqual returns true for equal bignums.
func TestBignumCompare(t *testing.T) {
	got := run(t, "a = 10**20; b = 10**20 + 1; puts a < b; puts a == 10**20")
	assert.Equal(t, "true\ntrue\n", got)
}
