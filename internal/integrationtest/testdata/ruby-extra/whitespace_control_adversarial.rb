# Whitespace, newline, and control-flow adversarial examples.

# Semicolons as statement separators
a = 1; b = 2; c = 3

# Newlines as statement terminators
x = 1
y = 2
z = 3

# Backslash line continuation
x = 1 + \
    2 + \
    3

# Backslash line continuation in method arguments
foo(1, \
    2, \
    3)

# Multiple newlines between statements
a = 1


b = 2

# Mixed semicolons and newlines
a = 1; b = 2
c = 3; d = 4

# Newline inside parens -- no statement terminator
x = (1 +
     2 +
     3)

# Newline inside brackets
arr = [
  1,
  2,
  3,
]

# Newline inside braces (hash)
hash = {
  key: "value",
  other: 123,
}

# Newline inside block
foo do
  bar
  baz
end

# Inline block with braces
foo { |x| x + 1 }

# __END__ -- everything after this should be ignored by lexer
# (only when __END__ is at start of line)
# __END__
# this is not ruby code

# __END__ not at line start -- should NOT trigger
x = "__END__"

# Trailing whitespace on various lines
a = 1
b = 2
c = 3

# Mixed tabs and spaces (should lex same as spaces)
	indented_with_tab
  indented_with_spaces

# Backslash-newline followed by more backslash-newlines
x = 1 \
 \
 \
+ 2

# Newline after operator (line continuation implied)
x = 1 +
    2 *
    3

# Newline after comma
foo(1,
    2,
    3)

# Newline inside string literal (literal newline in source)
y = "hello
world"

# Newline inside regex literal
r = /hello
world/x

# Newline after dot (method chaining)
result = object
  .method1
  .method2
  .method3

# Comment at end of line
a = 1  # comment
b = 2 #another comment

# Comment only line
# This is a comment line

# Multiple comment lines
# line 1
# line 2
# line 3

# Comment with hash-like content (should not be interpolation)
# #{this is a comment, not interpolation}

# Hash literal vs comment ambiguity
h = { :key => val } # this is a hash
