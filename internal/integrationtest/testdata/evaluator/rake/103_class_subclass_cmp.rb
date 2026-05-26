# minversion: 2.6
# Pin Class subclass comparison via infix < / <= / > / >=.
# Returns nil for unrelated classes (MRI semantics).

class A; end
class B < A; end
class C < B; end
class Z; end

# Subclass comparisons.
puts B < A                                    #=> true
puts C < A                                    #=> true
puts A < B                                    #=> false

# Inclusive.
puts A <= A                                   #=> true
puts B <= A                                   #=> true
puts B <= B                                   #=> true

# Reverse direction.
puts A > B                                    #=> true
puts A > C                                    #=> true
puts B > A                                    #=> false

# Unrelated classes return nil.
puts (A < Z).inspect                          #=> nil
puts (Z > A).inspect                          #=> nil

# Equality.
puts A == A                                   #=> true
puts A == B                                   #=> false
