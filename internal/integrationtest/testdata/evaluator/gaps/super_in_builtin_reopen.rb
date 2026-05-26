# Reopening a builtin (Integer) and calling super walks the ancestor
# chain to a method defined on a parent builtin class (Numeric). self
# is the builtin instance (5), not an *Instance -- dispatch must use
# the receiver's actual class for super lookup.
class Numeric
  def doublex
    self * 2
  end
end
class Integer
  def doublex
    super + 1
  end
end
puts 5.doublex                            #=> 11
puts 10.doublex                           #=> 21
