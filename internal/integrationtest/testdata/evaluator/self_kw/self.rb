puts self.class       #=> Object

class Foo
  def who
    self.class
  end

  def self.kind
    "class-level"
  end
end

f = Foo.new
puts f.who            #=> Foo
puts Foo.kind         #=> class-level
