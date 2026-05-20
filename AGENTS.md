# AGENTS.md

agent-facing notes for this repo. covers stuff that isn't obvious from grep
alone -- pkg layout, dev workflow, codegen, gotchas. iff this file disagrees
with the code, the code wins; please fix the discrepancy or flag it.

## what this is

ruby front-end as a go library, with an in-progress evaluator. lexer +
parser + ast are the stable surface (forked down from goruby/goruby);
`object` + `evaluator` are a fresh, ground-up rewrite of the runtime
side aimed at MRI-compatible behaviour. consumers either walk the AST
directly, re-emit via `(*ast.Program).String()`, or evaluate via
`evaluator.Eval(node, env)`.

## layout

- `token` -- token types + `Token` struct (24B noscan; see below)
- `lexer` -- ruby source -> token stream. `New(string)` / `NewBytes([]byte)`,
  `NextToken()` / `HasNext()`. heredoc lexer uses a cursor stack on
  immutable `l.input` (no per-heredoc splice allocs)
- `parser` -- token stream -> `*ast.Program`. `ParseFile`, `ParseExpr`,
  `ParseExprFrom`, `WithArena(*ast.Arena)` for cross-parse reuse,
  `WithContext` for caller-provided tracer, `WithVersion(token.RubyVersion)`
  for syntax-gating
- `ast` -- node types + `Walk`, `Inspect`, `Equal`, `Arena` (bump alloc)
- `object` -- ruby runtime value types (Integer, String, Symbol, Array,
  Hash, ...). leaf value types are pointer-free so their heap allocations
  are noscan-eligible, mirroring the ast leaf discipline. Symbol /
  FrozenString use int32 IDs into per-env pools; Integer has a small-int
  cache for -128..1152
- `evaluator` -- `Eval(node, env)` tree walker. covers literals,
  arithmetic + comparison + short-circuit, locals / globals / constants
  / instance vars / multi-assignment, control flow (if/unless/while/until/
  case-when/ternary/modifier forms), string interpolation + common
  String/Array/Hash/Integer methods, method definition (positional /
  defaults / splat / keyword args), `self` / `super`, classes + modules
  (attr_accessor, include, inheritance, class methods, constants,
  `is_a?`), blocks (each / map / select / reduce / yield + `&blk`
  capture), Proc + lambda + `.call` / `.()`, exceptions (raise /
  rescue / else / ensure + built-in hierarchy + ZeroDivisionError from
  `/0`), safe-nav `&.`. version threading mirrors the parser's pattern
