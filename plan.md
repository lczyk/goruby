# integration tests against real ruby gems

smoke-test the lexer, parser, and ast walker against real-world ruby source.
no execution, no semantic checks. catches crashes, panics, infinite loops,
and parser regressions.

## scope per layer

- lexer: tokenise to eof, no panic, no `ILLEGAL` tokens emitted
- parser: `parser.ParseFile` returns `(*ast.Program, nil)`. any non-nil error fails
- ast: `ast.Walk` w/ no-op visitor doesn't panic; `program.String()` doesn't panic

evaluator + interpreter: out of scope.

## layout

```
internal/integrationtest/
    gems_test.go          # //go:build integration
    gems.skip             # phase-tagged skip list
    testdata/
        gems.lock         # tab-separated: name<TAB>git-url<TAB>ref
        fetch_gems.sh     # populates gems/ from gems.lock
        gems/             # gitignored; one subdir per gem; .git removed
```

`gems.skip` lives next to `gems_test.go` (test config, not fixture data).

go test cwd = `internal/integrationtest/`, so `./testdata/gems/<name>/` is
the fixture root w/ no `findRepoRoot()` plumbing.

## gating + entry points

build tag `//go:build integration` on `gems_test.go`. default `make test`
never compiles it. opt-in via `make integration`.

three test funcs, all gated by the build tag:

- `TestGemsLex(t *testing.T)`
- `TestGemsParse(t *testing.T)`
- `TestGemsWalk(t *testing.T)`

each walks `./testdata/gems/` recursively for `.rb` files, runs a per-file
nested subtest via `t.Run(relpath, ...)` w/ `t.Parallel()`.

uses `github.com/lczyk/assert` for assertions.

## pass criteria

- **lex**: drive `lexer.New(src)` to `token.EOF`. fail if any `ILLEGAL`
  token emitted, or if a panic occurs
- **parse**: call `parser.ParseFile(fset, name, src, parser.AllErrors|parser.ParseComments)`.
  pass iff `err == nil && program != nil`
- **walk**: requires parse to have produced a `program`. call `ast.Walk` w/
  a no-op visitor, then `program.String()`. fail iff either panics

## per-file timeout

5s wall-clock per file, enforced in-test via goroutine + `select`:

```go
done := make(chan any, 1)
go func() {
    defer func() { done <- recover() }()
    // run phase under test
}()
select {
case r := <-done:
    if r != nil { t.Fatalf("panic: %v", r) }
case <-time.After(5 * time.Second):
    t.Fatal("timeout")
}
```

`recover` lives only inside the timeout goroutine, to forward the panic to
the test goroutine. no other recover -- go test handles panics natively.

on timeout the goroutine leaks; the test process exits at suite end. fine
for a smoke suite.

## skip-list (`gems.skip`)

format -- one entry per line:

```
<phase> <path-or-glob>[: reason]
```

- `<phase>` is `lex`, `parse`, or `walk`. entry only applies to that phase
- `<path-or-glob>` is relative to `testdata/gems/`. supports a single `*`
  glob segment. no `**`
- `# ...` for comments. blank lines ok

example:

```
parse minitest/lib/minitest/spec.rb: heredoc not supported
parse minitest/test/*: rspec-style metaprogramming
```

ships empty in phase 1.

### stale-entry detection (inline)

each subtest matches the file against `gems.skip`. if matched:
- run the underlying phase anyway
- if the phase _fails_ (as expected): `t.Skip(reason)`
- if the phase _passes_: `t.Fatalf("stale skip-list entry: <path>")`

glob entries are stale aggressively: if _any_ matched file would now pass,
the glob entry counts as stale. forces tight pruning.

`stale skip-list entry:` prefix lets ci grep for the failure mode.

## TestMain bootstrap

before subtests run:
1. parse `gems.skip` once, build matcher
2. read `gems.lock`, walk `testdata/gems/`:
    - each lock entry must have a corresponding `<name>/` w/ at least one
      `.rb` file. mismatch -> hard fail w/ "run `make gems`"
    - extra dirs not listed in lock -> log warning, don't fail
3. set up atomic counters per phase: pass / fail / skip / total

after subtests:
4. print summary, always (not gated on `-v`):

```
integration test summary:
  TestGemsLex:   M passed, N failed, K skipped (Q total)
  TestGemsParse: M passed, N failed, K skipped (Q total)
  TestGemsWalk:  M passed, N failed, K skipped (Q total)
```

per-subtest counter wiring uses `t.Cleanup`:

```go
t.Cleanup(func() {
    switch {
    case t.Skipped(): atomic.AddInt64(&parseSkip, 1)
    case t.Failed():  atomic.AddInt64(&parseFail, 1)
    default:          atomic.AddInt64(&parsePass, 1)
    }
})
```

## error reporting

```go
t.Errorf("parse failed: %s\n%v", relpath, err)
```

parser's `*Errors.Error()` already formats all collected errors w/ position.
no source dump.

## fixtures

`gems.lock` is tab-separated:

```
# name	git-url	ref
minitest	https://github.com/minitest/minitest.git	v5.25.4
rake	https://github.com/ruby/rake.git	v13.2.1
```

`fetch_gems.sh`:
- skips entries already present in `testdata/gems/<name>/`
- shallow clone w/ `--depth 1 --branch <ref>`. falls back to full
  clone + checkout for refs git can't resolve as branch/tag
- removes `.git/` post-clone (pure source snapshot, no pack-file walk)

starter set: minitest + rake only. expand once green.

## make targets

```make
gems:
	@bash internal/integrationtest/testdata/fetch_gems.sh

gems-clean:
	rm -rf internal/integrationtest/testdata/gems/

integration: gems
	go test -tags=integration -race -timeout 10m ./internal/integrationtest/...
```

`-race` flagged on -- lexer/parser shouldn't have data races but the
parallel subtest loop makes hangs more visible. no `-v` by default.

## .gitignore

```
internal/integrationtest/testdata/gems/
```

## ci

new `integration` job in `.github/workflows/lint_and_test.yml`:
- `actions/cache` keyed by `hashFiles('internal/integrationtest/testdata/gems.lock')`
- runs `make integration`

low priority; will sit untested for a while.

## phasing

1. **phase 1 (now)**: layout, fetch script, make targets, build-tag, three
   test funcs, empty `gems.skip`, lex pass criterion, parse pass criterion,
   walk pass criterion, summary, TestMain bootstrap. ci wiring. minitest +
   rake only
2. **phase 2**: triage failures into `gems.skip` until ci is green
3. **phase 3**: expand to 5-6 gems
4. **phase 4**: `String()` round-trip stability test (parse -> format ->
   parse, compare ast)

## non-goals

- evaluator / interpreter coverage
- semantic analysis
- ruby stdlib sources (separate problem -- not git, version selection,
  license)
- per-gem stats output (phase 4 if at all)
- automatic version bumps for gems
