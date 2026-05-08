# comprehensive syntax stress-test combining many edge cases
# each section exercises a different dark corner of Ruby

# flip-flop
i = 0
while i < 10
  puts i if (i == 3)..(i == 7)
  i += 1
end

# all bitwise/shift operators
a = 1 << 4 | 2
b = 256 >> 2 & 0x0f
c = ~a ^ b

# range forms
r1 = ..5
r2 = 1..
r3 = 1..10
r4 = 1...10

# for + until loop
for i in 1..3
  break if i == 2
end
x = 0
until x == 3
  x += 1
end

# block-local variables
[1,2].each do |i; shadow|
  shadow = i * 2
end

# stabby lambda
l = ->(x = 1, *args, &blk) { x }

# endless method
def quick(x) = x * 2

# heredoc with interpolation
s = <<~MSG
  #{1+2}
MSG

# percent literals with various delimiters
%q|pipe|
%Q!bang #{1}!
%w(paren words)
%i[sym array]
%s|sym|
%r{regex}
%x(ls)

# string concatenation
cat = "a" 'b' "c"

# implicit hash as last arg
foo a: 1, b: 2

# full rescue/ensure/else
begin
  work
rescue SpecificError => e
  handle(e)
rescue
  default
else
  cleanup_ok
ensure
  always
end

# rescue modifier
x = risky rescue safe

# case/when with splat
case x
when 1, 2, 3
  'few'
when *[4, 5]
  'from array'
else
  'other'
end

# defined? all forms
defined? x
defined?(x + y)
defined? @ivar

# super/yield
super
super()
super 1, 2
yield
yield 1

# alias/undef/BEGIN/END
alias :new :old
undef :foo
BEGIN { init }
END { cleanup }

# multi-assignment with splat
a, *b, c = 1, 2, 3, 4

# safe navigation
obj&.foo&.bar(1)

# rightward assignment (Ruby 2.7+/3.x)
1 => x

# keyword rest + forwarding
def accepts(**kwargs)
end
def forwards(...)
  target(...)
end

# unicode identifier
привет = "hello"

# singleton class
class << self
  def meta
  end
end

# refinements
module R
  refine String do
    def custom
    end
  end
end
using R

# __END__ marker (parser should handle this gracefully)
