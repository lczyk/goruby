# skip-evaluator: needs class definitions
class Counter
  def initialize
    @count = 0
  end

  def inc
    @count = @count + 1
  end

  def value
    @count
  end
end

c = Counter.new
puts c.value   #=> 0
c.inc
c.inc
c.inc
puts c.value   #=> 3
