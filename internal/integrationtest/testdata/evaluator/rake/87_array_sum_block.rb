# minversion: 2.6
# Pin Array#sum with block -- each element mapped through the
# block before accumulation. Previously the block was ignored
# silently.

puts [1, 2, 3].sum { |x| x * 10 }              #=> 60
puts (1..5).to_a.sum { |x| x ** 2 }            #=> 55

# Block returning Float upgrades the accumulator.
puts [1, 2, 3].sum { |x| x * 1.5 }             #=> 9.0

# Block returning String concats (with init "").
puts ["a", "b", "c"].sum("") { |s| s.upcase }  #=> ABC

# No-block path unchanged.
puts [1, 2, 3].sum                              #=> 6
puts [1.0, 2.0, 3.0].sum                        #=> 6.0
puts [].sum                                     #=> 0
