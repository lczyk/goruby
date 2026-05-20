# minversion: 2.3
# safe navigation operator `&.`. introduced 2.3.
s = nil
puts s&.length         #=>
puts "abc"&.length     #=> 3
