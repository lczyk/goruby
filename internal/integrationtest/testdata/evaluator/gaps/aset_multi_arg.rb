class Grid
  def initialize
    @cells = {}
  end
  def [](x, y); @cells[[x, y]]; end
  def []=(x, y, val); @cells[[x, y]] = val; end
end

g = Grid.new
g[1, 2] = "a"
g[3, 4] = "b"
puts g[1, 2]
puts g[3, 4]
