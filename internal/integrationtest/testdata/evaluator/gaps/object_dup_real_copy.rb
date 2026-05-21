a = [1, 2, 3]
b = a.dup
b << 4
puts a.inspect
puts b.inspect
puts a.equal?(b)

s = "hello"
t = s.dup
t << " world"
puts s
puts t
puts s.equal?(t)
