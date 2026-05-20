class Animal
  def initialize(name)
    @name = name
  end

  def speak
    "..."
  end

  def name
    @name
  end
end

class Dog < Animal
  def speak
    "woof"
  end
end

class Loud < Dog
  def speak
    super + "!"
  end
end

d = Dog.new("rex")
puts d.name       #=> rex
puts d.speak      #=> woof

l = Loud.new("max")
puts l.speak      #=> woof!
puts l.is_a?(Animal)  #=> true
