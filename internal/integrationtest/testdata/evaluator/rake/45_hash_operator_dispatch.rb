# minversion: 2.6
# Exercise Hash operator-style methods through __send__ / send /
# method().call(). Mirrors 44's String / Array work for the Hash
# corner: [], []=, == as dispatchable methods rather than infix-only.

h = { a: 1, b: 2 }

puts h.send(:[], :a)                            #=> 1
puts h.send(:[], :missing).nil?                 #=> true

h.send(:[]=, :c, 3)
puts h[:c]                                      #=> 3

# Reassigning an existing key.
h.send(:[]=, :a, 99)
puts h[:a]                                      #=> 99

# Structural equality.
puts h.send(:==, { a: 99, b: 2, c: 3 })         #=> true
puts h.send(:==, { a: 1, b: 2, c: 3 })          #=> false

# method-object dispatch.
getter = h.method(:[])
puts getter.call(:b)                            #=> 2
