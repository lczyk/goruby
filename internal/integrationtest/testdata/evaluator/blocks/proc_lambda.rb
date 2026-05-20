add = ->(a, b) { a + b }
puts add.call(2, 3)       #=> 5
puts add.(4, 5)           #=> 9

square = Proc.new { |x| x * x }
puts square.call(6)       #=> 36

def run(&blk)
  blk.call(7)
end

puts run { |n| n + 1 }    #=> 8
