def implicit
  1 + 2
end

def explicit
  return 99
  100  # unreachable
end

def early(x)
  return "neg" if x < 0
  "non-neg"
end

puts implicit       #=> 3
puts explicit       #=> 99
puts early(-1)      #=> neg
puts early(5)       #=> non-neg
