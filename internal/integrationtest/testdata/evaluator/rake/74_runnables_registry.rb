# minversion: 2.6
# Pin Minitest::Runnable.runnables auto-registry now that
# Class.inherited fires (iter 84-85). Subclasses of Runnable
# self-register so callers can iterate without manual tracking.

$LOAD_PATH.unshift __dir__ + "/../../gems/minitest/lib"

require "minitest"
require "minitest/test"

class AlphaTest < Minitest::Test
  def test_one; assert true; end
end

class BetaTest < Minitest::Test
  def test_two; assert true; end
end

# AlphaTest and BetaTest both land in Runnable.runnables via the
# inherited hook.
puts Minitest::Runnable.runnables.include?(AlphaTest)   #=> true
puts Minitest::Runnable.runnables.include?(BetaTest)    #=> true

# Filter just user-defined tests (skip the Test base class).
user_tests = Minitest::Runnable.runnables.reject { |k| k == Minitest::Test }
puts user_tests.length >= 2                              #=> true

# Their runnable_methods accumulate.
all_methods = user_tests.flat_map { |k| k.runnable_methods }.sort
puts all_methods.include?("test_one")                    #=> true
puts all_methods.include?("test_two")                    #=> true
