# ruby syntax changes by version

parser-visible syntax introduced in each ruby version. non-boundaries
(no parser changes): 2.2, 2.4, 3.3, 3.5.

boundary | changes
---------|--------
1.8->1.9 | stabby lambda `->(x) { }`, symbol-key hash `{a: 1}`, block-local vars `\|x; y\|`, encoding magic comment, unicode identifiers, `?c` returns string
1.9->2.0 | optional keyword args `def f(a: 1)`, double-splat `**kwargs`, `%i` / `%I` symbol arrays, `__dir__`, refinements `using` / `refine`
2.0->2.1 | required keyword args `def f(a:)`, rational literals `1r`, complex literals `1i`, combined `1ri`, `def` returns symbol
2.1->2.3 | safe navigation `&.`, frozen string literal pragma, squiggly heredoc `<<~`
2.3->2.5 | `rescue` / `else` / `ensure` in `do`/`end` blocks
2.5->2.6 | endless ranges `(1..)`, non-ascii constant names
2.6->2.7 | pattern matching `case/in`, numbered block params `_1`, beginless ranges `(..3)`, `**nil` in def, argument forwarding `def f(...)`
2.7->3.0 | endless method `def f(x) = expr`, rightward assignment `expr => pat`, one-line `expr in pat`, find pattern `[*, x, *]`
3.0->3.1 | hash value omission `{x:}`, anonymous block forwarding `def f(&); g(&)`, pin expressions `^(expr)`, pin ivars `^@a`
3.1->3.2 | anonymous rest forwarding `def f(*); g(*)`, anonymous kwrest `def f(**); g(**)`, anon rest/kwrest in literals `[1, *]` / `{a: 1, **}`
3.2->3.4 | `it` implicit block param (semantic -- valid syntax everywhere, AST changes), index `a[&b]` / `a[k: v]` removed
3.4->4.0 | leading `\|\|` / `&&` / `and` / `or` line continuation
