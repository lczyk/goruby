# minversion: 2.6
# Pin String#squeeze, String#prepend, String#delete.

# squeeze -- collapse consecutive duplicate chars.
puts "hello".squeeze                          #=> helo
puts "hello".squeeze("l")                     #=> helo
puts "mississippi".squeeze("ips")             #=> misisipi
puts "  abc  ".squeeze(" ").length            #=> 5

# prepend -- mutating push-on-front. Returns receiver.
s = "world"
r = s.prepend("hello ")
puts s                                        #=> hello world
puts r.equal?(s)                              #=> true

# delete -- remove all chars in the set. Non-mutating.
puts "abcd".delete("bd")                      #=> ac
puts "Hello World".delete("aeiou")            #=> Hll Wrld

# Combinations.
s2 = "aabbcc"
puts s2.squeeze                               #=> abc
puts s2.delete("b")                           #=> aacc
puts s2.delete("b").squeeze                   #=> ac
