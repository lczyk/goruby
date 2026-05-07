# Regex adversarial examples -- stress-test regex lexing in various contexts.

# Basic regex
/foo/

# Regex with escaped slash -- must not end the regex
/foo\/bar/

# Regex with character class containing slash
/[/]/

# Regex with character class containing escaped bracket
/[\[\]]/

# Regex with options
/foo/imx
/bar/ix
/baz/m

# Regex after various contexts (tests isRegexBeginContext)
x = /foo/
if /bar/ =~ str
foo(/baz/)
[/qux/]

# Regex with interpolation
/hello #{name} world/

# Regex with nested interpolation
/pattern #{ "sub#{x}" } more/

# Regex with escaped characters
/[\t\n\r\f\v]/

# Regex with unicode escapes
/\u{2603}/

# Regex with quantifiers
/a+b*c?d{3}e{1,5}f*/

# Regex with groups and alternation
/(foo|bar)baz/

# Regex with anchors
/^start.*end$/

# Regex with lookahead/lookbehind
/(?=foo)/
/(?!bar)/
/(?<=baz)/

# Regex as method argument
gsub(/pattern/, "replacement")

# Regex in case expression
case x
when /foo/
  true
end

# Division vs regex ambiguity -- these should all be division
a = b / c
a = b / c / d
foo(1 / 2)

# Percent regex literal with different delimiters
%r{/path/to/somewhere}
%r<xml>like</xml>>
%r|pipe/delimited|
%r(pa/rent/hetical)

# Percent regex with interpolation
%r{pattern #{name} suffix}

# Percent regex with options
%r{foo}imx

# Regex with newlines (multiline)
/
  pattern
  across
  lines
/x

# Empty regex
//

# Regex containing #
/# not a comment/
