# minversion: 2.6
# Pin user Comparable mixin: include Comparable + define <=>
# gives a class the full < / <= / > / >= / == / between? / clamp
# surface. The pattern Rake::Task uses internally and a common
# shape across gem code.

class Tagged
  include Comparable
  attr_reader :priority

  def initialize(priority)
    @priority = priority
  end

  def <=>(other)
    priority <=> other.priority
  end
end

a = Tagged.new(1)
b = Tagged.new(2)
c = Tagged.new(2)

puts a < b                                  #=> true
puts b > a                                  #=> true
puts a <= a                                 #=> true
puts a >= b                                 #=> false
puts b == c                                 #=> true
puts a == b                                 #=> false

# Comparable derives between?.
puts a.between?(Tagged.new(0), Tagged.new(5))   #=> true
puts a.between?(b, c)                           #=> false

# Sorting via Comparable's <=>.
sorted = [b, a, c].sort.map { |t| t.priority }
puts sorted.inspect                         #=> [1, 2, 2]

# min / max from Enumerable via duck-typed each in Array.
puts [b, a, c].min.priority                 #=> 1
puts [b, a, c].max.priority                 #=> 2
