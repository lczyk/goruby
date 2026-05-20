package dumpfmt

import (
	"bytes"
	"strings"
	"testing"

	"github.com/lczyk/assert"
)

func TestParseFormat(t *testing.T) {
	cases := []struct {
		in   string
		want Format
	}{
		{"", FormatYAML},
		{"yaml", FormatYAML},
		{"json", FormatJSON},
	}
	for _, c := range cases {
		got, err := ParseFormat(c.in)
		assert.NoError(t, err, "ParseFormat(%q)", c.in)
		assert.Equal(t, c.want, got, "ParseFormat(%q)", c.in)
	}

	_, err := ParseFormat("xml")
	assert.Error(t, err, "unknown format")
}

func TestEncodeYAMLScalar(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{nil, "null\n"},
		{42, "42\n"},
		{int64(-7), "-7\n"},
		{uint32(9), "9\n"},
		{true, "true\n"},
		{false, "false\n"},
		{"hello", "hello\n"},
		{"with spaces", "with spaces\n"},
		{"", "\"\"\n"},
		{"true", "\"true\"\n"},
		{"42", "\"42\"\n"},
		{"a:b", "\"a:b\"\n"},
		{"line\nbreak", "\"line\\nbreak\"\n"},
	}
	for _, c := range cases {
		var buf bytes.Buffer
		err := Encode(&buf, FormatYAML, c.in)
		assert.NoError(t, err, "Encode(%v)", c.in)
		assert.Equal(t, c.want, buf.String(), "Encode(%v)", c.in)
	}
}

func TestEncodeYAMLMap(t *testing.T) {
	in := map[string]any{
		"name":  "ada",
		"age":   42,
		"flags": []any{"a", "b"},
	}
	var buf bytes.Buffer
	err := Encode(&buf, FormatYAML, in)
	assert.NoError(t, err)
	got := buf.String()
	// Sort is alphabetical (no _type here) so age first.
	assert.Equal(t, "age: 42\nflags:\n  - a\n  - b\nname: ada\n", got)
}

func TestEncodeYAMLPutsTypeFirst(t *testing.T) {
	in := map[string]any{
		"_type": "*ast.Identifier",
		"Value": "x",
	}
	var buf bytes.Buffer
	err := Encode(&buf, FormatYAML, in)
	assert.NoError(t, err)
	got := buf.String()
	assert.That(t, strings.HasPrefix(got, "_type:"), "expected _type first, got: %q", got)
}

func TestEncodeYAMLNestedSequenceOfMaps(t *testing.T) {
	in := map[string]any{
		"items": []any{
			map[string]any{"k": 1},
			map[string]any{"k": 2},
		},
	}
	var buf bytes.Buffer
	err := Encode(&buf, FormatYAML, in)
	assert.NoError(t, err)
	want := "items:\n  - k: 1\n  - k: 2\n"
	assert.Equal(t, want, buf.String())
}

func TestEncodeJSONMatchesStdlibShape(t *testing.T) {
	in := map[string]any{"a": 1, "b": []any{true, false}}
	var buf bytes.Buffer
	err := Encode(&buf, FormatJSON, in)
	assert.NoError(t, err)
	got := buf.String()
	assert.ContainsString(t, got, `"a": 1`)
	assert.ContainsString(t, got, `"b": [`)
	assert.That(t, strings.HasSuffix(got, "\n"))
}

func TestEncodeStreamJSONIsJSONL(t *testing.T) {
	items := []any{
		map[string]any{"pos": 0, "type": "IDENT"},
		map[string]any{"pos": 2, "type": "+"},
	}
	var buf bytes.Buffer
	err := EncodeStream(&buf, FormatJSON, items)
	assert.NoError(t, err)
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	assert.Equal(t, 2, len(lines))
	for _, ln := range lines {
		assert.That(t, strings.HasPrefix(ln, "{") && strings.HasSuffix(ln, "}"), "line should be a json object: %q", ln)
	}
}

func TestEncodeStreamYAMLIsBlockSequence(t *testing.T) {
	items := []any{
		map[string]any{"pos": 0, "type": "IDENT"},
		map[string]any{"pos": 2, "type": "+"},
	}
	var buf bytes.Buffer
	err := EncodeStream(&buf, FormatYAML, items)
	assert.NoError(t, err)
	got := buf.String()
	assert.That(t, strings.HasPrefix(got, "- "), "first byte should be `- `: %q", got)
	assert.Equal(t, 2, strings.Count(got, "- pos:"), "expected two `- pos:` openers")
}

type leaf struct {
	Name   string
	Count  int
	hidden bool
}

