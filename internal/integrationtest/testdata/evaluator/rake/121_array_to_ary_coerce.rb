# Array#+ and Array#- coerce non-Array operands via #to_ary. Rake's
# FileList chains include + into a base Array via this path.

class WrapArr
  def initialize(arr); @arr = arr; end
  def to_ary; @arr; end
end

w = WrapArr.new([1, 2, 3])
p [0] + w                                   #=> [0, 1, 2, 3]
p [1, 2, 3, 4] - w                          #=> [4]

# Method-form dispatch (arr.send(:+, other)) also coerces.
p [0].send(:+, w)                           #=> [0, 1, 2, 3]
