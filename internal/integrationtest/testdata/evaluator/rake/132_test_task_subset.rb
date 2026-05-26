# minversion: 2.6
# Pin all of rake's test_rake_test_task. Iter 153 closed the
# kwarg-after-hash-entry parser gap so the previously-skipped
# test_task_order_only_prerequisites_key now runs.

$LOAD_PATH.unshift __dir__ + "/../../gems/minitest/lib"
$LOAD_PATH.unshift __dir__ + "/../../gems/rake/lib"

require "minitest"
require "minitest/test"
require_relative __dir__ + "/../../gems/rake/test/helper.rb"
require_relative __dir__ + "/../../gems/rake/test/test_rake_test_task.rb"

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

rep = Rep.new
TestRakeTestTask.runnable_methods.sort.each do |n|
  begin
    Minitest::Runnable.run_one_method(TestRakeTestTask, n, rep)
  rescue Exception
  end
end
puts "TestRakeTestTask: p=#{rep.passed} f=#{rep.failed} e=#{rep.errored}"
#=> TestRakeTestTask: p=15 f=1 e=1
