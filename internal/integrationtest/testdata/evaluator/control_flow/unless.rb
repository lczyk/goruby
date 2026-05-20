x = 10

unless x > 100
  puts "not huge"   #=> not huge
end

unless x == 10
  puts "never"
else
  puts "is ten"     #=> is ten
end

puts "guard" unless x.nil?   #=> guard
