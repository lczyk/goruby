h = { "a" => 1, "b" => 2 }
puts h["a"]               #=> 1
puts h["b"]               #=> 2
puts h["missing"]         #=>

h["c"] = 3
puts h["c"]               #=> 3
puts h.size               #=> 3

h2 = { a: 1, b: 2 }
puts h2[:a]               #=> 1
puts h2.keys.length       #=> 2
puts h2.has_key?(:a)      #=> true
puts h2.has_key?(:z)      #=> false
