# Array#member? aliases include?. Goruby previously returned
# NoMethodError. Rake's application building-imports path probes
# via Array#member? on a small dispatch set.

p [1, 2, 3].member?(2)                      #=> true
p [1, 2, 3].member?(4)                      #=> false
p [].member?(:x)                            #=> false
p ["a", "b"].include?("a") == ["a", "b"].member?("a") #=> true
