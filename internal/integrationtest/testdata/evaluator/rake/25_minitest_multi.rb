# minversion: 2.6
# Drive multiple test methods through Minitest::Runnable.run_one_method.
# Confirms the runner handles a mix of pass/fail results cleanly,
# the reporter captures each result, and assertion counts add up.

$LOAD_PATH.unshift __dir__ + "/../../gems/minitest/lib"

require "minitest"
require "minitest/test"

class MyTest < Minitest::Test
  def test_passing
    assert_equal 4, 2 + 2
  end

  def test_also_passing
    assert true
    assert_equal "hi", "hi"
  end

  def test_failing
    assert_equal 4, 2 + 3
  end
end

class StubReporter
  attr_reader :results
  def initialize; @results = []; end
  def prerecord(klass, name); end
  def record(result); @results << result; end
end

reporter = StubReporter.new
[:test_passing, :test_also_passing, :test_failing].each do |m|
  Minitest::Runnable.run_one_method(MyTest, m, reporter)
end

passes = reporter.results.count { |r| r.passed? }
fails  = reporter.results.count { |r| !r.passed? }
total_asserts = reporter.results.map(&:assertions).inject(0, :+)

puts "results: #{reporter.results.length}"  #=> results: 3
puts "passes: #{passes}"                    #=> passes: 2
puts "fails: #{fails}"                      #=> fails: 1
puts "assertions: #{total_asserts}"         #=> assertions: 4
