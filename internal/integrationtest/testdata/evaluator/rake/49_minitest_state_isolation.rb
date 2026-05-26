# minversion: 2.6
# Pin minitest's per-test instance isolation. Each runnable_method
# spins up a fresh Test instance, so setup runs before every test
# and instance vars don't leak between tests.

$LOAD_PATH.unshift __dir__ + "/../../gems/minitest/lib"

require "minitest"
require "minitest/test"

$shared_log = []

class IsolatedTest < Minitest::Test
  def setup
    @counter = 0
    @bag = []
    $shared_log << "setup"
  end

  def teardown
    $shared_log << "teardown(#{@bag.length})"
  end

  def test_a_increments
    @counter += 1
    @bag << :a
    assert_equal 1, @counter
    assert_equal [:a], @bag
  end

  def test_b_starts_fresh
    # @counter / @bag should be reset by setup; no leak from test_a.
    @counter += 10
    @bag << :b
    assert_equal 10, @counter
    assert_equal [:b], @bag
  end
end

class StubRep
  attr_reader :failed
  def initialize; @failed = 0; end
  def prerecord(*); end
  def record(r)
    @failed += 1 if !r.passed? && !r.skipped?
  end
end

rep = StubRep.new
IsolatedTest.runnable_methods.sort.each do |name|
  Minitest::Runnable.run_one_method(IsolatedTest, name, rep)
end

puts "failed: " + rep.failed.to_s          #=> failed: 0
puts $shared_log.join(",")                 #=> setup,teardown(1),setup,teardown(1)
