# syntax introduced in ruby 3.0 (fails 2.7, passes 3.0+)
#
# boundary: 2.7 -> 3.0
#
# changes exercised:
#   - endless method definition: def f(x) = expr
#   - one-line pattern matching (rightward assignment): expr => pattern
#   - one-line pattern matching (in): expr in pattern
#   - find pattern: [*, pattern, *]

# --- endless method definition ---
def double(x) = x * 2
def add(a, b) = a + b
def greet(name) = "hello #{name}"

# endless method with complex expression
def clamp(x) = [[x, 0].max, 100].min

# endless method on class
class Calculator
  def self.add(a, b) = a + b
  def square(x) = x * x
end

# --- one-line pattern matching: rightward assignment ---
[1, 2, 3] => [a, b, c]
{name: "ruby"} => {name: String => lang}

# rightward assignment with guard-less destructure
42 => Integer => x

# --- one-line pattern matching: in ---
if [1, 2, 3] in [Integer, Integer, Integer]
  :all_ints
end

unless :nope in String
  :ok
end

# --- find pattern ---
case [1, 2, "hello", 3]
in [*, String => s, *]
  s
end

# find pattern with multiple captures
case [1, 2, 3, 4, 5]
in [*, 2, Integer => after_two, *]
  after_two
end

# find pattern nested in hash
case {data: [1, "needle", 3]}
in {data: [*, String => found, *]}
  found
end
