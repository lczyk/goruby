# minversion: 2.6
# Pin Array#fetch -- positional index with optional default,
# raises IndexError when out of bounds without default.

arr = [10, 20, 30]

puts arr.fetch(0)                              #=> 10
puts arr.fetch(2)                              #=> 30
puts arr.fetch(-1)                             #=> 30
puts arr.fetch(-3)                             #=> 10

# Default arg returned when out of bounds.
puts arr.fetch(99, "missing")                  #=> missing
puts arr.fetch(-99, 0)                         #=> 0

# Without default, out of bounds raises IndexError.
begin
  arr.fetch(99)
rescue IndexError
  puts "raised"                                #=> raised
end
