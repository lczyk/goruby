# Recursive fib -- pure call-dispatch + integer-infix workload.
def fib(n)
  return n if n < 2
  fib(n - 1) + fib(n - 2)
end

5.times { fib(25) }
