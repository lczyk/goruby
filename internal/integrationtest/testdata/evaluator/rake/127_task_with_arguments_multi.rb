# minversion: 2.6
# Pin two more rake suites: task_with_arguments lands 17/18,
# multi_task lands 1/5. No evaluator changes needed; these benefit
# from prior iters' fixes.

$LOAD_PATH.unshift __dir__ + "/../../gems/minitest/lib"
$LOAD_PATH.unshift __dir__ + "/../../gems/rake/lib"

require "minitest"
require "minitest/test"
require_relative __dir__ + "/../../gems/rake/test/helper.rb"
require_relative __dir__ + "/../../gems/rake/test/test_rake_task_with_arguments.rb"
require_relative __dir__ + "/../../gems/rake/test/test_rake_multi_task.rb"

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

[TestRakeTaskWithArguments, TestRakeMultiTask].each do |k|
  rep = Rep.new
  k.runnable_methods.sort.each do |n|
    begin
      Minitest::Runnable.run_one_method(k, n, rep)
    rescue Exception
    end
  end
  puts "#{k}: p=#{rep.passed} f=#{rep.failed} e=#{rep.errored}"
end
#=> TestRakeTaskWithArguments: p=17 f=0 e=1
#=> TestRakeMultiTask: p=5 f=0 e=0
