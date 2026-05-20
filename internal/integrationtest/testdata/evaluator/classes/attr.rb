class Point
  attr_accessor :x, :y

  def initialize(x, y)
    @x = x
    @y = y
  end
end

p = Point.new(3, 4)
puts p.x      #=> 3
puts p.y      #=> 4
p.x = 10
puts p.x      #=> 10
