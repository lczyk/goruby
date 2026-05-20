package lexer

import (
	"fmt"
	"testing"
	"time"

	"github.com/lczyk/goruby/token"
)

const (
	// fuzzPerInputTimeout bounds wall-clock per fuzz input. Triggers as
	// t.Fatalf so the input lands in the corpus as a regression. Set
	// well above realistic-fixture time so only true hangs fail.
	fuzzPerInputTimeout = 10 * time.Second
	// fuzzMaxInputSize caps the input bytes the fuzz target will lex.
	// Lexer is O(N) but huge mutated inputs starve the worker pool via
	// per-token pool growth. 1 KiB covers any realistic Ruby construct.
	fuzzMaxInputSize = 1024
	// fuzzMaxTokens caps tokens per input as a backstop against infinite
	// loops not caught by no-progress detection (e.g. progress by 1 byte
	// per token across a huge input). Far above any realistic 1 KiB
	// source token count.
	fuzzMaxTokens = 100000
)

// lexerSeeds returns the shared seed corpus. Tilted toward lexer-heavy
// constructs the parser fuzz under-covers: heredocs, %-literals, deep
// interpolation, BOM, regex flags, char/numeric edges.
func lexerSeeds() []string {
	return []string{
		// heredoc variants
		"x = <<EOF\nbody\nEOF\n",
		"x = <<-EOF\n  indented\n  EOF\n",
		"x = <<~EOF\n  squiggly\nEOF\n",
		"x = <<\"EOF\"\nhello #{name}\nEOF\n",
		"x = <<'EOF'\nliteral #{not_interp}\nEOF\n",
		"x = <<EOF, <<BAR\na\nEOF\nb\nBAR\n",
		// %-literals (matching + mismatched delims)
		"%w[foo bar baz]",
		"%w{a b c}",
		"%w(a b c)",
		"%w<a b c>",
		"%i[a b c]",
		"%q{single}",
		"%Q{double #{x}}",
		"%s{sym}",
		"%r{/path/}i",
		"%r{a}imx",
		// string interpolation, nested
		"\"a #{ \"b #{c}\" }\"",
		"\"#{\"#{\"#{x}\"}\"}\"",
		"\"plain\"",
		"\"esc \\n \\t \\x41 \\u{1F600}\"",
		// BOM prelude + mid-source U+FEFF
		"\xef\xbb\xbfputs \"hi\"",
		"\xef\xbb\xbf# coding: utf-8\nx = 1",
		"x = 1\n\xef\xbb\xbfy = 2",
		"x\xef\xbb\xbfy = 1",
		// regex flags + escapes
		"/foo/",
		"/foo/imx",
		"/[a-z]+\\s*/",
		"/\\A.*\\z/m",
		// char literals
		"?a",
		"?\\n",
		"?\\u{1F600}",
		"?\\x41",
		// numeric edges
		"0b1010",
		"0xFF_FE",
		"0o755",
		"0d123",
		"1_000_000",
		"1.5e2",
		"1.5e-3",
		"1r",
		"1i",
		"1.5ri",
		// encoding/magic comments
		"# encoding: utf-8\nx = 1",
		"# frozen_string_literal: true\nx = 1",
		"#!/usr/bin/env ruby\nx = 1",
		// line continuations + line endings
		"x = 1 + \\\n2",
		"x = 1\r\ny = 2",
		"x\x00y",
		// symbol variants
		":\"foo\"",
		":\"#{x}\"",
		":'lit'",
		":foo",
		// global / ivar / cvar
		"$global = 1",
		"@ivar = 1",
		"@@cvar = 1",
		"$1",
		"$~",
		"$_",
		// operator clusters
		"!x && y || z",
		"a <=> b",
		"a === b",
		"a..b",
		"a...b",
		"obj&.method",
		// general smoke
		"1 + 2",
		"def foo(x)\n  x\nend",
		"if x\n  y\nelse\n  z\nend",
	}
}

// runLex drains the lexer to EOF with panic recovery + token cap. Returns
// non-empty panic message on panic.
func runLex(input string) string {
	var panicMsg string
	defer func() {
		if r := recover(); r != nil {
			panicMsg = fmt.Sprintf("%v", r)
		}
	}()
	l := New(input)
	for range fuzzMaxTokens {
		tok := l.NextToken()
		if tok.Type == token.EOF {
			return panicMsg
		}
	}
	return panicMsg
}

func FuzzLex(f *testing.F) {
	for _, s := range lexerSeeds() {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, input string) {
		if len(input) == 0 || len(input) > fuzzMaxInputSize {
			return
		}

		done := make(chan struct{})
		var panicMsg string
		go func() {
			defer close(done)
			panicMsg = runLex(input)
		}()
		select {
		case <-done:
		case <-time.After(fuzzPerInputTimeout):
			t.Fatalf("lex exceeded %s on input %q", fuzzPerInputTimeout, input)
		}
		if panicMsg != "" {
			t.Errorf("lexer panicked on %q: %s", input, panicMsg)
		}
	})
}

// FuzzLexNoProgress catches infinite loops cheaply without waiting for the
// per-input timeout. Fails if NextToken returns two consecutive non-EOF
// tokens at the same Pos -- a true forward-progress violation. Tokens
// with Pos == prevPos but End advancing are fine (zero-width markers
// precede a real token), so we track Pos+End together.
func FuzzLexNoProgress(f *testing.F) {
	for _, s := range lexerSeeds() {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, input string) {
		if len(input) == 0 || len(input) > fuzzMaxInputSize {
			return
		}

		defer func() {
			if r := recover(); r != nil {
				t.Errorf("lexer panicked on %q: %v", input, r)
			}
		}()

		l := New(input)
		prevPos, prevEnd := -1, int32(-1)
		stuck := 0
		for range fuzzMaxTokens {
			tok := l.NextToken()
			if tok.Type == token.EOF {
				return
			}
			if tok.Pos == prevPos && tok.End == prevEnd {
				stuck++
				// Allow a small run of same-span tokens (synthetic
				// markers, zero-width emissions) but fail on a stall.
				if stuck > 8 {
					t.Fatalf("lexer no progress at pos %d on input %q (token %v)", tok.Pos, input, tok)
				}
			} else {
				stuck = 0
			}
			prevPos, prevEnd = tok.Pos, tok.End
		}
		t.Fatalf("lexer exceeded %d tokens on input %q (likely infinite loop)", fuzzMaxTokens, input)
	})
}
