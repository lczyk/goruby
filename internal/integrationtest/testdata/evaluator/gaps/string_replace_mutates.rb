# String#replace must overwrite the receiver in place, not allocate
# a new String. Returns self.
s = "hello"
ret = s.replace("world")
puts s                                    #=> world
puts ret.equal?(s)                        #=> true

# Aliases still point at the same buffer after replace.
a = "abc"
b = a
a.replace("xyz")
puts b                                    #=> xyz
