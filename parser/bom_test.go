package parser

import (
	"testing"

	"github.com/lczyk/assert"
)

// TestUTF8BOMStripped asserts that a leading UTF-8 byte-order mark
// (`EF BB BF`) is transparently skipped before tokenization. Every MRI
// from 1.9 through 4.0 strips this prefix; goruby currently rejects it
// with a "lex error: Illegal character" on the BOM bytes.
//
// The BOM may only appear at the very start of the file -- a BOM in the
// middle of source is an error, mirroring MRI. Only the leading-BOM case
// is tested here; mid-source BOM rejection is implicit in normal lexing.
func TestUTF8BOMStripped(t *testing.T) {
	const bom = "\xef\xbb\xbf"

	cases := []struct {
		name string
		src  string
	}{
		{"bare bom + puts", bom + `puts "hi"`},
		{"bom + integer literal", bom + "42"},
		{"bom + assignment", bom + "x = 1"},
		{"bom + class def", bom + "class Foo\nend"},
		{"bom + method def", bom + "def f(a, b)\na + b\nend"},
		{"bom + string interp", bom + `"a #{1+2} b"`},
		{"bom + comment then code", bom + "# leading comment\nputs 1"},
		{"bom + heredoc", bom + "x = <<~EOF\n  body\nEOF\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prog, err := parseSource(tc.src)
			assert.NoError(t, err, "parse rejected BOM-prefixed source")
			assert.NotNil(t, prog, "no program returned")
		})
	}
}

// TestUTF8BOMOnlyAtStart asserts that a BOM in the *middle* of source is
// still treated as illegal (MRI behaviour): only the leading 3 bytes are
// stripped.
func TestUTF8BOMOnlyAtStart(t *testing.T) {
	const bom = "\xef\xbb\xbf"
	src := "x = 1\n" + bom + "y = 2"
	_, err := parseSource(src)
	assert.Error(t, err, "Illegal character")
}
