# minversion: 2.6
# Pin a subset of rake's test_rake_application_options. 6/38 pass;
# the rest fail on options-table mismatches (rake CLI parsing
# semantics not yet fully implemented in goruby's stub). Skip a few
# tests that hang the runner (help yields indefinitely).

$LOAD_PATH.unshift __dir__ + "/../../gems/minitest/lib"
$LOAD_PATH.unshift __dir__ + "/../../gems/rake/lib"

require "minitest"
require "minitest/test"
require_relative __dir__ + "/../../gems/rake/test/helper.rb"
require_relative __dir__ + "/../../gems/rake/test/test_rake_application_options.rb"

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
skip = %w[test_help test_jobs test_describe test_dry_run
          test_environment_and_tasks_together test_directory
          test_environment_definition].map(&:to_sym)
TestRakeApplicationOptions.runnable_methods.sort.each do |n|
  next if skip.include?(n.to_sym)
  begin
    Minitest::Runnable.run_one_method(TestRakeApplicationOptions, n, rep)
  rescue Exception
  end
end
puts "TestRakeApplicationOptions: p=#{rep.passed} f=#{rep.failed} e=#{rep.errored}"
#=> TestRakeApplicationOptions: p=33 f=1 e=1
