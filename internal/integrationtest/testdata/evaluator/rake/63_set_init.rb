# minversion: 2.6
# Pin Set.new(enumerable) -- previously the enumerable arg was
# silently discarded; Set.new([1,2,3]) produced an empty set.

require "set"

s = Set.new([1, 2, 3])
puts s.size                                   #=> 3
puts s.include?(2)                            #=> true
puts s.to_a.sort.inspect                      #=> [1, 2, 3]

# Dedup via rubyEqualDispatch.
s2 = Set.new([1, 1, 2, 2, 3])
puts s2.size                                  #=> 3

# Empty init.
s3 = Set.new
puts s3.empty?                                #=> true

# Add after init.
s.add(99)
puts s.size                                   #=> 4
puts s.include?(99)                           #=> true

# Adding a dup is a noop.
s.add(2)
puts s.size                                   #=> 4
