# minversion: 2.6
# Pin three more rake suites that benefit from prior fixes:
#   - test_rake_late_time (2/2) -- LateTime <=> always 1
#   - test_rake_cpu_counter (1/3) -- CpuCounter introspection
#   - test_rake_top_level_functions (3/5) -- bare top-level DSL

$LOAD_PATH.unshift __dir__ + "/../../gems/minitest/lib"
$LOAD_PATH.unshift __dir__ + "/../../gems/rake/lib"

require "minitest"
require "minitest/test"
require_relative __dir__ + "/../../gems/rake/test/helper.rb"
require_relative __dir__ + "/../../gems/rake/test/test_rake_late_time.rb"
require_relative __dir__ + "/../../gems/rake/test/test_rake_cpu_counter.rb"
require_relative __dir__ + "/../../gems/rake/test/test_rake_top_level_functions.rb"

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

[TestRakeLateTime, TestRakeCpuCounter, TestRakeTopLevelFunctions].each do |k|
  rep = Rep.new
  k.runnable_methods.sort.each do |n|
    begin
      Minitest::Runnable.run_one_method(k, n, rep)
    rescue Exception
    end
  end
  puts "#{k}: p=#{rep.passed} f=#{rep.failed} e=#{rep.errored}"
end
#=> TestRakeLateTime: p=2 f=0 e=0
#=> TestRakeCpuCounter: p=1 f=2 e=0
#=> TestRakeTopLevelFunctions: p=4 f=1 e=0
