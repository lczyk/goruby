def with_default(a, b = 10)
  a + b
end

puts with_default(1)        #=> 11
puts with_default(1, 2)     #=> 3

def with_splat(first, *rest)
  puts first                #=> 1
  p rest                    #=> [2, 3, 4]
end

with_splat(1, 2, 3, 4)

def with_kwargs(name:, age: 0)
  puts "#{name}/#{age}"
end

with_kwargs(name: "x")          #=> x/0
with_kwargs(name: "y", age: 9)  #=> y/9
