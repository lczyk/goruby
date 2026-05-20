# minversion: 2.0
# required-keyword args syntax `def f(x:)`. introduced 2.0.
def greet(name:, greeting: "hi")
  "#{greeting}, #{name}"
end

puts greet(name: "ada")                       #=> hi, ada
puts greet(name: "x", greeting: "yo")         #=> yo, x
