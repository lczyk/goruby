# Time#<=> now falls back to other.<=>(self) when the other side
# isn't a Time but is an Instance with its own <=>. Negates the
# result -- MRI's coerce-style fallback that lets Rake::LATE
# (whose <=> returns 1 for all inputs) compare both ways with Time.

class Late
  include Comparable
  def <=>(other); 1; end
end

late = Late.new
p Time.now < late                           #=> true
p Time.now <= late                          #=> true
p late > Time.now                           #=> true
p late >= Time.now                          #=> true

# Identity short-circuit on Comparable#==: same object is always ==
# even when <=> returns nonzero. MRI keeps Object#== / equal?
# semantics for this exact case.
p late == late                              #=> true
