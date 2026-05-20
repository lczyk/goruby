x = 10

if x > 5
  puts "big"        #=> big
end

if x < 5
  puts "never"
else
  puts "small no"   #=> small no
end

if x < 0
  puts "neg"
elsif x == 0
  puts "zero"
elsif x < 100
  puts "two digits" #=> two digits
else
  puts "huge"
end

puts(x > 0 ? "pos" : "neg")   #=> pos

puts "guarded" if x == 10     #=> guarded
puts "never" if x == 0
