# goruby parser/printer TODO

remaining `TestMRIParseTreeDiff` failures by cluster. ~159 fails as of writing.

## status

| # | item | status |
|---|------|--------|
| 1 | adjacent string-literal concatenation | [DONE] 6a78815 |
| 2 | local-variable tracking (vcall vs LocalVariableRead) | open |
| 3 | single-line def with `;` separator | tried, reverted (per-version MRI divergence) |
| 4 | comment / blank-line preservation for `__LINE__` | open (high risk) |
| 5 | string interpolation `#@var` / `#$var` / `#@@var` shorthand | [DONE] b40f374 |
| 6 | hash splat key-order preservation | [DONE] 6631b11 |
| 7 | BlockParametersNode emission when locals exist | blocked on #2 |
| 8 | magic encoding comment preservation | noop -- ParseComments mode covers it |
| 9 | splat key in multi-assign LHS positional | open |
| 10 | regex `%r:...:` form normalisation | open (small, 2 fails) |
| 11 | anonymous block-pass arg `&` in calls | works in practice |
| 12 | misc small clusters | tied to others |

## remaining open items

### 2. local-variable tracking (~30 fails)

**files**: minitest/spec (12), test_module (6), test_box (some), test_rational,
test_call (1), other minitest

**pattern**: `x = ...; x` -- second `x` is `LocalVariableReadNode` in MRI.
we emit `CallNode flags: variable_call` because we don't track defined
locals. roundtrip stable but tree shape differs.

**fix shape**: add scope stack (`map[string]bool` per scope frame), push on
def/block/lambda entry, mark names from assignments/params. at ident parse
time, if name in scope -> LocalVariable, else vcall.

**risk**: high. big surface area -- touches every assignment, every block,
every def, every iteration call. many existing tests assume vcall.

### 3. single-line def with `;` separator (~15 fails)

**files**: test_struct, test_assignment older versions, test_range,
test_object, test_eval

**pattern**: `def f(a); body; end` -- MRI 3.x+ wraps body as `NODE_BLOCK
[NODE_BEGIN(null), body]`; older MRI versions only wrap when trailing `;`
present. cannot satisfy both with single emit form.

**status**: tried `SingleLine bool` flag on FunctionLiteral + `;`-based
printer; broke ~120 tests on older MRI. reverted.

**next**: maybe version-conditional emit? Or accept the loss and only
target 3.4+ behaviour, marking older as skip.

### 4. comment / blank-line preservation for `__LINE__` (~20 fails)

**files**: test_box (7), test_enumerator (5), misc_keywords (10), test_integer

**pattern**: source has comments / blank lines. we strip them. subsequent
statements have wrong line numbers in MRI's `__LINE__` / `nd_line` tracking.

**risk**: high. earlier attempt broke 93 roundtrips. would need careful
per-node Pos handling, fallback when Pos unset.

### 9. splat in multi-assign LHS positional (~3 fails)

**files**: test_assignment older versions

**pattern**: `(x1.y1.z, *x2[1, 2, 3], self[4]) = ...` -- complex multi-target
with splat-of-index.

### 10. regex `%r:...:` form normalisation (~2 fails)

**files**: test_regexp (2)

**pattern**: `%r:\::` -- `\:` is delimiter escape, MRI normalises to just
`:` as regex content. we keep `\:`.

**fix shape**: lex-time normalize: strip `\` before delim char in regex
content for `%r` form.

**risk**: low. small surface.

### misc

bouncy-lang/interpreter (3), test_struct (7), test_module (6),
test_m17n (5 regex interp), test_call (3 ParenthesesNode wrap),
gems/minitest/spec (12 local-var tracking), test_io / test_process
/ test_rational / test_file_exhaustive (8 each, mostly adjacent-string
edge cases or heredoc-content quirks left).
