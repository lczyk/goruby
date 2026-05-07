# Indexing and bracket-disambiguation adversarial examples.
# Seed: the (a[0][a[1]]) pattern from pyramid-scheme's pyra.rb.

a = [[10, 20, 30], [0, 1, 2], [100, 200, 300], 3, 4, 5, 6]
b = a.dup
c = a.dup
d = [[[[[99]]]]]
e = [0]
h = { x: 1 }
x = 1

def foo(*args); args.flatten; end
def bar(*args); args.flatten; end

obj = Struct.new(:data) do
  def foo(*args); data; end
end.new([10, 20, 30])

m = Hash.new { |h, k| h[k] = {} }
m[[0,1]] = 2

# simple chained indexing
a[0][0]
a[0][1][2]
a[0][1][2][3]

# index expression contains indexing
a[a[0]]
a[a[a[0]]]
a[0][a[1]]
a[a[0]][a[1]]
a[a[0]][a[a[1]]]

# parenthesised chained indexing (the original trigger)
(a[0])
(a[0][1])
(a[0][a[1]])
(a[a[0]][a[1]])
((a[0])[a[1]])
((a[0])[(a[1])])

# indexing on parenthesised expressions
(a)[0]
(a)[0][1]
((a))[0]
((a)[0])[1]
(a[0])[a[1]]

# indexing on literals
[1,2,3][0]
[1,2,3][0][0]
([1,2,3])[0]
([1,2,3][0])
"hello"[0]
"hello"[0][0]
("hello")[0]
("hello"[0])

# indexing on hash literals
{a: 1}[:a]
({a: 1})[:a]
({a: 1, b: {c: 2}})[:b][:c]

# indexing on method return values
foo[0]
foo()[0]
foo(1)[0]
foo(1, 2)[0][1]
foo(a[0])[a[1]]
foo(a[0])[a[1]][a[2]]
bar(a[0], b[1])[c[2]]
obj.foo[0]
obj.foo[0][1]
obj.foo(1)[0]
obj.foo(a[0])[a[1]]

# indexing inside argument lists
foo(a[0], b[1])
foo(a[0][1], b[2][3])
foo(a[a[0]], b[b[1]])
[a[0], b[1], c[2]]
[a[0][1], b[c[2]]]

# indexing inside blocks / lambdas
-> { a[0] }
-> { a[0][a[1]] }
-> (*x) { x[0][x[1]] }
proc { |x| x[0] }
proc { |x| x[0][x[1]] }
lambda { |a| a[0][a[1]] }

# indexing with assignment
w = [0, [0, 0, 0], 0, 0, 0, 0, 0]
w[0] = 1
w[1][1] = 2
w[w[0]] = 3
z = [[0, 0], [0, 0]]
z[0][z[1][0]] = 4
w[0], w[1] = 1, 2

# indexing with op-assignment
w = [0, [0, 0, 0], 0, 0]
w[0] += 1
w[1][1] += 2
w[w[0]] += 3
y = [[0, 0], [0, 1]]
y[y[0][0]][y[1][1]] += 4
w[0] ||= 1
w[1][1] &&= 2

# []= method and [] method calls
a_copy = a.dup
a_copy.[](0)
a_copy.[]=(0, 1)

# splat / spread inside brackets
a[*b]
a[*b, 1]
foo(*a[0])
foo(a[0], *b[1])

# indexing in ternary
a[0] ? a[1] : a[2]
(a[0] ? a[1] : a[2])[0]
a[a[0] ? 1 : 2]

# indexing in boolean expressions
a[0] && a[1]
a[0] || a[1]
a[0] && a[1][2]
(a[0] && a[1])[0]
a[0] && b[a[1]]

# indexing mixed with ranges
a[0..1]
a[0..a[1]]
a[a[0]..a[1]]
a[0...a.size]
(a[0..1])[0]
a[0..1][0..1]

# nested structure access
a[0][1..2]
a[0..1][0][1]
a[0..1].map { |x| x[0] }

# indexing on conditional results
(if true then a else b end)[0]
(case x; when 1 then a; else []; end)[0]
(begin; a; rescue; b; end)[0]

# safe navigation with indexing
a&.[](0)
a&.foo[0]
a&.foo&.[](0)

# indexing on string interpolation
"#{a[0]}"
"#{a[0][1]}"
"#{a[a[0]]}"
"#{a[0]}#{b[a[1]]}"
"hello #{a[0][a[1]]} world"

# indexing inside heredocs
<<~HEREDOC
  #{a[0][a[1]]}
HEREDOC

# deeply nested -- stress test
a[b[c[d[e[0]]]]]
a[0][1][2][3][4][5]
a[a[a[a[a[0]]]]]
((((a[0])[1])[2])[3])
(a[(b[(c[0])])])

# indexing on begin/end
begin; a; end[0]

# indexing on block result
foo { [1, 2] }[0]

# negative indices
a[-1]
a[-1][-2]
a[a[-1]]
a[-a[0]]

# computed indices with arithmetic
a[0 + 1]
a[a[0] + a[1]]
a[a[0] * 2][a[1] - 1]
a[(a[0] + 1) * 2]

# multi-arg indexing
a[0, 1]
a[0, 2]

# space-before-bracket disambiguation
a [0]
a [0, 1]
