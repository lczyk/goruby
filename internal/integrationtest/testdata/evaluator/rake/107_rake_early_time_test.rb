# minversion: 2.6
# Run rake's test_rake_early_time.rb -- exercises EarlyTime
# sentinel + Time class through real test machinery.

$LOAD_PATH.unshift __dir__ + "/../../gems/minitest/lib"
$LOAD_PATH.unshift __dir__ + "/../../gems/rake/lib"

require "minitest"
require "minitest/test"
require_relative __dir__ + "/../../gems/rake/test/helper.rb"
require_relative __dir__ + "/../../gems/rake/test/test_rake_early_time.rb"

class Rep
  attr_reader :passed, :failed, :errored
  def initialize; @passed = 0; @failed = 0; @errored = 0; end
  def prerecord(*); end
  def record(r)
    if r.error?; @errored += 1
    elsif r.passed?; @passed += 1
    else; @failed += 1
    end
  end
end

rep = Rep.new
TestRakeEarlyTime.runnable_methods.sort.each do |n|
  Minitest::Runnable.run_one_method(TestRakeEarlyTime, n, rep)
end

# Full coverage: 4/4 of rake's EarlyTime tests pass after iter 148
# extended Time#<=> with the coerce-style reversed fallback.
puts "passed: #{rep.passed}"                  #=> passed: 4
puts "total: #{TestRakeEarlyTime.runnable_methods.length}"
                                              #=> total: 4
puts rep.passed + rep.failed + rep.errored == TestRakeEarlyTime.runnable_methods.length
                                              #=> true
