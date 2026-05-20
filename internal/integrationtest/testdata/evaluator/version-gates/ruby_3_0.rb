# minversion: 3.0
# `Hash#except`. introduced 3.0.
p({ a: 1, b: 2, c: 3 }.except(:b))    #=> {:a=>1, :c=>3}
