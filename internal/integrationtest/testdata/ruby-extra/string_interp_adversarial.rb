# String interpolation adversarial examples.

# Basic interpolation
"hello #{name}"

# Nested interpolation -- 3 levels deep
"a #{ "b #{ "c #{1 + 2}" }" } z"

# Mixed #{} and #$var / #@var in same string
"global #{$x}, instance #{@y}, expr #{1 + 2}"

# #$var interpolation (no braces)
"path is #$PATH"

# #@var and #@@var interpolation
"instance #@foo, class #@@bar"

# Interpolation with complex expressions
"result: #{foo.bar { |x| x + 1 }}"

# Interpolation inside single-quoted string -- should NOT interpolate
'no #{interpolation} here #$x #@y'

# Backslash-escaped interpolation -- no interp
"escaped \#{not_interpolated}"

# Adjacent interpolation segments
"#{a}#{b}#{c}"

# Interpolation with string inside
"outer #{ "inner #{x}" } outer"

# Empty interpolation
"before #{} after"

# Interpolation containing string with hash-like content
"#{ "looks like #{ "interp" } but isn't" }"

# Deeply nested with globals and ivars mixed
"a#{$x}b#@y c#{ "d#{$z}e" }f"

# Multiple #$var on same line
"#{$a} #{$b} #{$c}"

# Interpolation immediately adjacent to string content
"before#{expr}after"

# Backtick string with interpolation
`ls #{dir}/#{file}`

# Interpolation in string that also has escape sequences
"tab \t, newline \n, interp #{x}, unicode \u{2603}"

# #@var followed by word character -- should stop at non-ident
"#@var.foo"

# #$var with punctuation global
"pid is #$$, program is #$0"
