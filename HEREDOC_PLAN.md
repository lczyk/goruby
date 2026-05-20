# heredoc input-rewrite removal -- plan

context: PERF_IDEAS.md item (4). post-(4-partial) (delim O(n^2) fix landed in `52e895e`), the remaining heredoc hotspot is full-input string-concat splicing across 7 sites in `lexer/lexer.go`. accounts for ~700MB-1.3GB allocs on `BenchmarkParseRealFiles`. expected gain: ~10% parser throughput, -40% lexer allocs.

## scope: 7 splice sites in `lexer/lexer.go`

- `lexHeredocStart:1931` -- rest-of-line splice (main path)
- `lexHeredocStart:1943` / `:1945` / `:1949` -- inInterp variants
- `stripSquigInterpBody:2053` -- splice stripped body into input
- `lexHeredocBody:2095` -- re-inject postBody at body-end (empty body)
- `lexHeredocBody:2162` -- re-inject postBody (delim-match branch)
- `lexHeredocContent:2197` -- re-inject postBody (empty body)
- `lexHeredocContent:2262` -- re-inject postBody (delim-match branch)

(line numbers from post-(4-partial) commit; will drift.)

## core model: cursor stack on immutable input

`l.input` becomes write-once. lexer consumes via:
- `l.pos int` -- read position
- `l.segEnd int` -- end of current view of `l.input`
- `l.pending []segment` -- FIFO of upcoming `(start,end)` ranges to consume after current view exhausts

```go
type segment struct { start, end int }

func (l *Lexer) next() rune {
    if l.pos >= l.segEnd {
        if len(l.pending) == 0 {
            l.width = 0
            return eof
        }
        seg := l.pending[0]
        l.pending = l.pending[1:]
        l.pos = seg.start
        l.segEnd = seg.end
        l.start = l.pos // never let a token span a segment boundary
    }
    // ... existing rune-decode logic, bounded by l.segEnd ...
}
```

invariants:
- every emitted token's span `[l.start, l.pos)` lies within a single segment.
- enforced by `l.ignore()` at every heredoc transition site AND at segment-pop.
- `l.input` never mutated post-`New()`.
- all 68 existing `l.input[i]` / `l.input[a:b]` reads still valid (input is immutable, indices map to original bytes).

## transformations per site

### non-inInterp `lexHeredocStart` (main path)

```
OLD:
  l.heredocPostBody = l.input[restStart : nlPos+1]
  l.input = l.input[:restStart] + "\n" + l.input[nlPos+1:]
NEW:
  l.heredocRest = segment{restStart, nlPos + 1}
  l.pos = nlPos + 1   // jump directly to body
  l.start = l.pos
  // l.segEnd unchanged; body lex scans body from input directly
```

zero allocs. `heredocRest` field replaces `heredocPostBody string`.

### body-end re-injection (4 sites in lexHeredocBody / lexHeredocContent)

```
OLD:
  l.input = l.input[:l.pos] + l.heredocPostBody + l.input[l.pos:]
  l.heredocPostBody = ""
NEW:
  bodyEnd := l.pos
  rest := l.heredocRest
  l.heredocRest = segment{}
  l.pending = prepend(l.pending, segment{bodyEnd, l.segEnd})
  l.pos = rest.start
  l.segEnd = rest.end
  l.start = l.pos
```

zero alloc per transition; `prepend` amortizes to zero if `pending` cap pre-set.

### nested heredocs (`puts <<A, <<B`)

while reading rest-of-A segment, `<<B` is hit. setup for B:
- B's rest range = `{l.pos, l.segEnd}` (clamped to current segment -- A's rest ends at `\n`, B's rest can't extend past it)
- B's body content starts at `l.pending[0].start` (A's after-body continuation) -- because in source order, B's body lines follow A's delim line
- scan forward in `l.input` from `l.pending[0].start` to find B's delim line; that's B's body+delim end
- `pending[0]` is replaced by `{bodyB_delim_end, pending[0].end}` -- the after-B continuation
- transition then mirrors single-heredoc body-end

needs nested-aware setup logic in `lexHeredocStart`: when `<<EOS` is hit AND `len(l.pending) > 0` AND no `\n` exists between `l.pos` and `l.segEnd`, body source is in `pending[0]`, not in current segment.

