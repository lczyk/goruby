# minversion: 2.6
# Pin assert_output running inside Test#run with proper failure
# classification. Iter 81's raiseSignal-propagation + iter 82's
# bare-raise-rerise fixes together unblocked the path.

$LOAD_PATH.unshift __dir__ + "/../../gems/minitest/lib"

require "minitest"
require "minitest/test"

class IOTest < Minitest::Test
  def test_io_pass
    assert_output("hello\n") { puts "hello" }
  end

  def test_io_fail
    assert_output("expected\n") { puts "actual" }
  end

  def test_io_regex_pass
    assert_output(/world/) { puts "hello world" }
  end

  def test_unexpected_raise_in_capture
    assert_output("never reached\n") do
      raise "boom"
    end
  end
end

class Rep
  attr_reader :passed, :failed, :errored
  def initialize
    @passed = 0
    @failed = 0
    @errored = 0
  end
  def prerecord(*); end
  def record(r)
    if r.error?
      @errored += 1
    elsif r.passed?
      @passed += 1
    else
      @failed += 1
    end
  end
end

rep = Rep.new
IOTest.runnable_methods.sort.each { |n| Minitest::Runnable.run_one_method(IOTest, n, rep) }

puts "passed: " + rep.passed.to_s            #=> passed: 2
puts "failed: " + rep.failed.to_s            #=> failed: 1
puts "errored: " + rep.errored.to_s          #=> errored: 1
