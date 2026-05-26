# class << self inside a class body installs methods on the class's
# own singleton class -- equivalent to def self.foo. canonical
# class-method idiom.
class Foo
  class << self
    def bar
      "class method bar"
    end

    def baz
      "class method baz"
    end
  end
end
puts Foo.bar                              #=> class method bar
puts Foo.baz                              #=> class method baz
