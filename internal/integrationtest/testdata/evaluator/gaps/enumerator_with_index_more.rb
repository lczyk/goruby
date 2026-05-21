puts [10, 20, 30].each.with_index.to_a.inspect
puts [10, 20, 30].map.with_index { |v, i| [i, v] }.inspect
sum = [10, 20, 30].each.with_index.reduce(0) { |acc, (v, i)| acc + v * i }
puts sum
