a = [1, [2, [3, [4, 5]]]]
puts a.flatten.inspect
puts a.flatten(1).inspect
puts a.flatten(2).inspect

b = [1, [2, [3]]]
b.flatten!
puts b.inspect
