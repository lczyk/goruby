# minversion: 2.6
# Pin cycle-safe IsAncestor. Class include cycles previously
# stack-overflowed; the visited-set guard breaks the cycle.

module Inner
end

class Holder
  include Inner
end

# Deliberately create a cycle: Inner includes Holder (which already
# includes Inner). MRI raises ArgumentError on the second include;
# our impl silently accepts it. The important behaviour is that
# downstream is_a? / kind_of? / case-equal don't overflow.
Inner.include Holder rescue nil

puts Holder.new.is_a?(Inner)                  #=> true
puts Holder.new.is_a?(Holder)                 #=> true
puts Holder.new.is_a?(String)                 #=> false

# Normal include chains still work.
module M
end
class C
  include M
end
class D < C
end
puts D.new.is_a?(M)                           #=> true
puts D.new.is_a?(C)                           #=> true
puts D.new.is_a?(D)                           #=> true
