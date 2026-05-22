# Assignment as the right operand of a comparison. MRI grammar
# reduces `b = c` before the outer `==`, so `a == b = c` parses as
# `a == (b = c)`. The idiom shows up in conditions where the loop
# wants to compare against -- and remember -- the latest value:
#
#   next if prev == dump = compute()
#
# Originally surfaced by rasel/lib/rasel.rb in the gem corpus, line
# 116. The Pratt-style parser used to bail because the comparison's
# result is not assignable; parseAssignment now reparents the assign
# under the comparison's right operand.

prev = nil
dump = nil

# Single equality + single assignment.
if prev == dump = "alpha"
  puts dump
end

# Chained sites: each branch should re-bind and short-circuit.
3.times do |i|
  if prev != dump = "iter-#{i}"
    puts dump
    prev = dump
  end
end

# Non-Identifier lvalues on the RHS of the comparison: instance
# variables, globals, scoped constants.
class Holder
  attr_reader :slot
  def store(v); v == @slot = v; end
end

h = Holder.new
h.store(42)
puts h.slot
