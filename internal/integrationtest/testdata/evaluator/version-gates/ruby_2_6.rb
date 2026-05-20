# minversion: 2.6
# `Object#then` (alias for `yield_self`). introduced 2.6.
puts 3.then { |n| n * 10 }    #=> 30
