# minversion: 2.6
# Pin Integer.sqrt -- integer square root.

puts Integer.sqrt(0)                          #=> 0
puts Integer.sqrt(1)                          #=> 1
puts Integer.sqrt(4)                          #=> 2
puts Integer.sqrt(9)                          #=> 3
puts Integer.sqrt(15)                         #=> 3
puts Integer.sqrt(16)                         #=> 4
puts Integer.sqrt(17)                         #=> 4
puts Integer.sqrt(100)                        #=> 10
puts Integer.sqrt(1000000)                    #=> 1000

# Negative raises ArgumentError (MRI raises Math::DomainError; we
# don't model that subclass yet).
begin
  Integer.sqrt(-1)
rescue ArgumentError
  puts "raised"                               #=> raised
end
