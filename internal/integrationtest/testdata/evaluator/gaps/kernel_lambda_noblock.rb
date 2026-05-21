def wrap(&b)
  lambda(&b)
end
f = wrap { |x| x * 2 }
puts f.call(21)
puts f.lambda?
