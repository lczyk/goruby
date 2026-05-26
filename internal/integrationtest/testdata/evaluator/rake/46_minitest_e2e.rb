# minversion: 2.6
# End-to-end: a tiny minitest-style test file. Defines a Test class
# with a handful of tests (pass / fail / raise / skip), runs them
# through Minitest::Runnable.run_one_method, and collects results.

$LOAD_PATH.unshift __dir__ + "/../../gems/minitest/lib"

require "minitest"
require "minitest/test"

class CalcTest < Minitest::Test
  def test_add_passes
    assert_equal 4, 2 + 2
  end

  def test_sub_fails
    assert_equal 99, 1 - 0
  end

  def test_raises_unexpected
    raise "boom"
  end

  def test_skip_path
    skip "not yet"
  end

  def test_assert_raises_ok
    assert_raises(RuntimeError) { raise "nope" }
  end
end

# A minimal stub reporter that records counts and per-test outcomes.
class Recorder
  attr_reader :passed, :failed, :errored, :skipped
  def initialize
    @passed = 0
    @failed = 0
    @errored = 0
    @skipped = 0
  end
  def prerecord(klass, name); end
  def record(result)
    if result.skipped?
      @skipped += 1
    elsif result.error?
      @errored += 1
    elsif result.passed?
      @passed += 1
    else
      @failed += 1
    end
  end
end

rep = Recorder.new
CalcTest.runnable_methods.sort.each do |name|
  Minitest::Runnable.run_one_method(CalcTest, name, rep)
end

puts "passed: " + rep.passed.to_s            #=> passed: 2
puts "failed: " + rep.failed.to_s            #=> failed: 1
puts "errored: " + rep.errored.to_s          #=> errored: 1
puts "skipped: " + rep.skipped.to_s          #=> skipped: 1
