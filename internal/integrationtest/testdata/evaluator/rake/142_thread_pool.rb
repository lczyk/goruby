# minversion: 2.6
# Pin rake's test_rake_thread_pool. 4/7 pass; the rest need real
# threading semantics that goruby's serial Thread stub doesn't
# provide.

$LOAD_PATH.unshift __dir__ + "/../../gems/minitest/lib"
$LOAD_PATH.unshift __dir__ + "/../../gems/rake/lib"

require "minitest"
require "minitest/test"
require_relative __dir__ + "/../../gems/rake/test/helper.rb"
require_relative __dir__ + "/../../gems/rake/test/test_rake_thread_pool.rb"

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
TestRakeTestThreadPool.runnable_methods.sort.each do |n|
  begin
    Minitest::Runnable.run_one_method(TestRakeTestThreadPool, n, rep)
  rescue Exception
  end
end
puts "TestRakeTestThreadPool: p=#{rep.passed} f=#{rep.failed} e=#{rep.errored}"
#=> TestRakeTestThreadPool: p=3 f=3 e=1
