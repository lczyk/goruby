# syntax introduced in ruby 3.4 (passes all versions syntactically, but
# semantics change: `it` becomes an implicit block parameter in 3.4+)
#
# boundary: 3.3 -> 3.4
# note: 3.3 had no parser-relevant syntax changes, so this spans 3.2 -> 3.4
#
# this file is syntactically valid in all ruby versions because `it` was
# already a legal method name. the parser change is that in 3.4+, bare `it`
# inside a block without explicit parameters resolves to the implicit
# block parameter instead of a method call. a version-aware parser must
# emit different AST nodes depending on target version.
#
# changes exercised:
#   - `it` as implicit block parameter (semantic, affects AST not syntax)
#   - block passing in index disallowed: a[&b] is now an error
#   - keyword args in index disallowed: a[k: v] is now an error
#     (note: the last two are *removals* -- they FAIL in 3.4+ but passed before.
#      this file only contains syntax that PASSES in 3.4+.)

# --- `it` as implicit block parameter ---
[1, 2, 3].map { it * 2 }
[1, 2, 3].select { it > 1 }
["hello", "world"].map { it.upcase }

# `it` in nested blocks (each block gets its own `it`)
[[1, 2], [3, 4]].map { it.map { it * 10 } }

# `it` with method chaining
[1, 2, 3].map { it.to_s }.map { it.length }

# `it` in various block forms
[1, 2, 3].each do
  _ = it
end
