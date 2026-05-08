# syntax introduced in ruby 2.1 (fails 2.0, passes 2.1+)
#
# boundary: 2.0 -> 2.1
#
# changes exercised:
#   - required keyword arguments: def f(a:)
#   - rational literals: 1r, 2/3r
#   - complex literals: 1i, 2+3i
#   - rational + complex combined: 1ri
#   - def returns symbol (semantic, but requires parser awareness for chaining)

# required keyword arguments (2.0 only had optional kwargs)
def required_kw(a:, b:)
  [a, b]
end

def mixed_kw(x, y, a:, b: nil)
  [x, y, a, b]
end

def kw_splat(a:, **rest)
  [a, rest]
end

# rational literals
a = 1r
b = 42r
c = 3.14r

# complex literals
d = 1i
e = 42i
f = 3.14i

# combined rational + complex
g = 1ri

# rational in expressions
h = 1r + 2r
i = 1r * 3

# complex in expressions
j = 1i + 2i
k = 1 + 2i

# def returns symbol (enables chaining with e.g. private)
class Foo
  private def secret
    42
  end

  protected def semi_secret
    99
  end
end
