# Heredoc as last expression in an if-then-else branch.
# Heredoc tag appears mid-statement; body must coexist with the closing `end`.
x = if true
  <<-EOS
  hello
  EOS
end
