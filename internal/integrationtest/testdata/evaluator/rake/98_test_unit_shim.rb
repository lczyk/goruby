# minversion: 2.6
# Pin test/unit shim. require "test/unit" maps
# Test::Unit::TestCase -> Minitest::Test so legacy code that
# uses the older test/unit DSL runs against minitest.

$LOAD_PATH.unshift __dir__ + "/../../gems/minitest/lib"

require "minitest"
require "minitest/test"
require "test/unit"

# Test::Unit::TestCase is the minitest Test class.
puts Test::Unit::TestCase.equal?(Minitest::Test)   #=> true

# Subclass works like a minitest Test.
class MyLegacyTest < Test::Unit::TestCase
  def test_basic
    assert true
    assert_equal 4, 2 + 2
  end
end

# Ancestors chain confirms inheritance.
puts MyLegacyTest.ancestors.include?(Minitest::Test)
                                              #=> true
puts MyLegacyTest.ancestors.include?(Minitest::Assertions)
                                              #=> true

# Test instance can run a test method directly.
inst = MyLegacyTest.new(:test_basic)
inst.test_basic
puts "ran"                                    #=> ran
puts inst.assertions                          #=> 2
