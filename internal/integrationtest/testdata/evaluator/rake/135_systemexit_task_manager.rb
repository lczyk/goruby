# minversion: 2.6
# Kernel#exit now raises SystemExit as a regular Ruby exception so
# `rescue SystemExit` / `rescue Exception` catch. evalProgram unwraps
# an unrescued SystemExit into its legacy exit-code shape.
# SystemExit#status and #success? expose the integer code.

# Direct rescue.
begin
  exit 2
rescue SystemExit => e
  puts "caught code=#{e.status}"            #=> caught code=2
  puts "success? #{e.success?}"             #=> success? false
end

# rescue Exception also catches.
begin
  exit 0
rescue Exception => e
  puts "Exception=#{e.class}"               #=> Exception=SystemExit
  puts "code=#{e.status} success=#{e.success?}" #=> code=0 success=true
end

# Downstream: rake's task_manager suite raises rake's SystemExit
# mid-run from a misconfigured task path; previously the test runner
# crashed before exercising any other test.
$LOAD_PATH.unshift __dir__ + "/../../gems/minitest/lib"
$LOAD_PATH.unshift __dir__ + "/../../gems/rake/lib"
require "minitest"
require "minitest/test"
require_relative __dir__ + "/../../gems/rake/test/helper.rb"
require_relative __dir__ + "/../../gems/rake/test/test_rake_task_manager.rb"

class Rep
  attr_reader :passed
  def initialize; @passed = 0; end
  def prerecord(*); end
  def record(r); @passed += 1 if r.passed?; end
end

rep = Rep.new
TestRakeTaskManager.runnable_methods.sort.each do |n|
  begin
    Minitest::Runnable.run_one_method(TestRakeTaskManager, n, rep)
  rescue Exception
  end
end
puts "TestRakeTaskManager: p=#{rep.passed}"  #=> TestRakeTaskManager: p=15