- `internal/parsetreenorm` -- normaliser for MRI `--dump=parsetree` output;
  see [normalizer section](#parsetree-normalizer-for-tests) below
- `internal/dumpfmt` -- shared YAML / JSON emitter used by the debug-dump
  cmds. no third-party YAML dep (minimal hand-rolled block-style emitter)
- `cmd/parse-dump` -- ast tree dump (yaml/json) for a ruby file
- `cmd/lex-dump` -- token stream dump (yaml/jsonl) for a ruby file
- `cmd/parse-roundtrip` -- parse then re-emit via `ast.Format`; `--check`
  for diff-mode
- `cmd/eval` -- run a ruby file through the goruby evaluator; stdout is
  the program's output. accepts `--version=X.Y`
- `cmd/normalize-parsetree` -- cli wrapping `parsetreenorm` for ad-hoc diffs
- `cmd/gen-arena` -- codegen for `ast/arena_gen.go` (see below)
- `internal/integrationtest` -- gem / mri-golden / mri-parsetree-diff /
  evaluator-corpus suites
- `.rubies` -- pinned MRI binaries (built locally via `make rubies`)

## make targets

| target | what |
|---|---|
| `make test` | unit + integration (no MRI required) |
| `make unit` | go test ./... with race detector |
| `make lint` | go vet + gofmt -l check |
| `make fmt` | gofmt -s -w |
| `make spellcheck` | cspell via npx |
| `make verify` | lint + test + spellcheck + gen-check (pre-commit gate) |
| `make bench` | benchmarks. override `BENCH=...` `BENCHTIME=...` `PKG=...` |
| `make fuzz` | one fuzz target. override `FUZZ=` `FUZZTIME=` |
| `make cover` | coverage profile + html |
| `make gen` | regenerate code (currently just `ast/arena_gen.go`) |
| `make gen-check` | regen and fail iff generated files diverge from committed |
| `make gems` | fetch pinned ruby gem fixtures into `internal/integrationtest/testdata/gems/` |
| `make rubies` | fetch + build MRI binaries into `.rubies/versions/<X>/` |
| `make integration` | smoke suite against fetched gems |
| `make oracle` | full MRI parsetree diff across all built versions (~2 min) |
| `make little-oracle` | oracle restricted to MRI 2.6 (~10s, iterative form) |
| `make fuzz-list` | list every available fuzz target |
| `make rubies-verify` | run every test fixture against the built MRI binaries |
| `make rubies-golden` | regenerate `mri-golden.tsv` for the parser version-gate suite |
| `make eval-corpus-expected` | regenerate `.expected` files for the evaluator integration corpus from `#=> value` markers |
| `make eval-corpus-oracle` | run the evaluator corpus under pinned MRI and diff stdout against `.expected` (ORACLE_VER=2.6.0; V=1 for verbose; see `scripts/eval-corpus-oracle`) |

oracle + integration depend on `make rubies` + `make gems` having been
run first.

## token.Token shape (noscan, 24B)

`Token` is pointer-free so arena slabs holding embedded `Token` are
GC-noscan. layout: `Pos int (8B) + End int32 (4B) + LitOff int32 (4B) +
Type Type (4B) + 4 bools (4B) = 24B`.

- literal text lives in a per-parse `[]string` pool (`Arena.LitPool`),
  indexed by `Token.LitOff`. lexer appends as it emits.
- to resolve a token's text post-parse: `tok.LitOfPool(prog.LitPool)`
  (or `tok.LitOf(source)` if you still hold the input)
- fixed-literal types (keywords, operators, punctuation) can derive their
  text from `tok.Type.Literal()` -- compile-time-constant string table
- `tok.Literal` (the old string field) is gone. anywhere that wants the
  source text needs the pool or the type-derived literal

variable-content types (IDENT, INT, FLOAT, STRING, ...) require pool
resolution. fixed-content types must NOT use the pool path (the lit slot
is set to 0 / unused for them in the lexer's hot path).

## arena + slice-cap preservation

`ast.Arena` is a chunked bump allocator with a slab per AST node type.
each parse can either get a fresh arena (default) or reuse one across
calls via `parser.WithArena(arena)`. reuse: `arena.Reset()` rewinds every
slab's cursor without touching the chunks themselves, so subsequent
parses re-fill the same backing memory.

each `Arena.NewT()` constructor explicitly clears the slot's fields. slice
fields use `[:0]` truncation so backing arrays survive Reset -- the slice
cap built up over the first parse gets reused on every subsequent one,
killing slice-grow allocs in the steady state (`BlockStatement.Statements`,
`ArrayLiteral.Elements`, etc.).

contract: a caller that retains a `*ast.Identifier` from parse N and then
issues parse N+1 will see that pointer's data silently overwritten. don't
hold AST pointers across `arena.Reset` / re-parse.

### adding a new AST node type

1. add the struct in `ast/ast.go` with whatever fields it needs.
2. add a `slab[T]` field to `Arena` in `ast/arena.go` (one line):
   ```go
   fooSlab slab[Foo]
   ```
3. add the matching `a.fooSlab.reset()` call to `Arena.Reset()` (manual --
   one line; could be codegened but isn't).
4. `make gen` -- regenerates `ast/arena_gen.go` with the `NewFoo()`
   constructor.
5. commit `ast.go` + `arena.go` + `arena_gen.go` together.

CI guard: `make gen-check` (in `make verify`) re-runs the generator and
fails the commit if the result differs from what's checked in. catches
missed regen.

## cmd/gen-arena

generator that emits `ast/arena_gen.go`. derives the target type list from
`Arena.slab[T]` fields in `arena.go`; reads `ast.go` for each type's field
list and emits a per-field clear in the `NewT` body. supports `:0` slice
truncation, scalar zeroing, `pkg.Type{}` for cross-package structs,
`.Reset()` for in-package named types that expose one.

invoked via `//go:generate go run ../cmd/gen-arena -in . -out arena_gen.go`
in `ast/arena.go`. don't run it by hand -- `make gen` from repo root.

## parser arena reuse pattern (for batch / lsp / repl)

```go
arena := ast.NewArena()
for _, path := range files {
    src, _ := os.ReadFile(path)
    prog, err := parser.ParseFile(path, src, 0, parser.WithArena(arena))
    if err != nil { ... }
    // consume prog HERE -- next ParseFile clobbers its memory
    process(prog)
}
```

first parse warms up slab chunk counts; subsequent parses pay ~zero
mallocgc for AST nodes and slice-grow backings until peak footprint is
exceeded.

## parsetree normalizer (for tests)

`internal/parsetreenorm` exists to make `ruby --dump=parsetree` output
diffable across two source variants of the same program. MRI's
`--dump=parsetree` interleaves real tree structure with cosmetic noise
(node IDs, line ranges, sibling indices, last-sibling markers, ...) and
also varies its format between ruby versions (1.9 through 4.0). the
normaliser strips all of that and leaves a structural form suitable for
byte equality.

### usage

three entry points:

```go
parsetreenorm.Normalize(dump string) string
parsetreenorm.NormalizeWithSource(dump, src string) string
parsetreenorm.NormalizeRaw(dump string) string          // skip flat-form
parsetreenorm.NormalizeWithSourceRaw(dump, src string) string
```

- `Normalize` is the source-agnostic form -- strips IDs, locations,
  trailing-star markers, paren markers, `__FILE__` tempfile paths, header
  banners, no-op `NODE_BEGIN(null)` wrappers, single-child `NODE_BLOCK`s.
- `NormalizeWithSource` does all of the above PLUS masks `__LINE__`
  references: it scans `src` to find lines that contain `__LINE__` and
  rewrites the corresponding `nd_lit` integer in the dump to a constant
  sentinel. without this, two re-emitted variants of the same source with
  shifted line numbers compare unequal.
- the `Raw` variants skip the final "flat-form reparse" step. flat form
  produces one line per leaf with a `path.from.root = value` shape;
  callers that want to inspect the indented tree directly use `Raw`.

### where it's used

the only in-repo consumer is `internal/integrationtest/parsetree_test.go`
(`TestMRIParseTreeDiff`):

1. parse `src1` with goruby into AST.
2. emit `src2 := prog.String()`.
3. run `<rubyBin> --dump=parsetree` on both src1 and src2 (src1 cached).
4. `parsetreenorm.NormalizeWithSource(dump1, src1)` and same for src2.
5. byte-compare; on mismatch, report the first diverging line.

the `cmd/normalize-parsetree` cli is the same logic exposed for ad-hoc
manual diffs:

```
diff \
  <(./normalize-parsetree .rubies/versions/2.6.0/bin/ruby a.rb) \
  <(./normalize-parsetree .rubies/versions/2.6.0/bin/ruby b.rb)
```

useful when the oracle test reports a tree mismatch on a fixture and you
want to see the actual diff with full context.

### normalisation rule (do not violate)

every transform must preserve runtime semantics. display-only fields and
genuinely-no-op wrappers get stripped; anything that could change
behaviour stays. when extending: write the test case in
`internal/parsetreenorm/parsetreenorm_test.go` (or its fuzz target) and
make sure round-tripping a normalised tree through MRI still produces
the same parsetree.

### cache layer

`TestMRIParseTreeDiff` caches the (rubyBin, src) -> normalised-tree
mapping on disk under `parsetreeCacheDir` (content-addressed by src
sha256, partitioned by ruby version directory name). cache is invalidated
whenever `parsetreenorm` shape changes -- bump `parsetreeCacheDir`'s name
in `parsetree_test.go` when modifying the normaliser. otherwise stale
cached trees mask normalisation regressions.

### skip list

`internal/integrationtest/parsetree.skip` carries known-mismatch fixtures
keyed by `(file, mri-version, bucket)` where bucket is `tree-mismatch`,
`src2-rejected`, or `goruby-rejected`. stale skip entries (would have
passed) fail the test loudly so the list stays honest. fold a new
exception in by appending a row + grouping it with a `#`-prefixed comment
explaining why.

## debug-dump clis

three small helpers in `cmd/` for inspecting what the lexer / parser
produce on a given source. all read either a file path or stdin; all
honour `--version=X.Y` for parser/lexer gating.

- `cmd/parse-dump` -- ast tree on stdout. `--format=yaml` (default) or
  `--format=json`. each struct node carries a `_type` field naming its
  concrete type so polymorphic children are unambiguous. uses
  `internal/dumpfmt`.
  ```
  echo 'puts 1 + 2' | go run ./cmd/parse-dump
  go run ./cmd/parse-dump --version=2.7 --format=json fixture.rb
  ```
- `cmd/lex-dump` -- one entry per token. `--format=yaml` (default) emits
  a single block sequence; `--format=json` emits JSONL.
  ```
  echo 'a + b' | go run ./cmd/lex-dump --format=json
  ```
- `cmd/parse-roundtrip` -- parses source then re-emits it via
  `ast.Format`. by default prints the re-emitted source to stdout (no
  `--format` flag -- output is ruby, not yaml/json). `--check` exits
  non-zero on input/output mismatch with a minimal unified diff to
  stderr; useful for confirming the parser+printer round-trip on a
  fixture without standing up the oracle.
  ```
  diff -u fixture.rb <(go run ./cmd/parse-roundtrip fixture.rb)
  go run ./cmd/parse-roundtrip --check fixture.rb
  ```

shared format machinery lives in `internal/dumpfmt`: minimal hand-rolled
yaml emitter + stdlib json. `dumpfmt.Encode` writes a single document,
`dumpfmt.EncodeStream` writes a sequence (yaml: block sequence, json:
jsonl). `dumpfmt.TypedTree` wraps a struct tree in `map[string]any` with
`_type` tags via reflection, used by `parse-dump` for the ast.

## test helpers

unit + integration tests use `github.com/lczyk/assert` and its
`github.com/lczyk/assert/require` subpackage. distinction:

- `assert.*(t, ...)` -- records failure, continues. use when subsequent
  assertions in the same test still produce useful signal even when
  this one failed.
- `require.*(t, ...)` -- records failure and `t.FailNow()`-s. use when
  subsequent code in the test would panic / produce garbage if this
  assertion fails (e.g. type-asserts where downstream code dereferences
  the result).

most parser-test conversions land `require.That(t, ok)` after type
assertions to guard the subsequent field access. don't reach for
`require` for ordinary equality checks where the test can keep running.

## tests

- **unit**: `go test ./...` with race. fast.
- **integration**: `make integration`. needs `make gems` first. exercises
  lex / parse / walk against fetched ruby gems, plus the evaluator
  corpus harness (see below).
- **evaluator corpus**: `TestEvaluatorCorpus` in
  `internal/integrationtest/evaluator_test.go` walks supported subdirs
  under `testdata/evaluator/` (currently `literals`, `arithmetic`,
  `variables`, `control_flow`, `strings`, `hashes`, `arrays`,
  `methods`, `blocks`, `classes`, `modules`, `self_kw`, `exceptions`,
  `version-gates`). each fixture runs at `max(2.6, # minversion: X.Y)`
  so version-gated features get their declared ruby version. compares
  evaluator stdout against the sibling `.expected`. honours
  `# skip-evaluator: <reason>` (temporary opt-out without breaking the
  bash mri oracle). add subdirs to `supportedEvaluatorSubdirs` once
  every fixture in the dir is runnable.
- **evaluator mri oracle**: `make eval-corpus-oracle` runs the same
  fixtures under pinned MRI and diffs stdout against `.expected`,
  catching corpus drift independently of the goruby evaluator.
- **mri golden lex / parse**: per-MRI-version syntax-check golden tables
  in `internal/integrationtest/testdata/mri-golden.tsv`. needs
  `make rubies`.
- **mri parsetree diff oracle**: `make oracle`. ~5840 cases across all
  built MRI versions. uses the normaliser above. ~2min full sweep.
- **roundtrip**: `TestRoundtrip` parses src1 -> emits src2 -> parses src2
  -> diffs ASTs (after parsetree normalisation via MRI cross-check).
- **fuzz**: `make fuzz` (default targets `FuzzParse` in `./parser/`).
  Override pkg + target via `PKG=./internal/parsetreenorm
  FUZZ=FuzzNormalize` etc. `make fuzz-list` enumerates available
  targets. Live fuzz targets: `FuzzParse` (parser), `FuzzNormalize`
  + `FuzzNormalizeWellFormed` (parsetreenorm), plus the lexer's
  no-progress detector (`FuzzLex` family).

## debugging an oracle mismatch

when `TestMRIParseTreeDiff` reports `<file>/ruby_<ver>` mismatch, the
fastest path to the actual divergence is:

1. run goruby on the fixture, capture `prog.String()` output:
   ```
   go run ./cmd/normalize-parsetree \
     .rubies/versions/<ver>/bin/ruby <fixture>.rb > /tmp/src1.tree
   ```
2. emit `src2` with `cmd/parse-roundtrip` (parses + re-emits the source
   via `ast.Format`); save to `/tmp/src2.rb`:
   ```
   go run ./cmd/parse-roundtrip <fixture>.rb > /tmp/src2.rb
   ```
3. normalise src2 the same way:
   ```
   go run ./cmd/normalize-parsetree \
     .rubies/versions/<ver>/bin/ruby /tmp/src2.rb > /tmp/src2.tree
   ```
4. `diff /tmp/src1.tree /tmp/src2.tree | head -40` -- first non-trivial
   diff line names the divergent node path.

normalised dumps use a `path.from.root = value` flat form by default;
pass `--raw` to the cli for the indented tree shape when you want to
see structure visually.

if the difference is line-number-only (`__LINE__` text drift), confirm
`NormalizeWithSource` is masking it via `parsetreenorm` -- it should.
otherwise it's a genuine AST shape divergence (missing wrap node, wrong
splat encoding, etc.) and lives in parser / printer code.

if the fixture should be tolerated rather than fixed, add a row to
`internal/integrationtest/parsetree.skip` with the bucket
(`tree-mismatch` / `src2-rejected` / `goruby-rejected`) and a comment
explaining the root cause.

cache hazard: the oracle disk cache lives at
`internal/integrationtest/.cache/mri-parsetree-vN/<fullVer>/<sha256>.tree`.
content-addressed by source, so source edits invalidate naturally. but
changes to `parsetreenorm` shape do NOT invalidate the cache -- the
cache key encodes the version dir name and the source hash, not the
normaliser version. when modifying `parsetreenorm`, bump the cache
directory name (`parsetreeCacheDir` const in `parsetree_test.go`,
currently `.cache/mri-parsetree-v3`) so stale entries get rebuilt.

## hand-rolled vs codegen

- `ast/ast.go` -- hand
- `ast/arena.go` -- hand (slab, chunk, Arena struct, Reset, Init,
  `//go:generate` directive)
- `ast/arena_gen.go` -- **generated** by `cmd/gen-arena`. has "DO NOT
  EDIT" header. don't hand-edit; next regen clobbers
- `token/token.go` -- hand. has a `//go:generate stringer -type=Type`
  directive that we don't currently run -- `Type.String()` is hand-written
  in token.go. ignore or rip the directive

## performance: what's been tried

short ledger so you don't redo work. detail in commit messages.

landed:

- lexer token queue: channel -> slice deque (~2.8x lex, 1.8x parse)
- lexer ASCII fast-path in `next()` / `peek()`
- ordered map dedup-scan removal (parse hash literal ~2x)
- call-arg terminator bitset (parser dispatch)
- AST node byte-pack: `EndPos int` instead of full `EndToken token.Token`
  on closer-bearing nodes; `HashLiteral.Map` embedded by value;
  `StringLiteral.HeredocTag` derived from `Token.Type`
- `token.Token` 32B -> 24B noscan (drop `Literal string`; lit-pool
  indirection via `LitOff int32`)
- arena per-type bump alloc (`ast.Arena`, ~60 slabs)
- arena chunk reuse across parses via `Arena.Reset` + `parser.WithArena`
- slice-cap preservation on arena slot reuse (per-field NewX clears
  truncate slices via `:0` instead of nil-ing)
- heredoc cursor stack: 7 input-rewrite splice sites collapsed to a
  segment queue on immutable `l.input`
- trace-defer cost: cached `p.tracing` bool, `p.trace()` short-circuit
- `parser.WithArena` for batch / lsp / repl pools
- magic-encoding-comment preservation on re-emit (oracle fix)

tried and reverted / shelved:

- stale-detection in `arenaAlloc` (tuple return added ABI overhead
  that exceeded slot-clear savings)
- differentiated `slab.reset(clear)` with per-type clear callbacks
  (indirect-call cost per slot per Reset killed perf)
- bulk Token.Literal removal as a "rename .Literal field to .Literal()
  method" sweep (broke 100+ oracle cases due to symbol-key call sites
  where the source-derived text was used in compound nodes; eventually
  landed via the LitOff pool approach instead)

remaining levers (not pursued):

- variadic `acceptOneOf` / `currentTokenOneOf` -- 142 callsites still
  variadic. marginal (escape analysis usually pins to stack).

## related docs

- `README.md` -- public-facing project intro
- `features.md` -- supported ruby syntax checklist (parser-level)
- `internal/integrationtest/parsetree.skip` -- known oracle mismatches
  (file format documented inline; load via `loadGoldenSkips` in
  `internal/integrationtest/parsetree_test.go`)

## conventions

- one category per commit (`feat:` / `fix:` / `test:` / `perf:` / `chore:`
  / `refactor:` / `bench:` / `docs:` / `revert:` / `ci:`). enforced by
  pre-commit hook (conventional commits).
- british english in prose. `-ise` not `-ize`, `-our` not `-or`.
- ASCII only in source comments and docs. pre-commit hook rejects
  non-ASCII bytes in staged changes. use `->` / `--` / `<=` instead of
  the unicode glyph equivalents; same for curly quotes, bullet glyphs,
  the micro-sign, etc.
- no `Co-Authored-By` lines from agents in commits.
- marker comments must carry a parenthesised owner tag (e.g.
  `<MARKER>(name):` shape) so orphan flags do not accumulate. the
  pre-commit hook rejects bare forms of `t-o-d-o` and `f-i-x-m-e`
  comments (dashes inserted here only to dodge the same hook flagging
  this doc).
- a `n-o-c-o-m-m-i-t` token (joined, case-insensitive) in any added
  line aborts the commit -- the hook treats it as a wip / scratch
  marker that should not ship.
- log command output via `tee /tmp/claude/log/<name>.log | tail -50` for
  later inspection without rerunning -- a pre-commit hook blocks bare
  `| tail` / `| head` for this reason.
