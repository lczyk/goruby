class Base
  def self.greet
    "hello from Base"
  end
end

class Child < Base
  def self.greet
    super + " (via Child)"
  end
end

puts Child.greet
