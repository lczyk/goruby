# minversion: 2.6
# Exercise Rake::InvocationChain directly. Tests rake's
# LinkedList-derived invocation chain machinery -- conj, append,
# include?, member?.

$LOAD_PATH.unshift __dir__ + "/../../gems/rake/lib"

require "rake"

empty = Rake::InvocationChain.empty
puts empty.empty?               #=> true

one = empty.append("A")
puts one.empty?                 #=> false
puts one.member?("A")           #=> true
puts one.member?("B")           #=> false

two = one.append("B")
puts two.member?("A")           #=> true
puts two.member?("B")           #=> true
puts two.to_s                   #=> TOP => A => B
