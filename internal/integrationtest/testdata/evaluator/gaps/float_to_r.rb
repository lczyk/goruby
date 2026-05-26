# Float#to_r decomposes via IEEE 754 mantissa/exponent into a
# reduced Rational. Exact for binary-clean values like 0.5, 0.25.
# For 0.1 and friends, returns the exact Rational the float
# represents -- which differs from the literal you wrote (and from
# MRI 3.4+; 2.6 produces the same long quotient we do).
puts 0.5.to_r.inspect                     #=> (1/2)
puts 0.25.to_r.inspect                    #=> (1/4)
puts 0.75.to_r.inspect                    #=> (3/4)
puts 1.5.to_r.inspect                     #=> (3/2)
puts (-0.5).to_r.inspect                  #=> (-1/2)
puts 0.0.to_r.inspect                     #=> (0/1)
puts 2.0.to_r.inspect                     #=> (2/1)
