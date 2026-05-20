# User-class instantiation + method dispatch + instance var R/W. The
# tightest realistic OO hot loop.
class Counter
  def initialize
    @n = 0
  end
  def bump(by)
    @n += by
    self
  end
  def value
    @n
  end
end

3000.times do
  c = Counter.new
  300.times { |i| c.bump(i) }
  c.value
end