type branch struct {
	Label    string
	Children []*leaf
	Misc     map[string]int
}

func TestTypedTreeAddsTypeTagAndFiltersZero(t *testing.T) {
	root := &branch{
		Label: "root",
		Children: []*leaf{
			{Name: "a", Count: 1},
			{Name: "b"},
		},
		Misc: map[string]int{"x": 9},
	}
	tree := TypedTree(root)
	m, ok := tree.(map[string]any)
	assert.That(t, ok, "expected map root")
	assert.Equal(t, "*dumpfmt.branch", m["_type"])
	assert.Equal(t, "root", m["Label"])

	children, ok := m["Children"].([]any)
	assert.That(t, ok, "expected children slice")
	assert.Equal(t, 2, len(children))
	c0 := children[0].(map[string]any)
	assert.Equal(t, "*dumpfmt.leaf", c0["_type"])
	assert.Equal(t, "a", c0["Name"])
	assert.Equal(t, int64(1), c0["Count"].(int64))

	c1 := children[1].(map[string]any)
	_, hasCount := c1["Count"]
	assert.That(t, !hasCount, "zero Count should be omitted")
	_, hasHidden := c1["hidden"]
	assert.That(t, !hasHidden, "unexported field should be omitted")
}

func TestTypedTreeNilPointer(t *testing.T) {
	var p *leaf
	tree := TypedTree(p)
	assert.Nil(t, tree)
}

func TestNeedsYAMLQuoting(t *testing.T) {
	quote := []string{"true", "false", "null", "yes", "no", "~", "1.5", "-", "? x", "a:b", " trailing ", "with\nnl"}
	for _, s := range quote {
		assert.That(t, needsYAMLQuoting(s), "should quote %q", s)
	}
	plain := []string{"hello", "snake_case", "Camel", "a/b", "a-b", "value"}
	for _, s := range plain {
		assert.That(t, !needsYAMLQuoting(s), "should not quote %q", s)
	}
}

func TestMapKeysPutsTypeFirst(t *testing.T) {
	m := map[string]any{"zebra": 1, "_type": 2, "apple": 3}
	keys := mapKeys(m)
	assert.Equal(t, "_type", keys[0])
	assert.Equal(t, "apple", keys[1])
	assert.Equal(t, "zebra", keys[2])
}

// --- benchmarks --------------------------------------------------------

func benchTree() any {
	root := map[string]any{
		"_type": "*pkg.Root",
		"Name":  "root",
		"Children": []any{
			map[string]any{"_type": "*pkg.Leaf", "Name": "a", "Count": int64(1), "Tags": []any{"x", "y"}},
			map[string]any{"_type": "*pkg.Leaf", "Name": "b", "Count": int64(2)},
			map[string]any{"_type": "*pkg.Branch", "Children": []any{
				map[string]any{"_type": "*pkg.Leaf", "Name": "c"},
			}},
		},
		"Flags": map[string]any{"enabled": true, "debug": false},
	}
	return root
}

func BenchmarkEncodeYAML(b *testing.B) {
	tree := benchTree()
	var buf bytes.Buffer
	for b.Loop() {
		buf.Reset()
		if err := Encode(&buf, FormatYAML, tree); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeJSON(b *testing.B) {
	tree := benchTree()
	var buf bytes.Buffer
	for b.Loop() {
		buf.Reset()
		if err := Encode(&buf, FormatJSON, tree); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTypedTree(b *testing.B) {
	root := &branch{
		Label: "root",
		Children: []*leaf{
			{Name: "a", Count: 1},
			{Name: "b", Count: 2},
			{Name: "c", Count: 3},
			{Name: "d", Count: 4},
		},
		Misc: map[string]int{"x": 1, "y": 2, "z": 3},
	}
	for b.Loop() {
		_ = TypedTree(root)
	}
}

func BenchmarkEncodeStreamJSONL(b *testing.B) {
	items := make([]any, 100)
	for i := range items {
		items[i] = map[string]any{"pos": int64(i), "type": "IDENT", "literal": "x"}
	}
	var buf bytes.Buffer
	for b.Loop() {
		buf.Reset()
		if err := EncodeStream(&buf, FormatJSON, items); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeStreamYAMLSeq(b *testing.B) {
	items := make([]any, 100)
	for i := range items {
		items[i] = map[string]any{"pos": int64(i), "type": "IDENT", "literal": "x"}
	}
	var buf bytes.Buffer
	for b.Loop() {
		buf.Reset()
		if err := EncodeStream(&buf, FormatYAML, items); err != nil {
			b.Fatal(err)
		}
	}
}
