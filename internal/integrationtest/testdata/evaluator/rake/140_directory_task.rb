# minversion: 2.6
# Pin rake's test_rake_directory_task (5/5). Unlocked by exposing
# FileUtils methods as both class methods and instance methods, so
# `include FileUtils` (which rake's DSL does) lets the directory
# task's body call `mkdir_p` on the includer's instance.

$LOAD_PATH.unshift __dir__ + "/../../gems/minitest/lib"
$LOAD_PATH.unshift __dir__ + "/../../gems/rake/lib"

require "minitest"
require "minitest/test"
require_relative __dir__ + "/../../gems/rake/test/helper.rb"
require_relative __dir__ + "/../../gems/rake/test/test_rake_directory_task.rb"

class Rep
  attr_reader :passed
  def initialize; @passed = 0; end
  def prerecord(*); end
  def record(r); @passed += 1 if r.passed?; end
end

rep = Rep.new
TestRakeDirectoryTask.runnable_methods.sort.each do |n|
  begin
    Minitest::Runnable.run_one_method(TestRakeDirectoryTask, n, rep)
  rescue Exception
  end
end
puts "TestRakeDirectoryTask: p=#{rep.passed}"  #=> TestRakeDirectoryTask: p=5
