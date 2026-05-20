# Array#map + Array#sum block dispatch. Allocates a fresh result Array
# each call, exercising Array literal / Array#map.
15000.times do
  xs = (1..50).to_a
  ys = xs.map { |x| x * x + 1 }
  ys.sum
end
