# minversion: 3.2
# `Data.define` immutable value objects. introduced 3.2.
Point = Data.define(:x, :y)
p = Point.new(x: 3, y: 4)
puts p.x      #=> 3
puts p.y      #=> 4
