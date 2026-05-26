# minversion: 2.6
# minitest assert_raises -- expects a specific exception from the block.
# Covers exception-class matching in assertion form (vs raise-then-rescue
# of the previous driver).

$LOAD_PATH.unshift __dir__ + "/../../gems/minitest/lib"

require "minitest"
require "minitest/test"

class MyTest < Minitest::Test
  def test_raises_expected
    assert_raises(ArgumentError) do
      raise ArgumentError, "expected"
    end
  end

  def test_raises_wrong_class
    assert_raises(ArgumentError) do
      raise TypeError, "wrong class"
    end
  end
end

inst = MyTest.new(:test_raises_expected)
begin
  inst.test_raises_expected
  puts "test_raises_expected: pass"  #=> test_raises_expected: pass
rescue Minitest::Assertion => e
  puts "test_raises_expected: failed - #{e.message}"
end

inst2 = MyTest.new(:test_raises_wrong_class)
begin
  inst2.test_raises_wrong_class
  puts "test_raises_wrong_class: pass (unexpected)"
rescue Minitest::Assertion => e
  puts "test_raises_wrong_class: failed as expected"  #=> test_raises_wrong_class: failed as expected
rescue StandardError => e
  puts "test_raises_wrong_class: leaked #{e.class}"
end
