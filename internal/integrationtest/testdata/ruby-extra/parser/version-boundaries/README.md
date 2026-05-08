# version-boundaries fixtures

fixture  | passes         | fails | key syntax
---------|----------------|-------|-----------------------------------------------------------
ruby_1_9 | 1.9+           | 1.8   | stabby lambda, {a: 1}, block-local vars
ruby_2_0 | 2.0+           | 1.9   | keyword args, %i, refinements
ruby_2_1 | 2.1+           | 2.0   | 1r, 1i rational/complex literals
ruby_2_3 | 2.3+           | 2.2   | &. safe nav, <<~ squiggly heredoc
ruby_2_5 | 2.5+           | 2.4   | rescue/ensure in do/end blocks
ruby_2_6 | 2.6+           | 2.5   | endless ranges (1..)
ruby_2_7 | 2.7+           | 2.6   | pattern matching, _1, beginless ranges, **nil, ... forwarding
ruby_3_0 | 3.0+           | 2.7   | endless methods, rightward assign, find pattern
ruby_3_1 | 3.1+           | 3.0   | {x:} hash omission, def f(&), ^(expr) pins
ruby_3_2 | 3.2+           | 3.1   | def f(*); g(*), def f(**); g(**) anon forwarding
ruby_3_4 | all (AST diff) | n/a   | it implicit block param (semantic, not syntactic)
ruby_4_0 | 4.0+           | 3.4   | leading ||/&&/and/or line continuation
