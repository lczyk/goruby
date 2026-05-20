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
- **phase 3 deferred (second attempt failed)** -- with phase 5 done, the original strip-splice entanglement is gone, but a separate failure mode appeared: when inInterp pushes interpState then lexHeredocStart cursor for the inner heredoc, finishHeredoc's cursor switch clobbers the outer heredoc's state (and l.pending entries from outer don't survive). Full fix would require interpState to also save/restore `heredocRest` and `pending`, plus careful ordering of finishHeredoc vs interpStack pop. Attempt broke 731 integration tests (e.g. `mri-tests/test_alias.rb` heredoc-in-interp constructs); reverted. Cleanly possible but deserves its own focused session.
- **phase 4 done** -- commit `330ee17` (folded in). Nested heredocs (`<<A, <<B`) handled via `pending[0]` pop when `\n` at segment boundary.
- **phase 5 done** -- commit `f68cb41`. stripSquigInterpBody no longer mutates l.input; allocates small stripped+delim buffer and temporarily swaps l.input for body lex. LexRealFiles bytes/op 4.4MB -> 2.4MB (-45%).
- **phase 6 deferred** -- removing heredocPostBody and the splice fallback paths requires phase 3 to be done first.

### measured outcome (vs pre-(4-partial) baseline)

After phase 5:
- `BenchmarkLexRealFiles` bytes/op: 9.3MB -> 2.4MB **(-74%)**.
- `BenchmarkParseRealFiles` bytes/op: 27.6MB -> 19.9MB **(-28%)**.
- `BenchmarkLexRealFiles` allocs/op: 5901 -> 4072 (-31%).
- `BenchmarkLexSquigHeredoc` allocs/op: 14 -> 7 (-50%).

## phase 3 plan (deferred -- own session)

context: 2 attempts in current session both failed. with phase 5 done, the original "strip-splice mutates l.input under cursor" entanglement is gone, but a deeper issue surfaced: cursor-mode inner heredoc inside `#{}` clobbers outer state and `l.pending` invariants don't survive the interpState push/pop.

### failure case

```ruby
assert_no_memory_leak([], "#{<<~"begin;"}", "#{<<~'end;'}", rss: true)
begin;
  ...body lines...
end;
```

token order needed: `IDENT(assert_no_memory_leak) ( [] , STRING_BEG EMBEXPR_BEG STRING_BEG[heredoc-1] body STRING_END[heredoc-1] EMBEXPR_END STRING_END , STRING_BEG EMBEXPR_BEG STRING_BEG[heredoc-2] body STRING_END[heredoc-2] EMBEXPR_END STRING_END , rss : true )` then both heredoc bodies' source-lines lex *after* the outer `)\n`.

splice version handles this because postBody re-injection puts the outer-string-and-rest-of-line literally adjacent to the body region. cursor must achieve the same logical order without input mutation.

### two specific bugs the cursor attempt hit

1. **`interpState` doesn't save `heredocRest`.** when entering `#{}`, the outer heredoc's `heredocRest` (zero if outer is a regular string, non-zero if outer is itself a heredoc body) is not in `interpState`. inner heredoc setup writes to `heredocRest`. inner `finishHeredoc` clears it. on `}` pop, outer's `heredocRest` is not restored -- which matters when outer is a heredoc.

2. **`l.pending` lifetime crosses interpState boundaries.** inner `finishHeredoc` queues `{after-inner-body, l.segEnd}` to the front of `l.pending`. that segment is the original-source continuation *after* both the outer string AND the inner heredoc -- it must survive the `}` pop because the outer string still has rest-of-content to lex, and *its* after-outer-content is what was at `l.pos` before the inner heredoc started.

   so `pending` cannot be naively saved/restored across interpState push/pop: inner additions must persist, but they must be sequenced correctly with outer continuations.

### design

**save/restore split.** `interpState` gets new field `savedHeredocRest segment`. on push: snapshot `l.heredocRest`. on pop: restore it. `pending` is **not** in interpState; lives as a single shared FIFO whose entries are added by inner finishHeredoc and consumed in order.

**ordering invariant.** at the point inner `finishHeredoc` runs, `l.pos` is at the end of the body+delim+newline region. cursor switch:
- push `{l.pos, l.segEnd}` to front of pending -- this is "outer-string-rest-content AND all source past outer string".
- jump `l.pos = heredocRest.start, l.segEnd = heredocRest.end` -- inner heredoc's rest-of-line, which contains the `}` that closes `#{}` plus rest of outer string content up to its closing `"`.

after `}` token is emitted (advancing l.pos inside rest-of-line segment), interpState pops. `heredocRest` restored to outer's (typically zero). lex continues consuming rest of outer string content. when rest-of-line segment exhausts (hit its trailing `\n` or just end), pending pops the next entry -- which is the after-inner-body continuation, correctly resuming original source.

**multi-heredoc-per-line.** `"#{<<A}"foo#{<<B}"bar"`: two heredocs in two `#{}`s on the same outer source line. each pushes its own interpState. each inner finishHeredoc queues its own after-body-continuation to pending front. order in pending after both: `[after-B-continuation, after-A-continuation, ...]`. when both `}`s have popped and outer string closes, lexer reads its source-line trailing content, then on segment exhaustion pops after-B (B's body+delim is later in source than A's), then after-A. wait -- check this order: bodies appear in source as `body-of-A\nA\nbody-of-B\nB\n`. so after-A-continuation is at byte position past `A\n` = where body-of-B starts. but body-of-B has already been consumed by B's heredoc setup (which read it from `l.input` directly). so after-A-continuation should be byte position past `B\n` = where source after both heredocs continues. need to verify the front-push order is correct, or use a different ordering.

### concrete steps

1. **add `savedHeredocRest segment` to interpState struct.** update `pushInterp` and `restoreHeredocState` to snapshot/restore it (in addition to the existing heredoc fields).
2. **enable cursor for `inInterp && \n` case** in `lexHeredocStart`. straightforward analogue of the non-interp `\n` branch.
3. **enable cursor for `inInterp && }` case** in `lexHeredocStart`. `realNl` scan to find the body boundary, then `heredocRest = {restStart, realNl+1}`, jump `l.pos = realNl+1`.
4. **trace through `"#{<<A}rest"` by hand** to verify token Pos values, NEWLINE suppression, EMBEXPR_END ordering.
5. **add targeted unit tests** in `lexer/coverage_extra_test.go`:
   - `"#{<<EOS}\nbody\nEOS\n"` -- simplest heredoc-in-interp.
   - `"#{<<~MSG}\n  body\nMSG\n"` -- squig variant; verifies phase 5 swap + cursor coexist.
   - `"a#{<<A}b#{<<B}c"\nbody-a\nA\nbody-b\nB` -- two heredocs in two interps on same outer source line.
   - heredoc-in-interp-in-heredoc (the existing adversarial fixture).
6. **iterate against `make test`** until lexer+parser green; then `make oracle`. expect `mri-tests/test_alias.rb`, `test_settracefunc.rb`, `test_string.rb` etc. to be the canaries.
7. **once green, do phase 6**: remove `heredocPostBody` field, the 4 body-end re-injection sites in `finishHeredoc`, the splice fallback branches in `lexHeredocStart`, and the `heredocPostBody` save in interpState.

### risk areas

- **token Pos consistency.** cursor mode emits tokens at positions in original (unspliced) source. splice mode emitted at positions in mutated input. line-table (`AddLine`) consumes NEWLINE tokens; positions must be increasing-enough that line lookups don't degrade. test by checking parser error messages line/col fields against MRI golden files.
- **`HeredocStripped` flag** on STRING_END for squig must still be set. phase 5 keeps it set inside `stripSquigInterpBody`; verify still propagates after swap restoration.
- **nested heredoc inside an outer heredoc body that's inside an outer-outer interp.** existing test: `internal/integrationtest/testdata/ruby-extra/parser/adversarial/heredoc_in_heredoc_interp.rb`. run as canary.

### estimate

4-6 hours focused. core change is small (~50 LOC delta) but verification surface is large. land on its own branch.
