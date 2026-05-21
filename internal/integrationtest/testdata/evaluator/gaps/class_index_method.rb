class Base
  def self.[](*args)
    new(*args)
  end
  def initialize(*xs); @xs = xs; end
  def to_s; @xs.inspect; end
end

class Sub < Base
end

# Sub inherits Base.[], so Sub[1,2] should produce a Sub instance.
puts Sub[1, 2, 3].to_s
puts Sub[1, 2, 3].class
