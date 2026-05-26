# Numeric#lcm and nonzero?.
puts 4.lcm(6)                             #=> 12
puts 12.lcm(8)                            #=> 24

puts 0.nonzero?.inspect                   #=> nil
puts 5.nonzero?                           #=> 5
puts (-3).nonzero?                        #=> -3
