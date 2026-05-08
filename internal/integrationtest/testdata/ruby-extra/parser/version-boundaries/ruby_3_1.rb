# syntax introduced in ruby 3.1 (fails 3.0, passes 3.1+)
#
# boundary: 3.0 -> 3.1
#
# changes exercised:
#   - hash value omission: {x:} == {x: x}
#   - anonymous block forwarding: def f(&); g(&); end
#   - pin operator with expressions in patterns: ^(expr)
#   - pin operator with instance/class vars: ^@a, ^@@b
#   - parentheses optional in one-line pattern matching
#   - short-hand hash key in pattern matching

# --- hash value omission ---
x = 1
y = 2
h = {x:, y:}

# hash value omission mixed with regular pairs
name = "ruby"
version = 3
info = {name:, version:, type: :language}

# hash value omission in method call
def show(x:, y:)
  [x, y]
end
a = 10
b = 20
show(x: a, y: b)

# --- anonymous block forwarding ---
def wrap(&)
  puts "before"
  yield
  puts "after"
end

def relay(&)
  wrap(&)
end

# anonymous block with other params
def with_args(a, b, &)
  yield(a, b)
end

# --- pin operator with expressions ---
n = 3
case 6
in ^(n * 2)
  :matched
end

case "HELLO"
in ^("hello".upcase)
  :matched
end

# pin operator with instance variables
class Matcher
  def initialize(pattern)
    @pattern = pattern
  end

  def match?(value)
    case value
    in ^@pattern
      true
    else
      false
    end
  end
end

# --- parentheses optional in one-line pattern matching ---
[1, 2, 3] => _, x, _

# --- short-hand hash key in pattern matching ---
case {name: "ruby", version: 3}
in {name: /ruby/}
  :ok
end
