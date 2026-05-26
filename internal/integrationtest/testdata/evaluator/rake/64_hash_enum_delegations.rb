# minversion: 2.6
# Pin Hash Enumerable-shaped delegations that materialise to
# [[k,v], ...] and dispatch on the resulting Array. Covers the
# methods that weren't already explicitly registered on Hash.

h = {a: 1, b: 2, c: 3}

# group_by -- bucket entries by block result.
g = h.group_by { |k, v| v.odd? ? :odd : :even }
puts g[true].nil?                              #=> true
# (avoid relying on Hash#== ordering -- check via include?)
puts g[:odd].include?([:a, 1])                 #=> true
puts g[:odd].include?([:c, 3])                 #=> true
puts g[:even].include?([:b, 2])                #=> true

# partition -- split into truthy / falsy.
left, right = h.partition { |k, v| v.even? }
puts left.length                               #=> 1
puts right.length                              #=> 2

# flat_map -- block returns an array per pair, all flattened.
puts h.flat_map { |k, v| [k, v] }.inspect      #=> [:a, 1, :b, 2, :c, 3]

# take_while / drop_while -- consume entries in iteration order
# until the block returns false / starts returning true.
puts h.take_while { |k, v| v < 3 }.length      #=> 2
puts h.drop_while { |k, v| v < 3 }.length      #=> 1
