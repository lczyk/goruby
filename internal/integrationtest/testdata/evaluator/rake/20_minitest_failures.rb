# minversion: 2.6
# Drive minitest assertions in both pass and fail modes. Verifies
# Minitest::Assertion raises on a false assertion and that the
# assertion-counter increments correctly.

$LOAD_PATH.unshift __dir__ + "/../../gems/minitest/lib"

require "minitest"
require "minitest/test"

class MyTest < Minitest::Test
  def test_passing
    assert_equal 4, 2 + 2
    assert true
  end

  def test_failing
    assert_equal 4, 2 + 3
  end
end

# Pass case.
inst = MyTest.new(:test_passing)
inst.test_passing
puts "passing assertions: #{inst.assertions}"  #=> passing assertions: 2

# Fail case.
inst2 = MyTest.new(:test_failing)
begin
  inst2.test_failing
  puts "no exception"
rescue Minitest::Assertion => e
  puts "failed as expected"  #=> failed as expected
end
puts "failing assertions: #{inst2.assertions}"  #=> failing assertions: 1
