# Kernel#tap, Kernel#then / Kernel#yield_self all take a block and pass self.
result = 42.tap { |n| puts "saw #{n}" }
puts result

squared = 7.then { |n| n * n }
puts squared

doubled = "hi".yield_self { |s| s + s }
puts doubled
