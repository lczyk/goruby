# minversion: 2.6
# Pin two more rake suites that now run after iter 153 closed the
# kwarg-after-hash-entry parser gap:
#   - test_rake_task_arguments (17/19)
#   - test_rake_task_manager_argument_resolution (1/1)
# Both were previously hanging or failing at parse time.

$LOAD_PATH.unshift __dir__ + "/../../gems/minitest/lib"
$LOAD_PATH.unshift __dir__ + "/../../gems/rake/lib"

require "minitest"
require "minitest/test"
require_relative __dir__ + "/../../gems/rake/test/helper.rb"
require_relative __dir__ + "/../../gems/rake/test/test_rake_task_arguments.rb"
require_relative __dir__ + "/../../gems/rake/test/test_rake_task_manager_argument_resolution.rb"

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

[TestRakeTaskArguments, TestRakeTaskManagerArgumentResolution].each do |k|
  rep = Rep.new
  k.runnable_methods.sort.each do |n|
    begin
      Minitest::Runnable.run_one_method(k, n, rep)
    rescue Exception
    end
  end
  puts "#{k}: p=#{rep.passed} f=#{rep.failed} e=#{rep.errored}"
end
#=> TestRakeTaskArguments: p=18 f=1 e=0
#=> TestRakeTaskManagerArgumentResolution: p=1 f=0 e=0
