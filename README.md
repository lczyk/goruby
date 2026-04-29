# goruby

![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/lczyk/goruby)
![GitHub Tag](https://img.shields.io/github/v/tag/lczyk/goruby?label=release)
[![lint_and_test](https://github.com/lczyk/goruby/actions/workflows/lint_and_test.yml/badge.svg)](https://github.com/lczyk/goruby/actions/workflows/lint_and_test.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/lczyk/goruby)](https://goreportcard.com/report/github.com/lczyk/goruby)

An implementation of *a subset* of Ruby in Go. The goal is to be able to run [Pyramid-Scheme](https://github.com/ConorOBrien-Foxx/Pyramid-Scheme).

This project began as a fork of [`goruby/goruby`](https://github.com/goruby/goruby) and has since diverged.

## Build

```bash
make build       # ./bin/goruby, ./bin/girb, ./bin/grgr
```

## Commands

### `goruby`

Run a Ruby file (or one-line scripts via `-e`):

```bash
goruby program.rb
goruby -e 'puts "hello"'
goruby -e 'x = 1' -e 'puts x'
```

Flags: `--cpuprofile`, `-e`, `--trace-parse`, `--trace-eval`, `-v`/`--version`.

### `girb`

Interactive REPL with multiline expressions and readline support. `CTRL-D` exits.

```bash
girb
```

Flags: `--noecho`, `--noprompt`, `-v`/`--version`.

### `grgr`

AST-printer / round-trip tool. Parses a Ruby file, runs the transformer pass, and prints the recovered source with debug comments.

```bash
grgr program.rb
```

Flags: `--trace-transform`, `-v`/`--version`.

## Layout

- [`cmd/goruby`](cmd/goruby) — `goruby` binary entry point
- [`cmd/girb`](cmd/girb) — `girb` REPL entry point
- [`cmd/grgr`](cmd/grgr) — `grgr` AST-printer entry point
- [`ast`](ast) — AST node definitions
- [`lexer`](lexer) — tokeniser
- [`parser`](parser) — parser, produces an `ast.Program`
- [`transformer`](transformer) — AST rewrite passes
- [`evaluator`](evaluator) — tree-walking evaluator
- [`interpreter`](interpreter) — top-level glue used by `cmd/goruby`
- [`object`](object) — runtime object model (Integer, String, Array, Hash, Proc, ...)
- [`repl`](repl) — REPL loop used by `girb`
- [`token`](token) — token type and source positions
- [`trace`](trace) — parse / eval trace helpers
- [`internal/version`](internal/version) — generated build-info (`Version`, `CommitSHA`, `BuildDate`, `BuildInfo`)

## Versioning

`VERSION` is the source of truth. `make build` and `make test` regenerate `internal/version/version.go` via [`lczyk/version`](https://github.com/lczyk/version) so each binary's `--version` reports the current version, commit SHA, build date, and dirty-tree marker.

## Licence

MIT — see [LICENSE](LICENSE). Upstream copyright from the [`goruby/goruby`](https://github.com/goruby/goruby) fork is preserved at [LICENSE-goruby](LICENSE-goruby).
