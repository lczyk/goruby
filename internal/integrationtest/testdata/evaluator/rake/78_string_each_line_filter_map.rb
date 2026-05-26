# minversion: 2.6
# Pin String#each_line no-block + Array#filter_map. Both are
# common idioms that were missing.

# each_line without block returns an Array of lines (with trailing
# \n preserved on all but the last when input lacks one).
input = "alpha\nbeta\ngamma"
puts input.each_line.to_a.length              #=> 3
puts input.each_line.to_a[0]                  #=> alpha
puts input.each_line.to_a[2]                  #=> gamma

# Block form unchanged.
seen = []
input.each_line { |l| seen << l.chomp }
puts seen.join(",")                           #=> alpha,beta,gamma

# filter_map -- map + compact in one pass.
puts [1, 2, 3, 4, 5].filter_map { |x| x * 10 if x.even? }.inspect
                                              #=> [20, 40]
puts (1..10).to_a.filter_map { |x| "#{x}" if x % 3 == 0 }.inspect
                                              #=> ["3", "6", "9"]

# filter_map on empty.
puts [].filter_map { |x| x }.inspect          #=> []
