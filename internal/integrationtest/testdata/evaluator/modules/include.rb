module Greetable
  def greet
    "hi, #{name}"
  end
end

class Person
  include Greetable

  def initialize(name)
    @name = name
  end

  def name
    @name
  end
end

p = Person.new("ada")
puts p.greet                    #=> hi, ada
puts p.is_a?(Greetable)         #=> true
