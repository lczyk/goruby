# minversion: 2.6
# Minitest::Runnable.methods_matching discovers test_* methods on
# a Test subclass. Drives the runner across the discovered set
# without naming each method explicitly.

$LOAD_PATH.unshift __dir__ + "/../../gems/minitest/lib"

require "minitest"
require "minitest/test"

class MyTest < Minitest::Test
  def test_a; assert true; end
  def test_b; assert true; end
  def helper; end       # NOT a test (no test_ prefix)
end

methods = MyTest.runnable_methods.sort
puts methods.length    #=> 2
puts methods.join(",") #=> test_a,test_b