### inInterp branch in `lexHeredocStart` (lines 1932-1950)

`<<EOS` inside `#{}`: rest-of-line includes closing `}` and beyond. body lives in main source after the next real `\n`.

splice version captures `restStart..realNl` + `\n` separately and splices to remove `[restStart..realNl+1]`. cursor version collapses to one segment:

```
NEW:
  l.heredocRest = segment{restStart, realNl + 1}  // includes }, rest of line, and \n
  l.pos = realNl + 1   // jump to body
  l.start = l.pos
```

identical post-body-end transition to non-inInterp.

### `stripSquigInterpBody`

```
OLD:
  // strings.Builder produces stripped body, then:
  l.input = l.input[:bodyStart] + stripped + l.input[delimLineContentStart:]
NEW:
  // generalize segment to optionally carry overlay data
  l.squigStripped = stripped   // separate string
  // synthesize one segment that reads from l.squigStripped
  // body lex reads from synthesized segment; on exhaust, lexer resumes
  // in l.input at delimLineContentStart
```

requires generalizing `segment` from `{start, end int}` to `{data string, start, end int}` (when `data == ""`, reads from `l.input`). `next()` consults `seg.data` to source bytes.

**decision point**: this is ~7% of allocs, not 17%. if previous phases hit alloc target, skip phase 5 to limit diff surface.

## phasing (one commit per phase; full `make test` + `make oracle` after each)

1. **phase 1: cursor primitives** -- add `segEnd`, `pending` to Lexer struct; convert `next()` / `peek()` to consult them. all existing tests pass unchanged (pending empty, segEnd == `len(input)`, behavior identical).
2. **phase 2: non-inInterp main path + 4 body-end sites** -- replace splice with `heredocRest segment` + body-end transition. inInterp branch keeps old splice for now. covers ~70% of heredocs.
3. **phase 3: inInterp branch** -- collapse the 3 splice variants in `lexHeredocStart`'s inInterp path to cursor segments.
4. **phase 4: nested heredocs** -- nested `<<` during rest-of-line consumption: compute B's body range from `pending[0]`, replace `pending[0]` with after-B continuation. probably mostly works after phase 2-3; verify w/ targeted fixtures.
5. **phase 5: stripSquigInterpBody (optional)** -- generalize `segment` for overlay data; route squig body through synthesized segment. skip if alloc target already met.
6. **phase 6: cleanup** -- remove `heredocPostBody string` field, dead splice code paths, save-state code (lines 76-92).

## risks + mitigations

- **subtle off-by-one in segment ranges.** defensive assertions during dev (`l.start >= seg.start`, `l.pos <= l.segEnd`); strip before commit.
- **heredoc-inside-interp-inside-heredoc** exponentially harder. existing fixtures cover some shapes; oracle has more. run oracle after every phase.
- **token-span emit (`l.input[l.start:l.pos]`)** must always stay within one segment. enforce via `l.ignore()` at every transition + segment-pop.
- **`heredocPostBody != ""` checks at 4 sites.** replace with `l.heredocRest != (segment{})` or dedicated `inHeredoc bool`.
- **`save`/`restore` state code at lexer.go:76-92** copies `heredocPostBody` and `heredocDelim`. needs equivalent for new fields.

## verification (every phase)

1. `go test ./lexer/ ./parser/` -- fast feedback.
2. `make test` -- full unit + integration.
3. `make oracle` -- MRI parsetree diff. heredoc errors surface here.
4. focused bench: `BenchmarkLexHeredoc*`, `BenchmarkLexSquigHeredoc`, `BenchmarkLexRealFiles`, `BenchmarkParseRealFiles`.
5. commit only when green AND allocs improved (or held).

## expected outcome

rough per-phase alloc delta (from current 4368 allocs/op on LexRealFiles):
- phase 2: -50% on heredoc-heavy benches; -10-15% on LexRealFiles.
- phase 3-4: another -10-15% on LexRealFiles.
- phase 5: -5-10% additional.
- **target: LexRealFiles allocs/op 4368 -> ~2500. ParseRealFiles throughput +~10%.**

## work estimate

