# minversion: 2.6
# Drive a broader slice of minitest assertion methods. Exercises
# the assert_includes / refute / assert_nil family beyond the
# simple assert_equal / assert_raises shapes.

$LOAD_PATH.unshift __dir__ + "/../../gems/minitest/lib"

require "minitest"
require "minitest/test"

class MyTest < Minitest::Test
  def test_assorted
    assert [1, 2, 3].include?(2)
    refute [1, 2, 3].include?(5)
    assert_nil nil
    assert_includes [1, 2, 3], 2
    refute_equal 1, 2
  end
end

inst = MyTest.new(:test_assorted)
begin
  inst.test_assorted
  puts "pass, assertions=#{inst.assertions}"  #=> pass, assertions=6
rescue Minitest::Assertion => e
  puts "fail: " + e.message
end
