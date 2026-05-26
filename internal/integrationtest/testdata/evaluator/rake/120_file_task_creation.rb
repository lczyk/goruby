# minversion: 2.6
# Pin rake's test_rake_file_task + test_rake_file_creation_task drivers.
# file_task lands 10/13, file_creation_task lands 5/5. The remaining
# file_task gaps need Kernel#load + per-test diff plumbing that is
# deferred.

$LOAD_PATH.unshift __dir__ + "/../../gems/minitest/lib"
$LOAD_PATH.unshift __dir__ + "/../../gems/rake/lib"

require "minitest"
require "minitest/test"
require_relative __dir__ + "/../../gems/rake/test/helper.rb"
require_relative __dir__ + "/../../gems/rake/test/test_rake_file_task.rb"
require_relative __dir__ + "/../../gems/rake/test/test_rake_file_creation_task.rb"

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

[TestRakeFileTask, TestRakeFileCreationTask].each do |k|
  rep = Rep.new
  k.runnable_methods.sort.each do |n|
    Minitest::Runnable.run_one_method(k, n, rep)
  end
  puts "#{k}: p=#{rep.passed} f=#{rep.failed} e=#{rep.errored}"
end
#=> TestRakeFileTask: p=13 f=0 e=0
#=> TestRakeFileCreationTask: p=5 f=0 e=0
