package parser

import (
	"testing"

	"github.com/lczyk/assert"
)

const bom = "\xef\xbb\xbf"

// TestUTF8BOMStripped asserts that a leading UTF-8 byte-order mark
// (EF BB BF) is transparently skipped before tokenization. Every MRI
// from 1.9 through 4.0 strips exactly one BOM at byte 0 and continues
// lexing the remainder normally.
//
// Goruby diverges from MRI in one respect: MRI's ident scanner accepts
// U+FEFF as a valid identifier letter anywhere in source, so `BOMputs`
// is tokenized as the ident `BOMputs`. Goruby's ident scanner still
// rejects mid-source U+FEFF as an illegal char. The cases below are all
// ones where the leading BOM is the ONLY BOM in source, so the parses
// here are unambiguous.
func TestUTF8BOMStripped(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		// trivial: empty after strip
		{"bare bom only", bom},
		{"bom + newline only", bom + "\n"},

		// simple expressions
		{"bom + puts", bom + `puts "hi"`},
		{"bom + integer literal", bom + "42"},
		{"bom + float literal", bom + "3.14"},
		{"bom + negative int", bom + "-7"},
		{"bom + nil", bom + "nil"},
		{"bom + boolean", bom + "true"},
		{"bom + symbol", bom + ":foo"},
		{"bom + global", bom + "$x = 1"},
		{"bom + ivar", bom + "@x = 1"},
		{"bom + cvar", bom + "@@x = 1"},

		// assignments / operators
		{"bom + assignment", bom + "x = 1"},
		{"bom + multi-assign", bom + "a, b = 1, 2"},
		{"bom + op-assign", bom + "x += 1"},

		// blocks / classes / methods
		{"bom + class def", bom + "class Foo\nend"},
		{"bom + class w/ super", bom + "class Foo < Bar\nend"},
		{"bom + module def", bom + "module M\nend"},
		{"bom + method def", bom + "def f(a, b)\na + b\nend"},
		{"bom + method default arg", bom + "def f(a = 1)\na\nend"},
		{"bom + method kw arg", bom + "def f(a:)\na\nend"},
		{"bom + def self.foo", bom + "def self.foo\nend"},

		// strings / interp / heredoc
		{"bom + double-quoted string", bom + `"hello"`},
		{"bom + single-quoted string", bom + `'hello'`},
		{"bom + string interp", bom + `"a #{1+2} b"`},
		{"bom + heredoc", bom + "x = <<~EOF\n  body\nEOF\n"},
		{"bom + %w array", bom + "%w[a b c]"},
		{"bom + regex", bom + "/abc/"},

		// control flow
		{"bom + if-else-end", bom + "if x\n1\nelse\n2\nend"},
		{"bom + while", bom + "while x\nputs 1\nend"},
		{"bom + case-when", bom + "case x\nwhen 1\n:a\nend"},

		// comments / preludes
		{"bom + comment then code", bom + "# leading comment\nputs 1"},
		{"bom + magic encoding comment", bom + "# encoding: utf-8\nputs 1"},
		{"bom + frozen string literal pragma", bom + "# frozen_string_literal: true\nputs 1"},
		{"bom + shebang", bom + "#!/usr/bin/env ruby\nputs 1"},

		// containers
		{"bom + array literal", bom + "[1, 2, 3]"},
		{"bom + nested array", bom + "[[1, 2], [3, 4]]"},
		{"bom + hash literal", bom + "{a: 1, b: 2}"},
		{"bom + hash rocket", bom + `{"a" => 1}`},
		{"bom + range", bom + "(1..10)"},

		// blocks / lambdas
		{"bom + lambda arrow", bom + "->(x) { x * 2 }"},
		{"bom + do-end block", bom + "[1].each do |x|\nputs x\nend"},
		{"bom + brace block", bom + "[1].each { |x| puts x }"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prog, err := parseSource(tc.src)
			checkParserErrors(t, err)
			assert.NotNil(t, prog, "no program returned")
		})
	}
}

// TestUTF8BOMSingleStripOnly asserts that only ONE leading BOM is
// stripped. MRI strips one BOM at byte 0 then treats subsequent U+FEFF
// bytes as identifier letters. Goruby strips one but its ident scanner
// rejects further U+FEFF -- documented divergence. The test pins
// goruby's behavior: a double BOM at byte 0 must remain an error.
func TestUTF8BOMSingleStripOnly(t *testing.T) {
	src := bom + bom + "puts 1\n"
	_, err := parseSource(src)
	assert.Error(t, err, "Illegal character")
}

// TestUTF8BOMOnlyAtStart asserts that a BOM anywhere except byte 0 is
// not stripped. Goruby's ident scanner still rejects mid-source U+FEFF
// as illegal -- divergent from MRI (which accepts it as an ident letter)
// but consistent with goruby's current lexer.
func TestUTF8BOMOnlyAtStart(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"bom after newline", "x = 1\n" + bom + "y = 2"},
		{"bom after space", " " + bom + "puts 1"},
		{"bom between tokens", "x + " + bom + " 1"},
		{"bom at end of line", "x = 1 " + bom + "\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseSource(tc.src)
			assert.Error(t, err, "Illegal character")
		})
	}
}

// TestUTF8BOMPartialPrefix asserts that a partial BOM prefix (EF or
// EF BB) is NOT stripped -- only the full 3-byte sequence triggers
// strip. The partial bytes remain as illegal multibyte input.
func TestUTF8BOMPartialPrefix(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"EF only", "\xefputs 1"},
		{"EF BB only", "\xef\xbbputs 1"},
		// trailing bytes that look BOM-ish but are not at offset 0
		{"BB BF only (no EF)", "\xbb\xbfputs 1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseSource(tc.src)
			assert.Error(t, err, "Illegal character")
		})
	}
}

// TestUTF8BOMPreservedInLiterals asserts that BOM bytes embedded inside
// string literals or comments are preserved as-is (since the lexer is
// not running its ident-char check there). This is independent of the
// BOM-strip prelude.
func TestUTF8BOMPreservedInLiterals(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"bom inside double-quoted string", `x = "a` + bom + `b"`},
		{"bom inside single-quoted string", `x = 'a` + bom + `b'`},
		{"bom inside line comment", "# comment " + bom + " text\nputs 1"},
		{"bom inside heredoc body", "x = <<~EOF\n  a" + bom + "b\nEOF\n"},
		// leading BOM stripped, then second BOM inside subsequent string
		{"leading bom + bom in string", bom + `x = "` + bom + `"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prog, err := parseSource(tc.src)
			checkParserErrors(t, err)
			assert.NotNil(t, prog, "no program returned")
		})
	}
}

// TestUTF8BOMPositionsPostStrip asserts that source positions reported
// by the parser/lexer count from the post-strip stream. If the BOM were
// still in the input, every token's column would be off by 3 (the BOM
// byte count). The test parses BOM + a known-shape program and checks
// that the AST String() reproduces the same source as a BOM-less parse.
func TestUTF8BOMPositionsPostStrip(t *testing.T) {
	plain := "x = 1\n"
	bommy := bom + plain
	progPlain, err := parseSource(plain)
	checkParserErrors(t, err)
	progBom, err := parseSource(bommy)
	checkParserErrors(t, err)
	assert.Equal(t, progPlain.String(), progBom.String())
}
