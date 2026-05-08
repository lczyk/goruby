# syntax introduced in ruby 2.3 (fails 2.2, passes 2.3+)
#
# boundary: 2.2 -> 2.3
# note: 2.2 had no parser-relevant syntax changes, so this spans 2.1 -> 2.3
#
# changes exercised:
#   - safe navigation operator: &.
#   - frozen_string_literal pragma
#   - <<~ squiggly heredoc (indentation-stripping)
# frozen_string_literal: true

# safe navigation operator
a = nil
b = a&.to_s
c = a&.length
d = "hello"&.upcase&.reverse

# safe navigation with arguments
e = [1, 2, 3]
f = e&.fetch(0)

# safe navigation with block
g = e&.map { |x| x * 2 }

# safe navigation chained
h = a&.to_s&.upcase&.reverse&.strip

# safe navigation with assignment
obj = nil
obj&.foo = 1 rescue nil

# squiggly heredoc (strips leading indentation)
text = <<~HEREDOC
  hello
    indented
  world
HEREDOC

text2 = <<~'SINGLE'
  no #{interpolation} here
SINGLE

text3 = <<~`CMD`
  echo hello
CMD
