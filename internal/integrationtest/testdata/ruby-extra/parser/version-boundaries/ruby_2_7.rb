# syntax introduced in ruby 2.7 (fails 2.6, passes 2.7+)
#
# boundary: 2.6 -> 2.7
#
# changes exercised:
#   - pattern matching: case/in
#   - numbered block parameters: _1, _2, etc.
#   - beginless ranges: (..3)
#   - **nil in method definitions (no-keywords marker)
#   - argument forwarding: def f(...); g(...); end

# --- pattern matching (case/in) ---

# literal patterns
case 1
in Integer
  :ok
end

case [1, 2, 3]
in [Integer, Integer, Integer]
  :ok
end

# variable binding
case {name: "ruby", version: 3}
in {name: String => name}
  name
end

# array pattern with rest
case [1, 2, 3, 4]
in [first, *rest]
  [first, rest]
end

# guard clause
case 42
in x if x > 10
  :big
in x
  :small
end

# pin operator
expected = 42
case 42
in ^expected
  :match
end

# nested patterns
case {users: [{name: "alice"}, {name: "bob"}]}
in {users: [{name: String => first}, *]}
  first
end

# or-pattern
case :foo
in :foo | :bar
  :ok
end

# --- numbered block parameters ---
[1, 2, 3].map { _1 * 2 }
[[1, 2], [3, 4]].map { _1 + _2 }
{a: 1}.map { [_1, _2] }

# --- beginless ranges ---
a = (..5)
b = (...5)

# beginless range in case/when
case -10
when (..0)
  :negative
when (1..)
  :positive
end

# beginless range in array slice
arr = [1, 2, 3, 4, 5]
_ = arr[..2]

# --- **nil in method definition ---
def no_keywords(**nil)
  :ok
end

def positional_only(a, b, **nil)
  [a, b]
end

# --- argument forwarding ---
def forwarding(...)
  other(...)
end

def other(*args)
  args
end
