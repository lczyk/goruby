# Object#hash defaults to object_id. Without it, using arbitrary
# user-class instances as hash keys raised NoMethodError.

class C; end
c1 = C.new
c2 = C.new

p c1.hash.is_a?(Integer)                    #=> true
p c1.hash == c1.hash                        #=> true
p c1.hash != c2.hash                        #=> true

# Using as Hash keys works (Hash#[]= uses hash, then ==).
h = {}
h[c1] = "one"
h[c2] = "two"
p h[c1]                                      #=> "one"
p h[c2]                                      #=> "two"
p h.size                                     #=> 2
