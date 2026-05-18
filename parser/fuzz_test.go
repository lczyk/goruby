package parser

import (
	gotoken "go/token"
	"testing"

	"github.com/lczyk/goruby/ast"
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
	}
	// Regression seeds for crashes previously found by the fuzzer.
	seeds = append(seeds,
		"alia, alias", // nil-leak into ExpressionList from alias without args
	)
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, input string) {
		// Skip empty or whitespace-only inputs -- they're uninteresting.
		if len(input) == 0 {
			return
		}

		prog, err := ParseFile(gotoken.NewFileSet(), "fuzz.rb", []byte(input), ParseComments)
		if err != nil {
			// Parse errors are expected; the parser should not panic.
			return
		}
		if prog == nil {
			return
		}

		// AST.String() must not panic.
		_ = prog.String()

		// Walk must not panic.
		ast.Walk(ast.VisitorFunc(func(n ast.Node) ast.Visitor {
			if n != nil {
				_ = n.String()
			}
			return nil
		}), prog)

		// Inspect must not panic.
		ast.Inspect(prog, func(n ast.Node) bool {
			if n != nil {
				_ = n.String()
			}
			return true
		})
	})
}
