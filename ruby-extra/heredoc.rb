# heredoc edge cases

# squiggy heredoc with method chain
result = <<~TEXT.strip.upcase
  hello world
TEXT

# multiple heredocs as method args
foo(<<~A, <<~B)
  body a
A
  body b
B

# heredoc with interpolation
x = 42
s = <<~MSG
  the answer is #{x}
MSG

# backtick heredoc (command)
output = <<~`CMD`
  ls -la #{Dir.home}
CMD

# single-quoted heredoc (no interpolation)
s2 = <<~'RAW'
  literal #{not_interpolated}
RAW

# double-quoted heredoc (explicit)
s3 = <<~"DQ"
  interpolated #{x}
DQ

# indented heredoc (<<-)
s4 = <<-INDENTED
	content here
INDENTED

# heredoc with empty body
empty = <<~EMPTY
EMPTY

# heredoc as part of an expression chain
result = (<<~EXPR).lines.count
  line1
  line2
EXPR

# nested heredoc (heredoc inside string interpolation)
inner = "value"
outer = "the inner is #{<<~INNER.chomp}"
  nested #{inner}
INNER
