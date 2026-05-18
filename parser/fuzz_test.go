package parser

import (
	"fmt"
	"testing"
	"time"

	"github.com/lczyk/goruby/ast"
)

// perInputTimeout bounds how long a single fuzz input is allowed to take.
// Guards against parser hangs surfacing as unbounded test runs.
const perInputTimeout = 30 * time.Second

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
	}
	// Regression seeds for crashes previously found by the fuzzer.
	seeds = append(seeds,
		"alia, alias",         // nil-leak into ExpressionList from alias without args
		"begin\nrescue A A=", // nil Right on Assignment from EOF after `=` inside rescue arg
	)
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, input string) {
		// Skip empty or whitespace-only inputs -- they're uninteresting.
		if len(input) == 0 {
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
			_ = prog.String()
			ast.Walk(ast.VisitorFunc(func(n ast.Node) ast.Visitor {
				if n != nil {
					_ = n.String()
				}
				return nil
			}), prog)
			ast.Inspect(prog, func(n ast.Node) bool {
				if n != nil {
					_ = n.String()
				}
				return true
			})
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
