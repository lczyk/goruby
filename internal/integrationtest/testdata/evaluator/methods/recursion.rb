def fact(n)
  if n <= 1
    1
  else
    n * fact(n - 1)
  end
end

puts fact(0)    #=> 1
puts fact(1)    #=> 1
puts fact(5)    #=> 120
puts fact(10)   #=> 3628800

def fib(n)
  return n if n < 2
  fib(n - 1) + fib(n - 2)
end

puts fib(10)    #=> 55
