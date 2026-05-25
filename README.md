goruby
======

ruby implementation in go. forked from
[goruby/goruby](https://github.com/goruby/goruby). front-end (lexer,
parser, ast) is feature-complete against the syntax checklist in
`features.md`; the tree-walking evaluator + object model are active
WIP. the `girb` repl is gone; a `goruby` cli runs program files, `-e`
oneliners, or stdin.

## usage

```go
import (
    gotoken "go/token"
    "github.com/lczyk/goruby/ast"
    "github.com/lczyk/goruby/parser"
)

fset := gotoken.NewFileSet()
program, err := parser.ParseFile(fset, "example.rb", src, parser.AllErrors)
if err != nil {
    // *parser.Errors collects multiple errors with positions
}

ast.Inspect(program, func(n ast.Node) bool {
    switch x := n.(type) {
    case *ast.ClassExpression:
        // ...
    case *ast.FunctionLiteral:
        // ...
    }
    return true
})
```

## packages

- `token` -- token types and the `Token` struct
- `lexer` -- ruby source to token stream
- `parser` -- token stream to `*ast.Program`. `ParseFile`, `ParseExpr`,
  `ParseExprFrom`. modes: `ParseComments`, `Trace`, `AllErrors`
- `ast` -- node types + `Walk`, `Inspect`
- `object` -- runtime object model (classes, methods, environment,
  `Send` dispatch)
- `evaluator` -- tree-walking evaluator + builtins; stdlib stubs in
  `evaluator/stdlib`
- `cmd/goruby` -- cli entry point; `cmd/{lex-dump,parse-dump,
  parse-roundtrip,normalize-parsetree,gen-arena}` -- dev tools

## tests

```
make test          # unit tests
make verify        # vet + gofmt + test + spellcheck
```

## integration tests

smoke tests against real ruby gems (lex, parse, walk -- no execution).
fixtures fetched into `internal/integrationtest/testdata/gems/`,
gitignored. requires git on the path.

```
make gems          # fetch pinned gems (minitest, rake) into testdata/
make integration   # run the smoke suite against fetched fixtures
make gems-clean    # remove fetched gems
```

starter set is small; add gems by appending to
`internal/integrationtest/testdata/gems.lock`. see `plan.md` for design
notes on the suite.

## supported syntax

see `features.md` for the running checklist of ruby syntax recognised by
the parser, and what's still outstanding.

## license

MIT, see `LICENSE`.
