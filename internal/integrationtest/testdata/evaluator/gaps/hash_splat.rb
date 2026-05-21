h1 = {a: 1, b: 2}
h2 = {c: 3, **h1}
puts h2.inspect

def kw(a:, b:, c:)
  puts "#{a} #{b} #{c}"
end
kw(**{a: 1, b: 2, c: 3})
