package assert

import (
	"fmt"
	"reflect"
	"regexp"
	"runtime"
	"testing"

	"github.com/lczyk/assert/compare"
)

func getParentInfo(N int) (string, int) {
	parent, _, _, _ := runtime.Caller(1 + N)
	return runtime.FuncForPC(parent).FileLine(parent)
}

// convert 'args ...any' to the assertion message
// internal utility so we don't use variadics to make the calls a bit more consistent
func argsToMessage(default_func func() string, args []any) string {
	var msg string
	if len(args) == 0 {
		msg = default_func()
	} else {
		switch args[0].(type) {
		case string:
			msg = args[0].(string)
			msg = fmt.Sprintf(msg, args[1:]...)
		default:
			msg = fmt.Sprintf("%v", args)
		}
	}
	return msg
}

type anyErr struct{}

func (anyErr) Error() string { return "<any error>" }

const nestedAssertParent = 2

func assert(t testing.TB, N int, predicate bool, args []any) {
	t.Helper()
	if !predicate {
		file, line := getParentInfo(N)
		msg := argsToMessage(func() string { return "assertion failed" }, args)
		t.Errorf(msg+" in %s:%d", file, line)
	}
}

func assert_error(t testing.TB, N int, err error, expected any, args []any) {
	t.Helper()
	var msg_fun func() string

	// AnyError sentinel: any non-nil err passes; nil err fails.
	if e, ok := expected.(error); ok && e == AnyError {
		if err == nil {
			msg_fun = func() string { return "expected an error, got nil" }
		}
		if msg_fun != nil {
			msg := argsToMessage(msg_fun, args)
			file, line := getParentInfo(N)
			t.Errorf(msg+" in %s:%d", file, line)
		}
		return
	}

	switch expected := expected.(type) {
	case string:
		if err == nil {
			msg_fun = func() string {
				return fmt.Sprintf("expected error to match '%s', got no error (nil)", expected)
			}
		} else {
			// Regex pattern matched as substring against err.Error().
			re := regexp.MustCompile(expected)
			if !re.MatchString(err.Error()) {
				msg_fun = func() string {
					return fmt.Sprintf("expected error to match '%s', got '%v' (%T)", expected, err, err)
				}
			}
		}

	case error:
		if expected == nil {
			if err != nil {
				msg_fun = func() string {
					return fmt.Sprintf("expected no error, got '%v' (%T)", err, err)
				}
			}
		} else {
			if err == nil {
				msg_fun = func() string {
					return fmt.Sprintf("expected error '%v' (%T), got no error (nil)", expected, expected)
				}
			} else {
				if !compare.Errors(err, expected) && !compare.ErrorsIs(err, expected) {
					msg_fun = func() string {
						return fmt.Sprintf("expected error '%v' (%T), got '%v' (%T)", expected, expected, err, err)
					}
				}
			}
		}
	case nil:
		if err != nil {
			msg_fun = func() string {
				return fmt.Sprintf("expected no error, got '%v' (%T)", err, err)
			}
		}
	case *regexp.Regexp:
		if err == nil {
			msg_fun = func() string {
				return fmt.Sprintf("expected error '%v' (%T), got no error (nil)", expected, expected)
			}
		} else {
			re := regexp.MustCompile(expected.String())
			if !re.MatchString(err.Error()) {
				msg_fun = func() string {
					return fmt.Sprintf("expected error to match '%s', got '%v' (%T)", expected, err, err)
				}
			}
		}
	default:
		panic("expected type is not an error or string")

	}

	if msg_fun != nil {
		msg := argsToMessage(msg_fun, args)
		file, line := getParentInfo(N)
		t.Errorf(msg+" in %s:%d", file, line)
	}
}

func equal_cmp[T any](t testing.TB, N int, a T, b T, comparator func(T, T) bool, args []any) {
	t.Helper()
	if comparator(a, b) {
		return
	}
	file, line := getParentInfo(N)
	msg := argsToMessage(func() string {
		return fmt.Sprintf("expected '%v' (%T) == '%v' (%T)", a, a, b, b)
	}, args)
	t.Errorf(msg+" in %s:%d", file, line)
}

func equal_cmp_any(t testing.TB, N int, a any, b any, comparator func(any, any) bool, args []any) {
	defer func() {
		if r := recover(); r != nil {
			// If the comparator panics, we want to catch it and report it as a test failure.
			file, line := getParentInfo(4)
			t.Errorf("Comparator panicked: %v in %s:%d", r, file, line)
		}
	}()
	t.Helper()
	if comparator(a, b) {
		return
	}
	file, line := getParentInfo(N)
	msg := argsToMessage(func() string {
		return fmt.Sprintf("expected '%v' (%T) == '%v' (%T)", a, a, b, b)
	}, args)
	t.Errorf(msg+" in %s:%d", file, line)
}

// isNil handles the typed-nil-in-interface case: var p *T = nil; var i any = p
// — `i != nil` is true but the underlying value is nil.
func isNil(x any) bool {
	if x == nil {
		return true
	}
	v := reflect.ValueOf(x)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice, reflect.UnsafePointer:
		return v.IsNil()
	}
	return false
}

// Check that the type of obj is T.
func assert_type[T any](t testing.TB, N int, obj any, args ...any) T {
	t.Helper()
	if obj_T, ok := obj.(T); ok {
		return obj_T
	} else {
		file, line := getParentInfo(N)
		msg := argsToMessage(func() string {
			return fmt.Sprintf("expected type %T, got %T", (*T)(nil), obj)
		}, args)
		t.Errorf(msg+" in %s:%d", file, line)
	}
	return *new(T)
}
