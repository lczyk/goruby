# minversion: 2.6
# Pin rake's test_rake_clean (3/7) after FileUtils.rm_r alias +
# File.readable? / writable? / executable? predicates landed.
# The remaining 4 failures need rake's trace-verbosity output
# formatting which isn't fully wired yet.

$LOAD_PATH.unshift __dir__ + "/../../gems/minitest/lib"
$LOAD_PATH.unshift __dir__ + "/../../gems/rake/lib"

require "minitest"
require "minitest/test"
require_relative __dir__ + "/../../gems/rake/test/helper.rb"
require_relative __dir__ + "/../../gems/rake/test/test_rake_clean.rb"

class Rep
  attr_reader :passed
  def initialize; @passed = 0; end
  def prerecord(*); end
  def record(r); @passed += 1 if r.passed?; end
end

rep = Rep.new
TestRakeClean.runnable_methods.sort.each do |n|
  begin
    Minitest::Runnable.run_one_method(TestRakeClean, n, rep)
  rescue Exception
  end
end
puts "TestRakeClean: p=#{rep.passed}"  #=> TestRakeClean: p=4
