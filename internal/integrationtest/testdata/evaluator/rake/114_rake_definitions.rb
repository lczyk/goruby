# minversion: 2.6
# Pin rake's test_rake_definitions -- 5/6 pass. The remaining failure
# (test_implicit_file_dependencies) needs Kernel#open + Dir.mkdir
# integration that is deferred.

$LOAD_PATH.unshift __dir__ + "/../../gems/minitest/lib"
$LOAD_PATH.unshift __dir__ + "/../../gems/rake/lib"

require "minitest"
require "minitest/test"
require_relative __dir__ + "/../../gems/rake/test/helper.rb"
require_relative __dir__ + "/../../gems/rake/test/test_rake_definitions.rb"

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
TestRakeDefinitions.runnable_methods.sort.each do |n|
  Minitest::Runnable.run_one_method(TestRakeDefinitions, n, rep)
end
puts "p=#{rep.passed} f=#{rep.failed} e=#{rep.errored}"
#=> p=6 f=0 e=0
