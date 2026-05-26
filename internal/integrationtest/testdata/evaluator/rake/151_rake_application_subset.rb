# minversion: 2.6
# Pin a subset of rake's test_rake_application that runs cleanly.
# Test failures are options-table mismatches and display formatting
# gaps that goruby's rake stubs don't fully implement. The driver
# captures incidental stdout via a StringIO redirect so the final
# count line is the only output.

$LOAD_PATH.unshift __dir__ + "/../../gems/minitest/lib"
$LOAD_PATH.unshift __dir__ + "/../../gems/rake/lib"

require "minitest"
require "minitest/test"
require "stringio"
require_relative __dir__ + "/../../gems/rake/test/helper.rb"
require_relative __dir__ + "/../../gems/rake/test/test_rake_application.rb"

class Rep
  attr_reader :passed
  def initialize; @passed = 0; end
  def prerecord(*); end
  def record(r); @passed += 1 if r.passed?; end
end

# Redirect $stdout to a buffer for the whole run -- rake's
# display_tasks emits past capture_io for tests that print before
# wiring the capture, so we mute everything.
real_out = $stdout
real_err = $stderr
$stdout = StringIO.new
$stderr = StringIO.new

rep = Rep.new
TestRakeApplication.runnable_methods.sort.each do |n|
  begin
    Minitest::Runnable.run_one_method(TestRakeApplication, n, rep)
  rescue Exception
  end
end

$stdout = real_out
$stderr = real_err
puts "TestRakeApplication: p=#{rep.passed}" #=> TestRakeApplication: p=43
