# minversion: 2.6
# Range#step -- materialise the range, then take every Nth element.

# Step 1 (default-ish).
puts (1..5).step(1).to_a.inspect            #=> [1, 2, 3, 4, 5]

# Step 2.
puts (1..10).step(2).to_a.inspect           #=> [1, 3, 5, 7, 9]

# Step 3.
puts (1..10).step(3).to_a.inspect           #=> [1, 4, 7, 10]

# Exclusive range.
puts (1...5).step(2).to_a.inspect           #=> [1, 3]

# Step bigger than the range.
puts (1..3).step(10).to_a.inspect           #=> [1]
