package compare

import (
	"errors"
	"reflect"
)

// Errors reports whether err and target are structurally equivalent: both nil,
// or same dynamic type and equal Error() string.
//
// This does NOT walk the errors.Is wrap chain. Use ErrorsIs for that.
func Errors(err error, target error) bool {
	if err == nil || target == nil {
		return err == nil && target == nil
	}
	return reflect.TypeOf(err) == reflect.TypeOf(target) && err.Error() == target.Error()
}

// ErrorsIs reports whether err matches target under errors.Is semantics
// (identity or wrap chain). Both nil counts as match.
func ErrorsIs(err error, target error) bool {
	if err == nil && target == nil {
		return true
	}
	return errors.Is(err, target)
}

func Arrays[T comparable](a []T, b []T) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func Maps[T comparable, V comparable](a map[T]V, b map[T]V) bool {
	if len(a) != len(b) {
		return false
	}
	var vb V
	var ok bool

	// NOTE: the range on a map is in random order
	for k, va := range a {
		// Check if key exists in b
		if vb, ok = b[k]; !ok {
			return false
		}
		// Check if value is the same
		if va != vb {
			return false
		}
	}

	// All keys of a exist in b, and a and b have the same length, hence they
	// must have the same keys
	return true
}

// Check if two arrays are equal, regardless of the order of the elements.
func ArraysUnordered[T comparable](a []T, b []T) bool {
	if len(a) != len(b) {
		return false
	}
	am := make(map[T]int) // map from element to count
	for _, e := range a {
		am[e]++
	}
	// Iterate over b, decrementing the count of each element in am.
	for _, e := range b {
		if am[e] == 0 {
			return false
		}
		am[e]--
	}
	return true
}
