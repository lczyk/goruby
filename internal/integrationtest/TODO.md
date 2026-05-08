# parser compatibility gaps

each entry is valid Ruby (verified with `ruby -c 2.6`).

## resolved (since last commit)

- ~~unary operator methods~~ `def +@`, `def -@`
- ~~rescue with multiple exception classes~~ `rescue A, B => e`
- ~~index assignment on self~~ `self[1] = 2`
- ~~return with modifier in block~~ `{ |x| return x if x > 1 }`
- ~~trailing comma in call args~~ `foo(1, 2,)`
- ~~trailing comma in LHS~~ `a, = 1, 2, 3`
- ~~singleton method on global~~ `def $stdout.foo`
- ~~nested rescue with semicolons~~ `begin; 1; rescue; 2; rescue X; 3; end`
- ~~multiple rescue in method body~~ `def f; 1; rescue A; 2; rescue B; 3; end`
- ~~class variable assignment~~ `@@foo = 1`
- ~~`def [](x)` hang in class body~~
- ~~complex when clauses~~ `when 1..5`, `when /pat/`, `when String`
- ~~class/module/singleton body rescue~~ `class C; rescue; end`
- ~~block-local variables~~ `|i; x|`
- ~~else in begin/rescue/else/ensure~~
- ~~XOR operator~~ `a ^ b`
- ~~lambda with block capture~~ `->(&blk) {}`
- ~~lambda to_proc~~ `[1,2].map(&->(x) { x * 2 })`

## remaining

### medium

- **keyword args in block params** — `|a, *b, c:, d: 1, &blk|`. keyword and block-capture parameters in blocks (`blocks.rb`).
- **parenthesised LHS in multi-assignment** — `(a, b), c = [[1, 2], 3]`. nested parens on LHS (`assignment.rb`).
- **leading-dot method chaining** — `.method` on new line after receiver (`call.rb`).
- **automatic string concatenation** — `"a" "b"`. adjacent string literals (`strings.rb`).
- **defined? with complex args, BEGIN/END blocks** — `misc_keywords.rb`.

### large

- **beginless / endless ranges** — `..5`, `1..`. range prefix/infix handling (`ranges.rb`).
- **case/in pattern matching** — `case x; in 1; y; end`. full pattern-match syntax (`case.rb`).
- **refine / using** — `refine String do ... end`, `using M`. refinement keywords (`refine.rb`).
- **stress test** — combination of above (`stress_test.rb`).
