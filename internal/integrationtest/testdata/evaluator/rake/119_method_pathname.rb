# Method / Proc constants and a minimal Pathname class.

# Method / Proc are reachable as top-level Class constants. Rake's
# rule.rb branches on `Method === arg`.
p Method.is_a?(Class)                       #=> true
p Proc.is_a?(Class)                         #=> true
m = "x".method(:upcase)
p Method === m                              #=> true
p m.call                                    #=> "X"

# Pathname.new(s) -> instance with to_s / to_path / to_str / ==.
require "pathname"
pn = Pathname.new("a/b/c")
p pn.to_s                                   #=> "a/b/c"
p pn.to_path                                #=> "a/b/c"
p pn.to_str                                 #=> "a/b/c"
p pn == "a/b/c"                             #=> true
puts pn.inspect                             #=> #<Pathname:a/b/c>
