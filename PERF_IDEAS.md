# perf ideas

benchmarks run 2026-05-19 on Apple M3. raw logs in `/tmp/claude/log/parser-*.log`, `lexer-*.log`.

## throughput snapshot

| bench | MB/s | allocs/op |
|---|---|---|
| ParseRealFiles | 59 | 103,720 |
| LexRealFiles | 89 | 5,901 |
| ParseFuzzCorpus | 3.5 | 219 |
| ParseDeepUnary | 19 | 44 |
| ParseDeepParens | 26 | 25 |

parser is bottleneck. ~1.5x slower than lexer per byte. 100k allocs / 27MB source = ~3.8 allocs/KB. cpu profile dominated by GC syscalls (`kevent` 40%, `madvise` 19%, `pthread_cond_wait` 6%) -- classic alloc-pressure shape. fix allocs, free real cpu.

## top alloc hotspots (by alloc count)

1. `parser.parseExpressionList` -- 4.95M flat (16.6%)
2. `parser.parseBlockStatement` -- 4.49M (15%)
3. `parser.parseSymbolLiteral` -- 3.03M (10%)
4. `parser.parseInterpolatedString` -- 2.44M (8%)
5. `ast.arenaAlloc[Identifier]` -- 1.21M (4%)

## top alloc hotspots (by bytes)

1. `lexer.lexHeredocStart` -- 1.32GB cum / 770MB flat (16.8%)
2. `ast.arenaAlloc[Identifier]` -- 1.01GB (12.9%)
3. `lexer.lexHeredocContent` -- 668MB (8.3%)
4. `ast.arenaAlloc[ContextCallExpression]` -- 624MB (7.8%)
5. `lexer.stripSquigInterpBody` -- 569MB (7.1%)

## root causes + proposed refactors

ranked by ROI / size of change.

### 1. tracing infra cost on every parse function -- big, easy

`parser.go` has 88 `defer trace.TraceCtx(p.ctx)()` callsites. `TraceCtx` is `//go:noinline`, calls `ctx.Value(tracerKey{})` + type-assert + returns closure. profile shows `trace.GetTracer` at 4% flat cpu even with tracer off. each call also pays `defer` overhead.

**refactor:** gate tracing behind build tag (`//go:build trace`) or parser-level bool sampled once. zero-tracer path should be `if p.trace { ... }` inlined check, not generic ctx lookup. expected gain: ~5-10% cpu + removes 88 defer setups per non-trivial parse.

### 2. heredoc lexer rewrites entire input string -- big

`lexHeredocStart` at `lexer/lexer.go:1931`:
```go
l.input = l.input[:restStart] + "\n" + l.input[nlPos+1:]
```
allocates fresh full-input string **every heredoc**. same pattern in `stripSquigInterpBody` (line 2053). combined ~1.3GB allocs.

`l.heredocDelim += string(r)` (line 1899) is O(n^2) string concat per delim char.

**refactor options, increasing in scope:**
- **(min)** build heredoc delim into `[]byte` then `string()` once.
- **(medium)** make `Lexer.input` a `[]byte`. splice via small "edits" overlay; lexer reads via `read(pos)` helper consulting overlay. heredoc rest-of-line saved as `(start, end)` index pair, not string copy.
- **(max)** redesign heredocs as deferred queue: `<<EOS` seen -> push pending heredoc record (delim + body-start-position-after-eol). main lexer keeps moving through current line. on `\n`, drain queued heredocs by scanning forward, emit tokens, resume. no string mutation.

expected gain: 15-25% throughput on real ruby (heredocs common in tests/specs).

### 3. parser scratch slices alloc on every call -- medium, easy

hot funcs alloc fresh slices each call:
- `parseExpressionList:4834` -- `list := []ast.Expression{}` then `append`. cap 0, doubles. preallocate cap 4-8.
- `parseExpressionList:4917` -- `make([]token.Type, 0, len(end))` **inside comma loop** -- one alloc per element. hoist outside loop; reset with `[:0]`.
- `parseSymbolLiteral:2776-2781` -- `accepted := []token.Type{...}` literal + two `append(..., x...)` per call (3M calls). list is **constant**. promote to package-level `var symbolAcceptedTokens = ...`.
- `parseCallArguments`, `parseParameters` similar -- audit for `[]T{}` + append where cap bounded.

