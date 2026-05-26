# Range.new(begin, end[, exclusive]) returns a proper Range
# (was: Class.new fell through and returned a bare Instance,
# which crashed downstream method calls on the recv.(*Range) cast).

r = Range.new(1, 5)
p r.to_a                                    #=> [1, 2, 3, 4, 5]
p r.include?(3)                             #=> true
p r.size                                    #=> 5

# Exclusive end (default false).
ex = Range.new(1, 5, true)
p ex.to_a                                   #=> [1, 2, 3, 4]
p ex.last                                   #=> 5
