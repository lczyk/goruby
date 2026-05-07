# block edge cases: block-local variables, nesting, defaults

# block-local variables (shadowing -- vars after ;)
x = 10
[1,2,3].each do |i; x|
  x = i * 2  # does not affect outer x
end

# multiple block-local variables
[1,2,3].each do |i; a, b, c|
  a = i
  b = i
  c = i
end

# block with defaults AND block-locals
[1,2,3].each do |i = 1; x|
  x = i
end

# zero-arg block with only block-locals (semicolon, no params)
[1,2,3].each do |; x|
  x = 1
end

# brace block with block-locals
[1,2,3].each { |i; x| x = i * 2 }

# block capture + regular params
def captures(&blk)
  blk.call
end
captures { puts "hi" }

# block with complex params
foo do |a, *b, c:, d: 1, &blk|
  a + c + d
end
