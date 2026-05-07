# Indexing and bracket-disambiguation adversarial examples.
# Seed: the (a[0][a[1]]) pattern from pyramid-scheme's pyra.rb.

# a[N] == N for all indices, so a[a[a[...[0]...]]] == 0.
a = [0, 1, 2, 3, 4, 5, 6]
na = [[10, 20, 30], [0, 1, 2], [100, 200, 300]]
def foo(*args); args.flatten; end
obj = Struct.new(:data) { def foo(*args); data; end }.new([10, 20, 30])

# -- recursive self-indexing at increasing depth --
a[a[0]]
a[a[a[0]]]
a[a[a[a[0]]]]
a[a[a[a[a[0]]]]]
a[a[a[a[a[a[0]]]]]]
a[a[a[a[a[a[a[0]]]]]]]

# -- chained indexing where inner index is itself indexed --
na[0][a[1]]
na[a[0]][a[1]]
na[a[0]][a[a[1]]]
na[a[a[0]]][a[a[a[1]]]]

# -- parenthesised chained indexing (the original pyra.rb trigger) --
(na[0][a[1]])
(na[a[0]][a[1]])
((na[0])[a[1]])
((na[0])[(a[1])])
(((na)[a[0]])[a[1]])
((((na)[a[0]]))[a[1]])

# -- extra parens at every nesting level --
(a[(a[(a[0])])])
((a[0]))
(((a[0])))
((((a[0]))))
(a[((0))])
(a[(((0)))])

# -- indexing on parenthesised expressions --
((na)[0])[1]
(na[0])[a[1]]
((a))[((0))]
(((a)))[0]

# -- indexing on literal constructors --
[[1,2],[3,4]][0][1]
([1,2,3])[0]
([1,2,3][0])
({a: 1, b: {c: 2}})[:b][:c]

# -- indexing on method call results with nested index args --
foo(a[0])[a[1]]
foo(a[a[0]])[a[a[1]]]
foo(na[0][a[1]])[0]
obj.foo(a[0])[a[1]]
obj.foo[a[a[0]]]

# -- nested indexing inside argument lists --
foo(a[a[0]], na[a[1]][a[2]])
foo(na[a[0]][a[1]], a[a[a[2]]])
[a[a[0]], na[a[1]][a[2]]]

# -- indexing inside lambdas / procs --
-> { na[0][a[1]] }
-> (*x) { x[x[0]] }
proc { |x| x[0] if x.is_a?(Array) }
lambda { |v| v.is_a?(Array) ? v[v.size - 1] : v }

# -- indexing with assignment: nested lhs --
w = [[0, 0, 0], [0, 0, 0], 0, 0]
w[0][w[1][0]] = 9
w[a[0]][a[1]] = 8
z = [[0, 0], [0, 1]]
z[z[0][0]][z[1][1]] = 7
z[z[0][0]][z[1][1]] += 10

# -- explicit .[] and .[]= method call form --
a_copy = a.dup
a_copy.[](0)
a_copy.[]=(0, 99)
a_copy.[](a_copy.[](1))

# -- safe navigation with indexing --
a&.[](0)
nil&.[](0)
a&.[](a&.[](0))

# -- indexing on conditional / control-flow results --
(if true then na else [] end)[0][1]
(case a[1]; when 1 then na; else []; end)[0][a[1]]
(begin; na; rescue; []; end)[0][a[1]]
(true ? na : [])[0][a[1]]
(a[1] == 1 ? na[0] : na[1])[a[2]]

# -- indexing in boolean short-circuit --
a[1] && na[a[1]][a[2]]
a[0] || na[a[1]][a[2]]
(a[1] && na[a[1]])[a[2]]

# -- indexing mixed with ranges --
a[a[0]..a[2]]
na[0][a[0]..a[1]]
(a[0..2])[a[1]]
a[0..2][a[0]..a[1]]

# -- indexing inside string interpolation --
"#{na[0][a[1]]}"
"#{na[a[0]][a[a[1]]]}"
"prefix #{na[a[0]][a[1]]} middle #{a[a[a[2]]]} suffix"

# -- indexing inside heredoc interpolation --
<<~HEREDOC
  val: #{na[0][a[1]]}
  deep: #{na[a[0]][a[a[1]]]}
HEREDOC

# -- deeply nested across multiple [] on same receiver (Integer#[] is bit-index) --
a[1][0]
a[3][0]
a[3][1]
a[5][0][0]
a[a[3]][a[1]]
a[a[a[3]]][0]

# -- splat inside brackets --
a[*[0]]
a[*[0, 2]]
foo(*na[0])
foo(na[0][a[1]], *na[1])

# -- indexing on begin/end and block results --
begin; na; end[0][1]
foo { na }[0]

# -- space-before-bracket (argument list, not indexing) --
a [0]
a [0, 2]
foo [a[0]]
foo [a[0], a[1]]

# -- ternary with indexing on both branches --
(a[1] > 0 ? na[0] : na[1])[a[2]]
na[a[1] > 0 ? 0 : 1][a[1] > 0 ? 2 : 0]

# -- indexing result used as method receiver --
na[0][a[1]].to_s
na[a[0]][a[1]].to_s.length
(na[0][a[1]]).to_s
((na[0])[a[1]]).class

# -- compound nested: index + method + index --
na[0].dup[a[1]]
na[0].dup[a[a[1]]]
na.dup[a[0]][a[1]]
na.dup[a[0]].dup[a[1]]
