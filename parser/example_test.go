package parser_test

import (
	"fmt"

	"github.com/lczyk/goruby/ast"
	"github.com/lczyk/goruby/parser"
)

func ExampleParseFile() {
	src := `LANG = "Ruby"

module Foo

	def bar()
		puts "Hello world"
	end

end`

	f, err := parser.ParseFile("", src, parser.AllErrors)
	if err != nil {
		fmt.Println(err)
		return
	}

	// Print the statements from the programs AST
	for _, s := range f.Statements {
		if exp, ok := s.(*ast.ExpressionStatement); ok {
			fmt.Printf("%T\n", exp.Expression)
		} else {
			fmt.Printf("%T\n", s)
		}
	}

	// output:
	//
	// *ast.Assignment
	// *ast.ModuleExpression
}

func ExampleParseExpr() {
	src := `def bar()
	puts "Hello world"
end`

	expr, err := parser.ParseExpr(src)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Printf("%T\n", expr)

	// output:
	//
	// *ast.FunctionLiteral
}
