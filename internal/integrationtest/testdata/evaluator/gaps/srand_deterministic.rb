srand(42)
a = [rand(1000), rand(1000), rand(1000)]
srand(42)
b = [rand(1000), rand(1000), rand(1000)]
puts a.inspect
puts b.inspect
puts a == b
