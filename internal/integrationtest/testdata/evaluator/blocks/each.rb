[1, 2, 3].each { |x| puts x }   #=> 1
                                #=> 2
                                #=> 3

sum = 0
[10, 20, 30].each do |n|
  sum = sum + n
end
puts sum                        #=> 60

doubled = [1, 2, 3].map { |x| x * 2 }
p doubled                       #=> [2, 4, 6]
