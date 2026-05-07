# Second-wave adversarial: error paths, short interp, escape variants, regex contexts.

# ---- unterminated constructs ----
# Each of these should produce an ILLEGAL token, not panic.
# UNCOMMENT to test error paths (they emit ILLEGAL which is correct):
# "unterminated string
# /unterminated regex
# %q(unterminated
# <<UNTERM
# body
# `unterminated cmd

# ---- \uXXXX 4-char hex (no braces) ----
"A"  # 'A'
"e"  # e-acute
"snowman"  # snowman

# ---- short interpolation forms in different string types ----

# In double-quoted strings:
"global: #$var"
"instance: #@ivar"
"class: #@@cvar"
"pid #$$, args #$*"

# In heredoc body (interpolating):
x = <<INTERP
hello #$USER
instance #@name
class #@@counter
INTERP

# In regex body:
/pattern #$x suffix/
/prefix #@x/

# In backtick:
`ls #$dir`
`echo #@name`

# In percent literals:
%Q{hello #$world}
%W[word #$x #@y]
%I[#@prefix_foo #$suffix_bar]
%x{ls #$path}

# ---- regex after every isRegexBeginContext token ----
# These should all lex as regex, not division:
x = /after_assign/
x =~ /after_match/; x !~ /after_nmatch/
if /after_if/ =~ x; end
unless /after_unless/; end
while /after_while/; end
x ? /after_qmark/ : nil
/case; /when; /return /do
/break
/next
/and; /or; /not; /super

# After parens, brackets, braces:
(/after_lparen/)
[/after_lbracket/]
{ /after_lbrace/ }

# After comma, semicolon, colon:
foo(/after_comma/, /also/)
bar; /after_semi/
baz: /after_colon/

# After pipe, logical:
x || /after_or/
x && /after_and/
| /after_pipe/

# After unary operators:
! /after_bang/
~ /after_tilde/

# After then, hashrocket:
if x then /after_then/; end
{ key: => /after_rocket/ }

# ---- # in regex (must NOT be comment) ----
/# this is a pattern, not a comment/
/pattern # still regex/

# ---- regex options edge cases ----
/foo/i
/foo/m
/foo/x
/foo/o
/foo/u
/foo/n
/foo/e
/foo/s
/foo/imx
/foo/ms
/foo/xn

# ---- regex with multiple flags after interp ----
/pattern #{x}/imx

# ---- backtick with escapes ----
`echo "hello\tworld"`
`ls \#{not_interp}`

# ---- % with whitespace context (method call) ----
# After identifier with whitespace, % starts a percent literal
foo %w[a b]
bar %q{string}
baz %r/regex/

# ---- edge: modulo vs percent literal ----
5 % 3       # modulo
a = b % c   # modulo
x = 5 % 3   # modulo

# ---- floating point edge cases ----
.5          # float
5e10        # float
1.5e-5      # float with negative exponent
1.5e+5      # float with positive exponent

# ---- line continuation edge cases ----
x = "hello \
world"
y = 'single \
quote'

# ---- empty constructs ----
""
''
//
` `  # space in backtick
%q()
%Q{}
%w[]
%i[]
%r()

# ---- heredoc with weird delimiter names ----
<<_OK
body
_OK

<<EOS123
body
EOS123

# ---- squiggy heredoc with tab indentation ----
x = <<~TABS
\t\tindented with tabs
\t  mixed tab and spaces
TABS

# ---- multiple newlines in source (whitespace skipping) ----



x = 1



y = 2



# ---- very long identifier ----
this_is_a_very_long_identifier_name_that_should_not_cause_any_issues_with_the_lexer = 1

# ---- identifier with trailing ? and ! then operator ----
valid? && true
run! || false

# ---- global variable edge cases ----
$0
$1
$9
$:
$"
$'
$~
$!
$?
$.
$,
$;
$&
$*
$+
$-
$/
$\\
$`
$<
$>
$=
$_
$!

# ---- instance and class variables with punctuation contexts ----
@foo
@@bar
@foo.bar
@@bar.baz

# ---- colon edge cases ----
:x          # symbol
:           # bare colon (valid in some contexts)
::Foo       # scope resolution
Foo::Bar    # scope resolution

# ---- `&` ambiguity ----
x & y       # bitwise AND
x && y      # logical AND
x &= y      # AND-assign
x&.foo      # lonely operator
&block      # capture/proc

# ---- `*` ambiguity ----
x * y       # multiply
x ** y      # power
x *= y      # mul-assign
x **= y     # pow-assign
*x          # splat

# ---- ? followed by various chars ----
?a          # char literal 'a'
?\n         # char literal newline
?\C-x       # char literal control-x
?\u{2603}   # char literal unicode
?           # question mark (followed by space -> QMARK token)

# ---- __END__ and __LINE__ ----
__LINE__
__FILE__
__ENCODING__
# __END__ (commented out so rest of file parses)

# ---- mixed single/double quote strings adjacent ----
x = "double" 'single'

# ---- escape sequence variants ----
# control/meta combinations
"\C-x"
"\M-x"
"\C-\M-x"
"\M-\C-x"
"\cX"
"\C-\\"     # control-backslash
"\M-\\"     # meta-backslash

# hex escapes
"\x41"
"\x4"

# octal escapes
"\o{101}"
"\o40"
"\o7"

# unicode
"A"
"\u{2603}"
"\u{1F600 1F601}"

# ---- interpolation depth 5 levels ----
"a#{"b#{"c#{"d#{"e#{1}"}"}"}"}z"

# ---- heredoc with only backslash-newline content ----
# (empty after line-continuation processing)

# ---- float with rational suffix and exponent ----
1.5e10r
2.5E-5i

# ---- numbers with underscores in exponent ----
# 1.5e1_0  -- this might not be valid, skip

# ---- multiple adjacent string literals ----
"hello" "world"
"a" "b" "c"

# ---- comment with #{} and #@ inside ----
# #{this is a comment} #@still_comment
x = 1  # trailing #{not_interp} #@nope

# ---- backslash at end of line in source (not string) ----
x = 1 \
+ 2

# ---- mixed heredoc + other tokens on same line ----
x = <<EOS
hello
EOS
y = 1

# ---- heredoc with semicolons on delim line ----
# <<EOS ; (the ; becomes part of the post-body)

# ---- CLASS << is LSHIFT not heredoc ----
class Foo << Bar
end

class Foo << Bar; end
