# hash edge cases

# key: shorthand (symbol keys)
h1 = {a: 1, b: 2, c: 3}

# mixed notation
h2 = {:a => 1, b: 2, "c" => 3}

# hashrocket with various key types
h3 = {
  1 => "one",
  2.0 => "two",
  true => "yes",
  false => "no",
  nil => "nil key",
  :sym => "symbol",
  "str" => "string",
}

# hash as last argument without braces (implicit hash)
def foo(opts = {})
  opts
end
foo(a: 1, b: 2)
foo :a => 1, :b => 2

# empty hash
h4 = {}

# trailing comma in hash
h5 = {a: 1, b: 2,}

# hash with expression keys
h6 = {1 + 2 => 3, 4 * 5 => 20}

# keyword argument destructuring
def destructure(**kwargs)
  kwargs
end
destructure(a: 1, b: 2)

# method call with hash splat
def takes_hash(a:, b:)
  a + b
end
opts = {a: 1, b: 2}
takes_hash(**opts)
