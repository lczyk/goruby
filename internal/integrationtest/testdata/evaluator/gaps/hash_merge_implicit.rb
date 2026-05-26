# Hash#merge with an implicit-hash arg (`h.merge(c: 3)`) -- the
# trailing `key: val` shape is sugar for `h.merge({c: 3})`. The
# dispatcher must bundle pending kwargs into a Hash arg when the
# target builtin doesn't consume kwargs of its own.
puts({a:1,b:2}.merge(c:3).inspect)               #=> {:a=>1, :b=>2, :c=>3}
puts({a:1,b:2}.merge(b:10) { |k,o,n| o+n }.inspect) #=> {:a=>1, :b=>12}
puts({a:1}.merge(b:2, c:3).inspect)              #=> {:a=>1, :b=>2, :c=>3}

# Same pattern on Array (push w/ trailing label form should still
# treat the kwarg-shaped arg as a hash element, MRI 2.6 behaviour).
a = []
a.push({k: 1})
puts a.inspect                                    #=> [{:k=>1}]
