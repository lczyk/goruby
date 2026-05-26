# minversion: 2.6
# Pin boolean & / | / ^ infix operators. MRI semantics:
# nil and false are false; everything else is truthy.

# true / false combinations.
puts true & true                        #=> true
puts true & false                       #=> false
puts true | false                       #=> true
puts true | true                        #=> true
puts true ^ false                       #=> true
puts true ^ true                        #=> false

# nil truthy ops.
puts nil & true                         #=> false
puts nil | true                         #=> true
puts nil ^ true                         #=> true

# Truthy-conversion for non-bool right operand.
puts true & 1                           #=> true
puts true & nil                         #=> false
puts false | "x"                        #=> true
puts false | nil                        #=> false

# Real-world: flag combination.
flag_a = true
flag_b = false
puts flag_a & flag_b                    #=> false
puts flag_a | flag_b                    #=> true
puts flag_a ^ flag_b                    #=> true
