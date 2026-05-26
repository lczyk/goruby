# minversion: 2.6
# Drive minitest's Runnable.run -- the real per-test invocation
# wrapper that handles setup/teardown/exception capture. Confirms
# the runner machinery works end to end for a single test method.

$LOAD_PATH.unshift __dir__ + "/../../gems/minitest/lib"

require "minitest"
require "minitest/test"

class MyTest < Minitest::Test
  def test_one
    assert_equal 4, 2 + 2
  end
end

# Minitest::Runnable.run_one_method runs the named method through
# the full lifecycle. Pass a stub reporter that records each
# result. Returns nil; results land on the reporter.
class StubReporter
  attr_reader :results
  def initialize; @results = []; end
  def prerecord(klass, name); end
  def record(result); @results << result; end
end

reporter = StubReporter.new
Minitest::Runnable.run_one_method(MyTest, :test_one, reporter)

result = reporter.results.first
puts "passed: " + result.passed?.to_s          #=> passed: true
puts "assertions: " + result.assertions.to_s    #=> assertions: 1
