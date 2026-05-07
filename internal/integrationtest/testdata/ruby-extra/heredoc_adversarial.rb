# Heredoc adversarial examples -- stress-test the lexer's heredoc handling.

# Basic heredoc
x = <<EOS
hello world
EOS

# Quoted delimiters -- single-quoted (literal, no interp), double-quoted (interp), backtick (cmd)
x = <<'LITERAL'
no #{interpolation} here
LITERAL

x = <<"INTERP"
hello #{name} world
INTERP

x = <<`CMD`
ls #{dir}
CMD

# Squiggy heredoc -- strips common leading whitespace
x = <<~SQUIG
    indented content
      with uneven indentation
  minimal indent stripped
SQUIG

# Indented heredoc with <<-
def foo
  x = <<-INDENT
    body content
  INDENT
end

# Chained heredocs on one line
x = method(<<A, <<B)
body A
A
body B
B

# Heredoc with trailing method call
x = <<EOS.chomp
  trailing whitespace
EOS

# Chained heredocs with trailing method
x = method(<<A.chomp, <<B.strip)
body A
A
body B
B

# Empty heredoc body
x = <<EMPTY
EMPTY

# Heredoc with only blank lines
x = <<BLANKS



BLANKS

# Heredoc body containing the delimiter as partial match (not at line start)
x = <<DELIM
  this is not DELIM
  nor is DELIM here
DELIM

# Squiggy heredoc with interpolation and uneven indentation
x = <<~SQUIGGY
    hello #{name}
      deeply #{nested} indented
  minimal
SQUIGGY

# Squiggy heredoc, literal (single-quoted)
x = <<~'SQUIGLIT'
    no #{interp}
      whitespace stripped anyway
SQUIGLIT

# Squiggy heredoc with backtick (command execution + indent strip)
x = <<~`SQCMD`
    ls #{path}
      echo done
SQCMD

# Heredoc inside method call, no parens
foo <<EOS, bar
content
EOS

# Multiple heredocs on separate lines in same expression
x = [<<A, <<B]
a
A
b
B

# Heredoc delimiter with underscores and digits
x = <<EOS_123
valid delim
EOS_123

# Heredoc after CLASS is LSHIFT, not heredoc
class Foo << Bar
  def method; end
end

# Squiggy heredoc with blank lines (blanks don't affect indent calculation)
x = <<~BLANK_TEST

    indented

    after blank
BLANK_TEST

# Heredoc with backslash-newline in body (literal heredoc should not process escapes)
x = <<'LITERAL'
line one \
still line one
LITERAL

# Backtick heredoc, chained with string heredoc
x = [<<`CMD`, <<STR]
ls #{path}
CMD
hello
STR

# Squiggy heredoc where first non-blank line has zero indent
x = <<~NOINDENT
no indent at all
  indented
NOINDENT
