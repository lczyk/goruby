package parser_test

import (
	"fmt"

	"github.com/lczyk/goruby/parser"
)

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
