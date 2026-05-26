# Block-less iterators must return an Enumerator (we model this as a
# wrapper that supports .to_a). The fixture only chains .to_a since
# that's what the corpus typically needs after a method-without-block.
p 1.upto(3).to_a                          #=> [1, 2, 3]
p 5.downto(3).to_a                        #=> [5, 4, 3]
p 3.times.to_a                            #=> [0, 1, 2]

# Hash#count without block returns the entry count.
puts({a:1, b:2, c:3}.count)               #=> 3

# Hash#sort_by without block returns an Enumerator over [key, val]
# pairs. .to_a -> the underlying pair array. Hash iterates in
# insertion order so the output preserves source order.
p({a:1, b:2, c:3}.sort_by.to_a)           #=> [[:a, 1], [:b, 2], [:c, 3]]
