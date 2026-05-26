# `self.foo` is an explicit-receiver call. Private methods reject
# self as receiver too (since Ruby 2.7 self.foo= is allowed for
# attr writers; 2.6 still rejects the read). bare `foo` from inside
# the class is allowed.
class C
  def go
    foo                                   # bare, ok
  end
  def go_self
    self.foo                              # explicit self, private rejects
  end
  private
  def foo
    "f"
  end
end

c = C.new
puts c.go                                 #=> f
begin
  c.go_self
rescue NoMethodError
  puts "self.foo rejected"                #=> self.foo rejected
end
