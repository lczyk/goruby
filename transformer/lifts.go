package transformer

import (
	"go/token"

	"github.com/lczyk/goruby/ast"
	"github.com/lczyk/goruby/parser"
	"github.com/lczyk/goruby/trace/printer"
)

type context_spec string

type replacement_spec struct {
	statement ast.Statement
	name      string
}

func MustCompileStatement(code string) ast.Statement {
	program, tracer, err := parser.ParseFileEx(token.NewFileSet(), "", code, false)
	if err != nil {
		panic(err)
	}
	if tracer != nil {
		walkable, err := tracer.ToWalkable()
		if err != nil {
			panic(err)
		}
		walkable.Walk(printer.NewTracePrinter())
	}

	if len(program.Statements) != 1 {
		panic("MustCompileStatement expects a single statement")
	}
	return program.Statements[0]
}

var context_call_expr_replacements map[context_spec]replacement_spec = map[context_spec]replacement_spec{
	"find_all": {
		name: "_grgr_find_all",
		statement: MustCompileStatement(`
_grgr_find_all = -> (range, fun) {
    out = []
    i = 0
    loop {
        break if i >= range.size
        if fun[i]
            out.push(i)
        end
        i += 1
    }
    out
}`),
	},
}
