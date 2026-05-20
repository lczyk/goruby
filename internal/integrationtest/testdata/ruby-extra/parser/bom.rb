# BOM-prefixed source. A leading UTF-8 byte-order mark (0xEF 0xBB 0xBF)
# must be transparently stripped by the parser before tokenization. MRI
# strips it on every supported version (1.9 through 4.0).

puts "hello bom"

# Constants, methods, classes -- nothing else is special; only the leading
# 3-byte BOM is the test surface. The rest is normal Ruby exercised to make
# sure the post-BOM stream lexes / parses identically to a BOM-less file.

CONST_A = 42
CONST_B = "string with #{CONST_A} interp"

def greet(name = "world")
  "hi, #{name}"
end

class Box
  attr_accessor :x, :y
  def initialize(x, y)
    @x = x
    @y = y
  end

  def to_s
    "Box(#{@x}, #{@y})"
  end
end

b = Box.new(1, 2)
puts b
puts greet
puts greet("ruby")

# Method with block, hash literal, symbol, range -- a small grab-bag of
# constructs that all need clean tokenization after BOM strip.
[1, 2, 3].each { |n| puts n * 2 }
h = { a: 1, b: 2, c: 3 }
h.each_pair do |k, v|
  puts ":#{k} => #{v}"
end
puts (1..5).to_a.inspect
