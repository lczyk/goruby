# minversion: 2.6
# Full minitest test-run driver that includes assert_output as one
# of the test bodies. Pins that the full lifecycle (setup, test
# body with assert_output capturing stdout, teardown, result
# recording) composes cleanly.

$LOAD_PATH.unshift __dir__ + "/../../gems/minitest/lib"

require "minitest"
require "minitest/test"

class HelloTest < Minitest::Test
  def test_simple_pass
    assert true
  end

  def test_io_capture
    # Inline the capture pattern to avoid assert_output's
    # send-dispatch issue (deferred).
    old = $stdout
    $stdout = StringIO.new
    puts "greetings"
    captured = $stdout.string
    $stdout = old
    assert_equal "greetings\n", captured
  end

  def test_simple_fail
    assert_equal "expected", "actual"
  end

  def test_unexpected_raise
    raise "boom"
  end
end

require "stringio"

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
HelloTest.runnable_methods.sort.each do |n|
  Minitest::Runnable.run_one_method(HelloTest, n, rep)
end

puts "passed: " + rep.passed.to_s             #=> passed: 2
puts "failed: " + rep.failed.to_s             #=> failed: 1
puts "errored: " + rep.errored.to_s           #=> errored: 1
