# syntax introduced in ruby 1.9 (fails 1.8, passes 1.9+)
#
# boundary: 1.8 -> 1.9
#
# changes exercised:
#   - stabby lambda syntax: ->() {}
#   - symbol-key hash literals: {a: 1}
#   - block-local variables: proc { |x; y| }
#   - encoding declaration (magic comment)
# encoding: utf-8

# stabby lambda
f = -> { 1 }
g = ->(x) { x + 1 }
h = ->(x, y) { x + y }

# stabby lambda with do/end
k = ->(x) do
  x * 2
end

# symbol-key hash literals
a = {a: 1, b: 2, c: 3}
c = {a: 1, :b => 2}

# block-local variables (semicolon separator)
x = 10
proc { |a; x| x = a }.call(99)
proc { |a, b; c, d| c = a; d = b }.call(1, 2)
lambda { |a; x, y| x = a }.call(5)
