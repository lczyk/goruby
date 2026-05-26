# Hash#has_value? -- predicate for value membership. Counterpart of
# has_key?. value? is the shorter alias.
h = {a: 1, b: 2, c: 3}
puts h.has_value?(2)                      #=> true
puts h.has_value?(99)                     #=> false
puts h.value?(1)                          #=> true
puts h.value?(99)                         #=> false
