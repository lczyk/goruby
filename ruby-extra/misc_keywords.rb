# miscellaneous keywords: defined?, __FILE__, __LINE__, etc.

# defined? with various arguments
defined? x
defined?(x)
defined?(x + y)
defined? @x
defined? $x
defined? X
defined? puts
defined?(super)

# __FILE__ and __LINE__
file = __FILE__
line = __LINE__

# __ENCODING__
enc = __ENCODING__

# __dir__
dir = __dir__

# __callee__ and __method__
def check_names
  [__callee__, __method__]
end

# super with various forms
class Parent
  def foo(x)
  end
end

class Child < Parent
  def foo(x)
    super
    super(x)
    super(x, x)
    super()
  end
end

# yield with various forms
def yields
  yield
  yield 1
  yield 1, 2
  yield(1)
  yield(1, 2)
end

# alias and undef
alias :new_name :old_name
alias new_name2 old_name2
undef :method_name
undef method_name2, method_name3

# BEGIN and END blocks
END { puts "at exit" }
BEGIN { puts "at startup" }
