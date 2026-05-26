# minversion: 2.6
# Exercise operators on String / Array as dispatchable methods.
# Mirrors the Integer / Float work from 43_minitest_more_asserts.
# Lets minitest's assert_operator + reflective callers route the
# operator through __send__.

# String operators.
puts "a".send(:+, "b")                         #=> ab
puts "ab".send(:*, 3)                          #=> ababab
puts "a".send(:<, "b")                         #=> true
puts "abc".send(:==, "abc")                    #=> true
puts "abc".send(:==, "abd")                    #=> false
puts "abc".__send__(:<=>, "abd")               #=> -1

s = "hello"
s.send(:<<, " world")
puts s                                         #=> hello world

# Array operators.
puts [1, 2].send(:+, [3, 4]).inspect           #=> [1, 2, 3, 4]
puts [1, 2, 3].send(:-, [2]).inspect           #=> [1, 3]
puts [1, 2].send(:==, [1, 2])                  #=> true
puts [1, 2].send(:==, [1, 3])                  #=> false

arr = [1, 2]
arr.send(:<<, 3)
puts arr.inspect                               #=> [1, 2, 3]

# Method object dispatch -- m.call(arg) drives the same path.
m = 5.method(:+)
puts m.call(3)                                 #=> 8
