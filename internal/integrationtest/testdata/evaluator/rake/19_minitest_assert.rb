# minversion: 2.6
# Build a minitest test class inline and run a single assertion.
# Exercises Minitest::Test inheritance + assert plumbing -- the
# core machinery any minitest-using gem depends on.

$LOAD_PATH.unshift __dir__ + "/../../gems/minitest/lib"

require "minitest"
require "minitest/test"

class MyTest < Minitest::Test
  def test_addition
    assert_equal 4, 2 + 2
  end
end

# Manually drive a single test method (skip the runner -- it
# pulls in a wider surface). The test method returns no value but
# raises Minitest::Assertion on failure.
result = nil
begin
  inst = MyTest.new(:test_addition)
  inst.test_addition
  result = "passed"
rescue Minitest::Assertion => e
  result = "failed: #{e.message}"
end
puts result  #=> passed
