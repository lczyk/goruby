package token

import (
	"fmt"
	"strconv"
	"strings"
)

// RubyVersion represents a ruby major.minor version for parser targeting.
// The zero value means "latest" (no version constraint).
type RubyVersion struct {
	Major int
	Minor int
	set   bool
}

// knownBoundaries lists every major.minor where parser-visible syntax changed.
// Sorted ascending. Used to resolve bare major versions to the highest known
// minor for that major.
var knownBoundaries = []RubyVersion{
	{1, 9, true},
	{2, 0, true},
	{2, 1, true},
	{2, 3, true},
	{2, 5, true},
	{2, 6, true},
	{2, 7, true},
	{3, 0, true},
	{3, 1, true},
	{3, 2, true},
	{3, 4, true},
	{4, 0, true},
}

// LatestVersion returns the highest known ruby version.
func LatestVersion() RubyVersion {
	return knownBoundaries[len(knownBoundaries)-1]
}

// ParseVersion parses a ruby version string. Accepted forms:
//   - ""     -> zero value (latest)
//   - "3"    -> highest known minor for major 3
//   - "3.1"  -> exactly 3.1
//   - "2.7"  -> exactly 2.7
func ParseVersion(s string) (RubyVersion, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return RubyVersion{}, nil
	}

	parts := strings.SplitN(s, ".", 3)
	if len(parts) > 2 {
		return RubyVersion{}, fmt.Errorf("ruby version %q: patch component not supported, use major.minor", s)
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return RubyVersion{}, fmt.Errorf("ruby version %q: invalid major: %w", s, err)
	}

	if len(parts) == 1 {
		return resolveLatestMinor(major)
	}

	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return RubyVersion{}, fmt.Errorf("ruby version %q: invalid minor: %w", s, err)
	}

	return RubyVersion{Major: major, Minor: minor, set: true}, nil
}

// MustParseVersion is like ParseVersion but panics on error.
func MustParseVersion(s string) RubyVersion {
	v, err := ParseVersion(s)
	if err != nil {
		panic(err)
	}
	return v
}

func resolveLatestMinor(major int) (RubyVersion, error) {
	var best RubyVersion
	found := false
	for _, v := range knownBoundaries {
		if v.Major == major {
			best = v
			found = true
		}
	}
	if !found {
		return RubyVersion{}, fmt.Errorf("ruby version %q: unknown major version", strconv.Itoa(major))
	}
	return best, nil
}

// IsSet returns true if a version was explicitly configured (not the zero
// value / "latest" default).
func (v RubyVersion) IsSet() bool { return v.set }

// AtLeast returns true if v >= other (or if v is unset, meaning latest).
func (v RubyVersion) AtLeast(other RubyVersion) bool {
	if !v.set {
		return true
	}
	if v.Major != other.Major {
		return v.Major > other.Major
	}
	return v.Minor >= other.Minor
}

func (v RubyVersion) String() string {
	if !v.set {
		return "latest"
	}
	return fmt.Sprintf("%d.%d", v.Major, v.Minor)
}

// Compare returns -1, 0, or 1 comparing v to other. Unset (latest) is
// greater than any set version.
func (v RubyVersion) Compare(other RubyVersion) int {
	if !v.set && !other.set {
		return 0
	}
	if !v.set {
		return 1
	}
	if !other.set {
		return -1
	}
	if v.Major != other.Major {
		if v.Major < other.Major {
			return -1
		}
		return 1
	}
	if v.Minor != other.Minor {
		if v.Minor < other.Minor {
			return -1
		}
		return 1
	}
	return 0
}
