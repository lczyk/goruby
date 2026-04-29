# assert

[![lint_and_test](https://github.com/lczyk/assert/actions/workflows/lint_and_test.yml/badge.svg)](https://github.com/lczyk/assert/actions/workflows/lint_and_test.yml)

Mini package to make writing tests in golang a bit neater -- a little bit more like `pytest`.

For example this:

```go
func TestExample(t *testing.T) {
	a := 1
	b := 2
	if a == b {
		t.Errorf("Expected %d to not equal %d", a, b)
	}
}
```

becomes:

```go
func TestExample(t *testing.T) {
	a := 1
	b := 2
	assert.That(t, a != b)
}
```

Not a big difference but over the course of a large test suite it adds up.