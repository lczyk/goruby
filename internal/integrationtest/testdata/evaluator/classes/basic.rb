class Greeter
  def initialize(name)
    @name = name
  end

  def hello
    "hello, #{@name}"
  end
end

g = Greeter.new("world")
puts g.hello                #=> hello, world
puts g.class                #=> Greeter
puts g.is_a?(Greeter)       #=> true