- phase 1: 2-3h. mechanical, tight scope.
- phase 2: 4-6h. core conversion, careful testing.
- phase 3: 3-4h. inInterp gymnastics.
- phase 4: 2-3h. nested cases.
- phase 5: 4-6h or skip.
- phase 6: 1h.
- **total: 1.5-2 days focused work. standalone branch.**

## status

- (4-partial) delim O(n^2) fix landed -- commit `52e895e`. LexRealFiles allocs -26%.
- **phase 1 done** -- commit `77fe3d2`. cursor primitives added; behavior unchanged.
- **phase 2 done** -- commit `330ee17`. Non-interp main path + 4 body-end sites use cursor. byteAt lookahead added.
- **phase 3 done** -- commits `075c07b` (3a: inInterp+\n cursor) and `7a642ce` (3b: inInterp+} cursor). Root cause of earlier 731-test breakage identified: `stripSquigInterpBody` consults `l.input` by absolute position, ignoring the cursor segments. The fix is the same nested-heredoc handling already used for the non-interp path: when the body source would land at the boundary of the current cursor segment, pop the next pending segment and use its start as the body source.
- **phase 4 done** -- commit `330ee17` (folded in). Nested heredocs (`<<A, <<B`) handled via `pending[0]` pop when `\n` at segment boundary.
- **phase 5 done** -- commit `f68cb41`. stripSquigInterpBody no longer mutates l.input; allocates small stripped+delim buffer and temporarily swaps l.input for body lex. LexRealFiles bytes/op 4.4MB -> 2.4MB (-45%).
- **phase 6 done** -- commit `d7d664f`. Removed `heredocPostBody`, splice fallbacks, `syncSegEnd`, `cursorMode` bool tracking. -38 LOC net.

### measured outcome (vs pre-(4-partial) baseline)

After phase 5:
- `BenchmarkLexRealFiles` bytes/op: 9.3MB -> 2.4MB **(-74%)**.
- `BenchmarkParseRealFiles` bytes/op: 27.6MB -> 19.9MB **(-28%)**.
- `BenchmarkLexRealFiles` allocs/op: 5901 -> 4072 (-31%).
- `BenchmarkLexSquigHeredoc` allocs/op: 14 -> 7 (-50%).

## phase 3 root cause (resolved)

original blocker: when inner heredoc inside `#{}` used cursor and the rest-of-line ended at the boundary of the current cursor segment (= where the next pending segment begins), `stripSquigInterpBody` scanned `l.input` from `l.pos` without respecting segments. for chained `"#{<<~"begin;"}\n#{<<~"end;"}"`-style constructs, the second heredoc's body source landed at the segment boundary and strip read the wrong region (predecessor's delim leaked into successor's body).

resolution: mirror the non-interp nested-heredoc handling. when `realNl+1 >= l.segEnd && len(l.pending) > 0`, pop `pending[0]` and use its `start` as the body source. one extra branch, no new state. see `lexHeredocStart` (phase 3b commit `7a642ce`).

interpState already saves `heredocRest` (added in phase 2), so outer-heredoc state survives the `#{}` boundary.

## remaining loose ends

minor, not blocking:

- **nested squig-inside-squig collision (latent)**: `stripSquigInterpBody` writes `l.heredocSavedInput`/`l.heredocSavedSegEnd`/`l.heredocSquigRestorePos`. If an outer squig heredoc's body contains `#{}` whose body contains another squig heredoc, the inner strip call overwrites the outer's saved fields. No test exercises this (typical heredoc-in-interp uses non-squig); oracle clean. Fix would require either (a) saving/restoring these three fields in `interpState`, or (b) stacking them. Tracked by the canary test `TestLexerHeredocNestedSquigInSquigInterp`.
- **`stripSquigInterpBody` name**: function no longer strips in-place; builds buffer + swaps. Could rename to `setupSquigBodyBuffer` for clarity. Cosmetic.
- **`l.heredocSavedInput != ""` sentinel**: works because real input is never empty, but a dedicated bool would be cleaner. Cosmetic.
- **token Pos in cursor segments**: emitted tokens have positions in original (unspliced) source -- this is correct, matches actual file offsets, but means error messages in heredoc-in-interp constructs may report positions that "jump backwards" relative to emit order. `AddLine` (called from `nextToken` on NEWLINE) handles this since lines are tracked by Pos, not emit order. No issue observed.
