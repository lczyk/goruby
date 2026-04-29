package assert

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/lczyk/assert/compare"
)

func That(t testing.TB, predicate bool, args ...any) {
	t.Helper()
	assert(t, nestedAssertParent, predicate, args)
}

func Equal[T comparable](t testing.TB, a T, b T) {
	t.Helper()
	assert(t, nestedAssertParent, a == b, []any{"expected '%v' (%T) == '%v' (%T)", a, a, b, b})
}

func NotEqual[T comparable](t testing.TB, a T, b T) {
	t.Helper()
	assert(t, nestedAssertParent, a != b, []any{"expected '%v' (%T) != '%v' (%T)", a, a, b, b})
}

// Assert that error is nil.
func NoError(t testing.TB, err error, args ...any) {
	t.Helper()
	assert_error(t, nestedAssertParent, err, nil, args)
}

// AnyError matches any non-nil error when passed as the expected arg to Error.
// Use this sentinel rather than the empty string for clarity at call sites.
var AnyError error = anyErr{}

// Error asserts that err is non-nil and matches expected.
//
// expected may be:
//   - nil: passes only if err is nil (equivalent to NoError)
//   - AnyError: passes for any non-nil err
//   - error: structural match (same dynamic type and Error() string),
//     OR errors.Is wrap-chain match. ErrorIs is the strict-wrap-chain variant.
//   - string: regex pattern, matched as a substring against err.Error()
//     (regexp.MustCompile(s).MatchString(err.Error())). Note that this is
//     NOT an exact-equality check — anchor with ^...$ if you need that.
//     Special characters (.()?+ etc.) are interpreted as regex metacharacters.
//   - *regexp.Regexp: regex pattern, matched as a substring against err.Error()
func Error(t testing.TB, err error, expected any, args ...any) {
	t.Helper()
	assert_error(t, nestedAssertParent, err, expected, args)
}

// ErrorIs asserts that err matches expected via errors.Is semantics
// (identity or wrap chain). Use Error for structural type+message match.
func ErrorIs(t testing.TB, err error, expected error, args ...any) {
	t.Helper()
	if compare.ErrorsIs(err, expected) {
		return
	}
	file, line := getParentInfo(1)
	msg := argsToMessage(func() string {
		if err == nil {
			return fmt.Sprintf("expected error '%v' (%T), got no error (nil)", expected, expected)
		}
		if expected == nil {
			return fmt.Sprintf("expected no error, got '%v' (%T)", err, err)
		}
		return fmt.Sprintf("expected errors.Is(%v, %v) to be true, got err='%v' (%T)", err, expected, err, err)
	}, args)
	t.Errorf(msg+" in %s:%d", file, line)
}

// Compare two values using a custom comparator function.
func EqualCmp[T any](t testing.TB, a T, b T, comparator func(T, T) bool, args ...any) {
	t.Helper()
	equal_cmp(t, nestedAssertParent, a, b, comparator, args)
}

// Compare two values of any type using a custom comparator function.
// This is a more generic version of EqualCmp, but it is less type-safe.
// The comparator function is responsible for type assertions.
func EqualCmpAny(t testing.TB, a any, b any, comparator func(any, any) bool, args ...any) {
	t.Helper()
	equal_cmp_any(t, nestedAssertParent, a, b, comparator, args)
}

// Compare two arrays for element-wise equality.
func EqualArrays[T comparable](t testing.TB, a []T, b []T, args ...any) {
	t.Helper()
	equal_cmp(t, nestedAssertParent, a, b, compare.Arrays, args)
}

// Compare two maps for key/value equality.
func EqualMaps[T comparable, V comparable](t testing.TB, a map[T]V, b map[T]V, args ...any) {
	t.Helper()
	equal_cmp(t, nestedAssertParent, a, b, compare.Maps, args)
}

func EqualArraysUnordered[T comparable](t testing.TB, a []T, b []T, args ...any) {
	t.Helper()
	equal_cmp(t, nestedAssertParent, a, b, compare.ArraysUnordered, args)
}

// Assert that x is nil. Handles typed-nil-in-interface (e.g. (*T)(nil) inside any).
func Nil(t testing.TB, x any, args ...any) {
	t.Helper()
	if isNil(x) {
		return
	}
	file, line := getParentInfo(1)
	msg := argsToMessage(func() string { return fmt.Sprintf("expected nil, got '%v' (%T)", x, x) }, args)
	t.Errorf(msg+" in %s:%d", file, line)
}

// Assert that x is not nil. Handles typed-nil-in-interface.
func NotNil(t testing.TB, x any, args ...any) {
	t.Helper()
	if !isNil(x) {
		return
	}
	file, line := getParentInfo(1)
	msg := argsToMessage(func() string { return fmt.Sprintf("expected non-nil, got nil (%T)", x) }, args)
	t.Errorf(msg+" in %s:%d", file, line)
}

// Assert that len(x) == n. x must be array, chan, map, slice, or string.
func Len(t testing.TB, x any, n int, args ...any) {
	t.Helper()
	v := reflect.ValueOf(x)
	switch v.Kind() {
	case reflect.Array, reflect.Chan, reflect.Map, reflect.Slice, reflect.String:
	default:
		panic(fmt.Sprintf("Len: unsupported kind %s", v.Kind()))
	}
	got := v.Len()
	if got == n {
		return
	}
	file, line := getParentInfo(1)
	msg := argsToMessage(func() string { return fmt.Sprintf("expected len %d, got len %d: %v", n, got, x) }, args)
	t.Errorf(msg+" in %s:%d", file, line)
}

func Type[T any](t testing.TB, obj any, args ...any) T {
	t.Helper()
	return assert_type[T](t, nestedAssertParent, obj, args...)
}

func EqualLineByLine(t testing.TB, a string, b string) {
	t.Helper()
	// Ignore a single trailing newline on either side so comparing file-like
	// content to a literal doesn't fail on an incidental EOF newline.
	a = strings.TrimSuffix(a, "\n")
	b = strings.TrimSuffix(b, "\n")
	a_lines := strings.Split(a, "\n")
	b_lines := strings.Split(b, "\n")
	assert(t, nestedAssertParent, len(a_lines) == len(b_lines), []any{"expected '%d' lines, got '%d'", len(a_lines), len(b_lines)})
	if len(a_lines) != len(b_lines) {
		return // no point in checking the lines if the number of lines is different
	}
	for i := range a_lines {
		assert(t, nestedAssertParent, a_lines[i] == b_lines[i], []any{"expected line %d to be '%s', got '%s'", i + 1, a_lines[i], b_lines[i]})
	}
}

func ContainsString(t testing.TB, haystack string, needle string) {
	t.Helper()
	assert(t, nestedAssertParent, strings.Contains(haystack, needle), []any{"expected needle string '%s' to be in a haystack string '%s'", needle, haystack})
}

func Panic(t testing.TB, f func(), f_recover func(t testing.TB, rec any), args ...any) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			if f_recover != nil {
				f_recover(t, r)
			}
			return
		}
		assert(t, nestedAssertParent+1, false, []any{"expected panic, but no panic occurred"})
	}()
	f()
}
