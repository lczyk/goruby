# minversion: 2.7
# `Enumerable#tally` and numbered block parameter `_1`. both introduced 2.7.
p %w[a b a c b a].tally    #=> {"a"=>3, "b"=>2, "c"=>1}
p [1, 2, 3].map { _1 * 2 } #=> [2, 4, 6]
