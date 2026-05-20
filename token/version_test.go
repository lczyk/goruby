package token

import (
	"testing"

	"github.com/lczyk/assert"
)

func TestParseVersion(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", "latest"},
		{"3", "3.4"},
		{"2", "2.7"},
		{"4", "4.0"},
		{"1", "1.9"},
		{"3.0", "3.0"},
		{"2.7", "2.7"},
		{"1.9", "1.9"},
	}
	for _, tt := range tests {
		v, err := ParseVersion(tt.input)
		assert.NoError(t, err, "ParseVersion(%q)", tt.input)
		assert.Equal(t, v.String(), tt.want)
	}
}

func TestParseVersionErrors(t *testing.T) {
	for _, input := range []string{"abc", "3.1.2", "99"} {
		_, err := ParseVersion(input)
		assert.That(t, err != nil, "ParseVersion(%q): expected error", input)
	}
}

func TestVersionAtLeast(t *testing.T) {
	tests := []struct {
		v, other string
		want     bool
	}{
		{"", "4.0", true},
		{"3.0", "2.7", true},
		{"3.0", "3.0", true},
		{"3.0", "3.1", false},
		{"2.7", "3.0", false},
		{"1.9", "1.9", true},
	}
	for _, tt := range tests {
		v := MustParseVersion(tt.v)
		other := MustParseVersion(tt.other)
		assert.Equal(t, v.AtLeast(other), tt.want)
	}
}

func TestLatestVersion(t *testing.T) {
	v := LatestVersion()
	assert.That(t, v.Major >= 4, "LatestVersion major = %d, want >= 4", v.Major)
	assert.That(t, v.IsSet(), "LatestVersion should be set")
}

func TestVersionIsSet(t *testing.T) {
	unset := RubyVersion{}
	assert.That(t, !unset.IsSet(), "zero value should not be set")
	v := MustParseVersion("3.0")
	assert.That(t, v.IsSet(), "parsed version should be set")
}

func TestMustParseVersionPanic(t *testing.T) {
	assert.Panic(t, func() { MustParseVersion("invalid") }, nil)
}

func TestParseVersionUnknownMajor(t *testing.T) {
	_, err := ParseVersion("99")
	assert.That(t, err != nil, "expected error for unknown major")
}

func TestParseVersionInvalidMinor(t *testing.T) {
	_, err := ParseVersion("3.x")
	assert.That(t, err != nil, "expected error for invalid minor")
}

func TestVersionCompare(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"3.0", "3.0", 0},
		{"3.1", "3.0", 1},
		{"3.0", "3.1", -1},
		{"4.0", "3.4", 1},
		{"2.7", "3.0", -1},
		{"", "", 0},
		{"", "4.0", 1},
		{"4.0", "", -1},
	}
	for _, tt := range tests {
		a := MustParseVersion(tt.a)
		b := MustParseVersion(tt.b)
		assert.Equal(t, a.Compare(b), tt.want)
	}
}
