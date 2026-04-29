package object

import (
	"sort"
	"testing"

	"github.com/lczyk/assert"
)

func TestGetMethods(t *testing.T) {
	contextMethods := map[string]RubyMethod{
		"foo": newMethod(nil),
		"bar": newMethod(nil),
	}
	myClass := &class{
		instanceMethods: NewMethodSet(contextMethods),
	}

	t.Run("no super methods", func(t *testing.T) {
		result := getMethods(myClass, false)
		var methods []string
		for i, elem := range result.Elements {
			sym, ok := elem.(*Symbol)
			assert.That(t, ok, "Expected all elements to be symbols, got %T at index %d", elem, i)
			methods = append(methods, sym.Inspect())
		}
		assert.Equal(t, len(contextMethods), len(result.Elements))
		sort.Strings(methods)
		expectedMethods := []string{":foo", ":bar"}
		sort.Strings(expectedMethods)
		for i := range expectedMethods {
			assert.Equal(t, expectedMethods[i], methods[i])
		}
	})

	t.Run("add super methods", func(t *testing.T) {
		result := getMethods(myClass, true)
		var methods []string
		for i, elem := range result.Elements {
			sym, ok := elem.(*Symbol)
			assert.That(t, ok, "Expected all elements to be symbols, got %T at index %d", elem, i)
			methods = append(methods, sym.Inspect())
		}
		someExpectedMethods := []string{":foo", ":bar", ":raise", ":==", ":!="}
		sort.Strings(someExpectedMethods)
		for _, method := range someExpectedMethods {
			var found bool = false
			for _, m := range methods {
				if method == m {
					found = true
					break
				}
			}
			assert.That(t, found, "Expected method %s to be found in %s", method, methods)
		}
	})
}
