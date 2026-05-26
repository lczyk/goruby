# String#scan with a block yields each match and returns self. Rake's
# pathmap relies on this shape to walk the spec format string.

out = []
"a1b22c333".scan(/\d+/) do |m|
  out << m
end
p out                                      #=> ["1", "22", "333"]

groups = []
"a1b22".scan(/([a-z])(\d+)/) do |pair|
  groups << pair
end
p groups                                   #=> [["a", "1"], ["b", "22"]]

# Block form returns self.
ret = "abc".scan(/./) { |_| }
p ret                                      #=> "abc"

# Non-block scan unchanged.
p "a1b22".scan(/\d+/)                      #=> ["1", "22"]

# Pathmap exercise via rake's String ext -- the prod consumer.
$LOAD_PATH.unshift __dir__ + "/../../gems/rake/lib"
require "rake/ext/string"
puts "this/is/a/dir/abc.rb".pathmap("%d")  #=> this/is/a/dir
puts "this/is/a/dir/abc.rb".pathmap("%f")  #=> abc.rb
puts "this/is/a/dir/abc.rb".pathmap("%n")  #=> abc
puts "abc.rb".pathmap("%x")                #=> .rb
