# minversion: 2.6
# Pin Comparable derivations (clamp, between?) on builtin types
# (String, Integer, Float). Previously only user-defined classes
# whose receiver was an *Instance reached spaceshipCompare; the
# fix routes builtin receivers through callMethod to find <=>.

# Numeric clamp.
puts 5.clamp(0, 10)                         #=> 5
puts (-1).clamp(0, 10)                      #=> 0
puts 20.clamp(0, 10)                        #=> 10

# Float clamp.
puts 3.5.clamp(0.0, 5.0)                    #=> 3.5
puts (-1.0).clamp(0.0, 5.0)                 #=> 0.0
puts 10.0.clamp(0.0, 5.0)                   #=> 5.0

# String clamp -- lexical ordering.
puts "b".clamp("a", "z")                    #=> b
puts "A".clamp("c", "z")                    #=> c
puts "zz".clamp("a", "m")                   #=> m

# between?
puts 5.between?(0, 10)                      #=> true
puts 5.between?(6, 10)                      #=> false
puts "m".between?("a", "z")                 #=> true
