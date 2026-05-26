# minversion: 2.6
# Pin Kernel#catch / #throw -- non-local exit by symbol tag.

# Throw / catch with matching tag returns the thrown value.
result = catch(:done) do
  10.times do |i|
    throw :done, "found #{i}" if i == 3
  end
  "nothing"
end
puts result                                   #=> found 3

# Loop terminates via throw without iterating further.
counter = 0
catch(:stop) do
  100.times do
    counter += 1
    throw :stop if counter == 5
  end
end
puts counter                                  #=> 5

# Throw with no value returns nil.
got = catch(:tag) do
  throw :tag
  "unreachable"
end
puts got.inspect                              #=> nil

# Block completing without throw returns block's value.
got2 = catch(:tag) do
  "no throw"
end
puts got2                                     #=> no throw

# Nested catch + throw matches outermost.
trace = []
result = catch(:outer) do
  trace << "outer-start"
  catch(:inner) do
    trace << "inner-start"
    throw :outer, "out-from-inner"
    trace << "inner-end"
  end
  trace << "outer-end"
end
puts result                                   #=> out-from-inner
puts trace.inspect                            #=> ["outer-start", "inner-start"]
