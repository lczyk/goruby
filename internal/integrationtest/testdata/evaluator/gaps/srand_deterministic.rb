srand(42)
a = [rand(1000), rand(1000), rand(1000)]
srand(42)
b = [rand(1000), rand(1000), rand(1000)]
# Value sequence differs from MRI (different PRNG algo), but srand
# must yield the same sequence on identical seeds.
puts a == b
puts a.size == 3
puts a.all? { |x| x.is_a?(Integer) && x >= 0 && x < 1000 }
