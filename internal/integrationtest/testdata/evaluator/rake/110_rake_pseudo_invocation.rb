# minversion: 2.6
# Pin rake's test_rake_pseudo_status + test_rake_invocation_chain
# end-to-end. invocation_chain also exercises include-based const
# lookup (the test class does `include Rake`).

$LOAD_PATH.unshift __dir__ + "/../../gems/minitest/lib"
$LOAD_PATH.unshift __dir__ + "/../../gems/rake/lib"

require "minitest"
require "minitest/test"
require_relative __dir__ + "/../../gems/rake/test/helper.rb"
require_relative __dir__ + "/../../gems/rake/test/test_rake_pseudo_status.rb"
require_relative __dir__ + "/../../gems/rake/test/test_rake_invocation_chain.rb"

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

[TestRakePseudoStatus, TestRakeInvocationChain].each do |k|
  rep = Rep.new
  k.runnable_methods.sort.each do |n|
    Minitest::Runnable.run_one_method(k, n, rep)
  end
  puts "#{k}: passed=#{rep.passed} failed=#{rep.failed} errored=#{rep.errored}"
end
#=> TestRakePseudoStatus: passed=2 failed=0 errored=0
#=> TestRakeInvocationChain: passed=8 failed=0 errored=0
