package parser

import (
	"fmt"
	"testing"
	"time"

	"github.com/lczyk/goruby/ast"
)

// perInputTimeout bounds how long a single fuzz input is allowed to take.
// Guards against parser hangs surfacing as unbounded test runs.
const (
	// perInputTimeout bounds wall-clock per fuzz input. Triggers as
	// t.Fatalf, so the input lands in the corpus as a regression. The
	// per-input work (ParseFile + String + Walk + Inspect) is O(N^2)
	// in AST depth, so deeply-nested mutated inputs can chew real
	// CPU under the cap; the maxInputSize guard caps the worst case.
	// Set well above realistic-fixture time so only true hangs fail.
	perInputTimeout = 10 * time.Second
	// maxInputSize caps the input bytes the fuzz target will parse.
	// Without this, the engine generates inputs in the hundreds of KB
	// which produce deep ASTs and starve the worker pool through the
	// per-node String/Walk/Inspect passes. 4 KiB covers any realistic
	// Ruby construct.
	maxInputSize = 1024
)

func FuzzParse(f *testing.F) {
	seeds := []string{
		"1 + 2",
		"def foo(x)\n  x\nend",
		"if x\n  y\nelse\n  z\nend",
		"case x\nwhen 1\n  y\nend",
		"[1, 2, 3]",
		"\"hello #{name}\"",
		"/regex/",
		"->(x) { x }",
		"super(1, 2)",
		"alias foo bar",
		"undef foo",
		"module M\nend",
		"class C < P\nend",
		"begin\nrescue E => e\nend",
		"1 => x",
		"def foo(**kwargs)\nend",
		"def foo(...)\nend",
		"{a: 1, b: 2}",
		"..5",
		"...10",
		"obj&.method",
		"@ivar = 42",
		"$global += 1",
		"Foo::Bar::baz",
		"!x && y || z",
		"x += y -= z",
		"%w[foo bar baz]",
		"def foo\nrescue\nend",
		// flip-flop (cond-position `..`/`...`)
		"if (i == 1)..(i == 5)\n  puts i\nend",
		"if (i == 1)...(i == 5)\n  puts i\nend",
		"puts i if (i == 1)..(i == 5)",
		"unless (k == 2)..(k == 6)\n  puts k\nend",
		"while (p == 0)..(p == 3)\n  p += 1\nend",
		"((i == 1)..(i == 5)) ? :in : :out",
		"if !((w == 3)..(w == 6))\n  puts w\nend",
		// BOM-prefixed source (lexer prelude)
		"\xef\xbb\xbfputs \"hi\"",
		"\xef\xbb\xbf# coding: utf-8\nx = 1",
		// mid-source U+FEFF as ident-letter
		"x = 1\n\xef\xbb\xbfy = 2",
		"x\xef\xbb\xbfy = 1",
		// heredoc variants
		"x = <<EOF\nbody\nEOF\n",
		"x = <<-EOF\n  indented\n  EOF\n",
		"x = <<~EOF\n  squiggly\nEOF\n",
		"x = <<\"EOF\"\nhello #{name}\nEOF\n",
		"x = <<'EOF'\nliteral #{not_interp}\nEOF\n",
		// percent literals (beyond %w)
		"%q{single}",
		"%Q{double #{x}}",
		"%i[a b c]",
		"%s{sym}",
		"%r{/path/}i",
		// endless method (3.0+)
		"def foo = 42",
		"def bar(x) = x * 2",
		// pattern matching / deconstruction
		"case x\nin [a, b]\n  a + b\nend",
		"case x\nin {a:, b:}\n  a + b\nend",
		"case x\nin Integer => n\n  n\nend",
		// numeric base + underscores + char literal
		"0b1010",
		"0xFF_FE",
		"0o755",
		"1_000_000",
		"1.5e2",
		"?a",
		"?\\n",
		// symbol variants
		":\"foo\"",
		":\"#{x}\"",
		":'lit'",
		// splat / multi-assign
		"a, *b, c = 1, 2, 3, 4",
		"[*a, *b]",
		// modifier rescue + chained modifiers
		"x = y rescue nil",
		"puts x if cond rescue nil",
		// singleton class
		"class << obj\n  def foo\n  end\nend",
		// retry / redo / next / break with arg
		"begin\nretry\nrescue\nend",
		"loop { break 42 }",
		"loop { next :x }",
		// for loop
		"for i in 1..10\n  puts i\nend",
		// global match vars + regex captures
		"x = $1\ny = $~\n",
		// __FILE__ / __LINE__ / __dir__
		"__FILE__\n__LINE__\n__dir__\n",
	}
	// Regression seeds for crashes previously found by the fuzzer.
	seeds = append(seeds,
		"alia, alias",        // nil-leak into ExpressionList from alias without args
		"begin\nrescue A A=", // nil Right on Assignment from EOF after `=` inside rescue arg
		"begin\nrescue[",     // nil element in ArrayLiteral when `[` followed by EOF as rescue exception class
	)
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, input string) {
		// Skip empty or whitespace-only inputs -- they're uninteresting.
		if len(input) == 0 {
			return
		}
		// Cap mutated-input size. Without this, the engine generates
		// huge inputs that produce deep ASTs and chew CPU through the
		// per-node String/Walk/Inspect passes, throttling exec rate.
		if len(input) > maxInputSize {
			return
		}

		done := make(chan struct{})
		var panicMsg string
		go func() {
			defer close(done)
			defer func() {
				if r := recover(); r != nil {
					panicMsg = fmt.Sprintf("%v", r)
				}
			}()
			prog, err := ParseFile("fuzz.rb", []byte(input), ParseComments)
			if err != nil || prog == nil {
				return
			}
			// One root-level String exercises every node's String via the
			// recursive descent. Calling String again per-node inside Walk
			// / Inspect makes the whole pass O(depth^2) for deeply nested
			// inputs (`&&&...`), starving the worker pool. Walk and Inspect
			// still run to catch nil-deref panics in visitor traversal.
			_ = prog.String()
			ast.Walk(ast.VisitorFunc(func(n ast.Node) ast.Visitor { return nil }), prog)
			ast.Inspect(prog, func(n ast.Node) bool { return true })
		}()
		select {
		case <-done:
		case <-time.After(perInputTimeout):
			t.Fatalf("parse exceeded %s on input %q", perInputTimeout, input)
		}
		if panicMsg != "" {
			t.Errorf("parser panicked on %q: %s", input, panicMsg)
		}
	})
}
