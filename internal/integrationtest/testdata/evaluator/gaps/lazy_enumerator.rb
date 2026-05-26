# Enumerator::Lazy supports chained map / select / reject / take /
# take_while / drop / drop_while plus filter_map / flat_map. Forcers
# first / to_a / force / each pull the pipeline.

# Basic chain on a finite Array.
p [1,2,3,4,5].lazy.map { |x| x * 2 }.to_a   #=> [2, 4, 6, 8, 10]
p [1,2,3,4,5].lazy.select { |x| x.odd? }.to_a #=> [1, 3, 5]
p [1,2,3,4,5].lazy.reject { |x| x.odd? }.to_a #=> [2, 4]

# first(n) on an infinite Range -- the canonical lazy use case.
p (1..Float::INFINITY).lazy.map { |x| x * x }.first(4) #=> [1, 4, 9, 16]

# take / drop as chain ops, not forcers (Lazy#take returns Lazy).
p (1..Float::INFINITY).lazy.take(5).to_a    #=> [1, 2, 3, 4, 5]
p [1,2,3,4,5,6].lazy.drop(2).to_a           #=> [3, 4, 5, 6]

# take_while / drop_while.
p [1,2,3,4,5].lazy.take_while { |x| x < 4 }.to_a #=> [1, 2, 3]
p [1,2,3,4,5].lazy.drop_while { |x| x < 4 }.to_a #=> [4, 5]

# flat_map fans values out.
p [[1,2],[3,4]].lazy.flat_map { |x| x }.to_a #=> [1, 2, 3, 4]

# Chain composition + early termination on infinite source.
p (1..Float::INFINITY).lazy.select { |x| x % 3 == 0 }.first(3) #=> [3, 6, 9]
