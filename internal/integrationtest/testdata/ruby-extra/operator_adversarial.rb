# Operator and disambiguation adversarial examples.

# Arithmetic
1 + 2
3 - 4
5 * 6
7 / 8
9 % 10
2 ** 3     # power
2 ** 3 ** 4  # right-associative power

# Comparison
a < b
a > b
a <= b
a >= b
a == b
a != b
a <=> b    # spaceship
a === b    # case equality

# Pattern matching operators
a =~ b     # match
a !~ b     # not match

# Logical
a && b
a || b
a and b
a or b
!a
not a

# Bitwise
a & b      # bitwise AND
a | b      # bitwise OR
a ^ b      # XOR
a << b     # left shift / append
a >> b     # right shift
~a         # complement

# Range
1..10      # inclusive
1...10     # exclusive
..10       # beginless range
1..        # endless range (requires Ruby 2.6+)

# Assignment operators
a = 5
a += 1
a -= 1
a *= 2
a /= 2
a %= 3
a **= 2
a &= 1
a |= 1
a ^= 1
a <<= 1
a >>= 1
a &&= true
a ||= false

# Parallel assignment
a, b = 1, 2
a, *b = [1, 2, 3]
a, b, *c = 1, 2, 3, 4

# Captures / block arguments
x = &block
foo(&block)
foo.bar(&baz)

# Lonely operator (&.)
foo&.bar
foo&.bar&.baz
foo&.bar(1, 2)

# Lambda
-> { 1 }
->(x) { x + 1 }
->(x, y) { x + y }

# Singleton class
class << self
  def foo; end
end

# << after CLASS is LSHIFT, not heredoc
class Foo << Bar; end

# << with identifiers should be heredoc, not LSHIFT
# (tested in heredoc_adversarial.rb)

# Ternary
a ? b : c
a ? b ? c : d : e  # nested ternary

# Scope operator
Foo::Bar
::Baz

# Hash syntax
{ :key => "value" }
{ "key" => value }
{ key: "value" }   # label syntax

# Splat
a = *b
foo(*args)
a, *rest = [1, 2, 3, 4]

# Various operator adjacency
a+-b       # a + (-b)
a-+b       # a - (+b)
a*-b       # a * (-b)
a**-b      # a ** (-b)
