a = [3, 1, 2]
p a.sort           #=> [1, 2, 3]
p a.reverse        #=> [2, 1, 3]
puts a.length      #=> 3
puts a.first       #=> 3
puts a.last        #=> 2
puts a.include?(2) #=> true
puts a.include?(9) #=> false

p [1, 2, 3].map { |x| x * x }      #=> [1, 4, 9]
p [1, 2, 3, 4].select { |x| x.even? } #=> [2, 4]
puts [1, 2, 3].reduce(0) { |s, x| s + x }  #=> 6
