# __method__ / __callee__ keywords return the current method name
# as a Symbol; nil at top level. Rake's application.rb calls
# __method__ to label trace frames.

class C
  def foo; __method__; end
  def bar; __callee__; end
end

p C.new.foo                                 #=> :foo
p C.new.bar                                 #=> :bar
p __method__                                #=> nil

# Aliased call still reports the original method's name -- ruby
# semantics: __method__ is the def-site name, __callee__ is the
# call-site name. Our shim returns the def-site name for both
# (no separate alias tracking), which is close enough for the
# rake corpus.
class D
  def real; __method__; end
  alias :surrogate :real
end
p D.new.surrogate                           #=> :real
