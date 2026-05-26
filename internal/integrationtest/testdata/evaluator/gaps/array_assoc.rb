# Array#assoc / rassoc: find a nested 2-element array by its first /
# second element. Returns the matching sub-array, or nil.
pairs = [[:a, 1], [:b, 2], [:c, 3]]
puts pairs.assoc(:b).inspect              #=> [:b, 2]
puts pairs.assoc(:z).inspect              #=> nil
puts pairs.rassoc(2).inspect              #=> [:b, 2]
puts pairs.rassoc(99).inspect             #=> nil
