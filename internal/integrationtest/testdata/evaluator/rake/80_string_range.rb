# minversion: 2.6
# Pin String-bound Range cover? / include?. Previously errored
# with "Range#cover? needs Integer bounds"; now lex-compare path
# kicks in.

# Inclusive range.
r = "a".."e"
puts r.cover?("c")                        #=> true
puts r.cover?("e")                        #=> true
puts r.cover?("f")                        #=> false
puts r.cover?("A")                        #=> false

# Exclusive range.
e = "a"..."e"
puts e.cover?("d")                        #=> true
puts e.cover?("e")                        #=> false

# include? is the same dispatch.
puts r.include?("d")                      #=> true
puts r.include?("z")                      #=> false

# Iteration via succ.
puts r.to_a.inspect                       #=> ["a", "b", "c", "d", "e"]
puts ("aa".."ad").to_a.inspect            #=> ["aa", "ab", "ac", "ad"]

# Non-string subjects on a string range -> false.
puts r.cover?(3)                          #=> false
puts r.cover?(:b)                         #=> false