**refactor:** sweep `:= []T{}` / `:= make([]T, 0)` in hot funcs. preallocate cap or move constants to package scope. expected gain: shave 20-30% of 100k allocs/op.

### 4. defer-closure heap alloc for state save/restore -- medium, easy

`parser.go:4740` and `:4833`:
```go
prevSuppressHR := p.suppressHashrocket
p.suppressHashrocket = true
defer func() { p.suppressHashrocket = prevSuppressHR }()
```
closure captures `prevSuppressHR` -- escapes to heap. `defer func literal` also slower than open-coded defer.

**refactor:** restore at each `return` site, or single-tail return with restore there. or stack-allocated guard:
```go
type suppressGuard struct { p *parser; prev bool }
func (g *suppressGuard) restore() { g.p.suppressHashrocket = g.prev }
g := suppressGuard{p, p.suppressHashrocket}; p.suppressHashrocket = true
defer g.restore()
```
escape analysis usually keeps stack-bound.

### 5. arena AST nodes -- mostly good -- ~~one squeeze left~~ SKIPPED

arena slabs eliminate per-node `runtime.newobject`. `arenaAlloc[Identifier]` tops 1GB / 1.2M objects -- proportional to source size, not directly avoidable.

**audit done -- drop-`Value` not feasible.** scanned ~70 Identifier construction sites in `parser/parser.go`:
- ~28 trivial sites: `Value = p.curToken.Literal` (would be safe to drop).
- ~42 computed-name sites where `Value != Token.Literal`:
    - method suffix: `name + "="` (parseAliasName)
    - bracket methods: `"[]"`, `"[]="` (parseAliasName, parseUndefName, multiple parseMethodName paths)
    - lambda call: `"call"`
    - placeholder: `"nil"` (parseOneParameter forwarding paths)
    - computed: `receiver.String()`, `ivar.String()`, `sym.String()`, `sym.Value.String()`, locally-computed `name :=` (alias, hash sym keys, method declarations, parameter destructuring, etc.)
    - empty placeholder: `""` (parseUndefName error path)

dropping the field saves 16B x 1.2M = ~19MB total. modest. options for the 42 computed sites all have issues:
- **mutate `Token.Literal`** + accessor: risks breaking `End()` (`Token.Pos + len(Value)` -- if Value=="[]=" but Token spans only "[", End() is wrong); also other code treats `Token.Literal` as source text.
- **separate override field**: same 16B, no saving.
- **keep field, drop trivial writes**: saves nothing (struct size unchanged).

none clean. moving on. similar audit for other arena types where Token + something-from-Token coexist -- `ContextCallExpression` (#2 byte-hotspot, 624MB) most promising.

### 6. lexer's `New(string(src))` -- small

bench does `lexer.New(string(src))` -- explicit byte->string copy. parser passes through same way. lexer-on-`[]byte` (see (2)) eliminates copy.

### 7. lexer variadic `acceptOneOf` / `currentTokenOneOf` -- small

called constantly with variadic `token.Type` args. each variadic call allocs `[]token.Type` unless escape analysis pins to stack. disassemble to check; if heap, replace hot 1-2 arg cases with explicit `currentTokenIs(a) || currentTokenIs(b)`.

## suggested attack order

independent. land in order w/out conflict.

1. **trace gating** (build tag or single bool) -- 1hr, ~5-10% cpu.
2. **parser scratch-slice sweep** (hoist constants, preallocate caps, kill in-loop make) -- 2-3hrs, ~20-30% alloc count drop.
3. **defer-closure refactor** in `parseExpressionList` / `parseCallArguments` -- 30min.
4. **heredoc input-rewrite removal** -- biggest single win, biggest scope. 1-2 days. needs careful test coverage (heredocs tricky). standalone branch.

(1)-(3) plausibly push `ParseRealFiles` to ~80-90 MB/s (parity with lexer). (4) opens room beyond.
