# minversion: 2.6
# Minitest setup/teardown hooks. setup runs before each test method;
# teardown runs after, even on assertion failure. Drive the lifecycle
# via run_one_method (the minitest runner's internal entry point).

$LOAD_PATH.unshift __dir__ + "/../../gems/minitest/lib"

require "minitest"
require "minitest/test"

class MyTest < Minitest::Test
  TRACE = []

  def setup
    TRACE << "setup:#{name}"
  end

  def teardown
    TRACE << "teardown:#{name}"
  end

  def test_one
    TRACE << "body:test_one"
    assert true
  end

  def test_two
    TRACE << "body:test_two"
    assert_equal 1, 1
  end
end

# Drive each test method through the lifecycle manually.
[:test_one, :test_two].each do |m|
  inst = MyTest.new(m)
  inst.setup
  inst.send(m)
  inst.teardown
end

puts MyTest::TRACE.join(",")
#=> setup:test_one,body:test_one,teardown:test_one,setup:test_two,body:test_two,teardown:test_two
