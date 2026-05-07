# Percent literal adversarial examples.

# %q -- non-interpolating string
%q{hello world}
%q(no #{interp})
%q[using brackets]
%q<angle brackets>
%q|pipe|
%q!bang!

# %Q -- interpolating string
%Q{hello #{name}}
%Q(nested (parens) here)

# %w -- word array (non-interpolating)
%w[foo bar baz]
%w{one two three}
%w(a\ b c)  # escaped space
%w(hello\ world)
%w[]
%w{}

# %W -- word array (interpolating)
%W[hello #{name} world]
%W{path #{$PATH}}

# %i -- symbol array (non-interpolating)
%i[foo bar baz]
%i{one two}

# %I -- symbol array (interpolating)
%I[#{prefix}_foo #{prefix}_bar]

# %s -- single symbol
%s{foo}
%s[bar]

# %r -- regex (see regex_adversarial.rb)
%r{pattern}

# %x -- backtick command (interpolating)
%x{ls #{dir}}
%x(date)

# Bare % -- defaults to %Q (interpolating string)
%{hello #{name}}
%<angle>
%[bracket]
%(paren)
%|pipe|

# Nested delimiters -- paired delimiters track depth
%Q{outer {inner1 {inner2} inner1} outer}
%w(foo (bar) baz)
%W(hello #{ "(nested)" })
%q<outer <inner> outer>

# Modulo vs percent literal -- these should be MODULO
a = 5 % 3
a = b % c

# Method call syntax: foo %w[a b]
result = sprintf %w[one two]

# Mixed percent literals in expressions
x = [%w[a b], %i[c d], %q{str}]

# Percent literal with escaped closer
%q(hello \) world)
%q{hello \} world}

# Empty percent literals
%q()
%Q{}
%w[]
%i()
