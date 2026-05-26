# minversion: 2.6
# Pin three more rake suites:
#   - test_rake (3/3) -- top-level Rake module sanity
#   - test_trace_output (4/4) -- Rake.trace_output backing
#   - test_rake_win32 (5/6) -- Win32 helper guard

$LOAD_PATH.unshift __dir__ + "/../../gems/minitest/lib"
$LOAD_PATH.unshift __dir__ + "/../../gems/rake/lib"

require "minitest"
require "minitest/test"
require_relative __dir__ + "/../../gems/rake/test/helper.rb"
require_relative __dir__ + "/../../gems/rake/test/test_rake.rb"
require_relative __dir__ + "/../../gems/rake/test/test_trace_output.rb"
require_relative __dir__ + "/../../gems/rake/test/test_rake_win32.rb"

class Rep
  attr_reader :passed, :failed, :errored
  def initialize; @passed = @failed = @errored = 0; end
  def prerecord(*); end
  def record(r)
    if r.error?; @errored += 1
    elsif r.passed?; @passed += 1
    else; @failed += 1
    end
  end
end

[TestRake, TestTraceOutput, TestRakeWin32].each do |k|
  rep = Rep.new
  k.runnable_methods.sort.each do |n|
    begin
      Minitest::Runnable.run_one_method(k, n, rep)
    rescue Exception
    end
  end
  puts "#{k}: p=#{rep.passed} f=#{rep.failed} e=#{rep.errored}"
end
#=> TestRake: p=3 f=0 e=0
#=> TestTraceOutput: p=4 f=0 e=0
#=> TestRakeWin32: p=5 f=0 e=1
