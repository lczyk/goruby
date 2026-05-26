# minversion: 2.6
# Pin rake's test_rake_file_list: 29/67 pass. The rest need real
# filesystem globbing semantics (FileList::glob) and FileList's
# proxied-Array behaviour that goruby's stub doesn't fully model.

$LOAD_PATH.unshift __dir__ + "/../../gems/minitest/lib"
$LOAD_PATH.unshift __dir__ + "/../../gems/rake/lib"

require "minitest"
require "minitest/test"
require_relative __dir__ + "/../../gems/rake/test/helper.rb"
require_relative __dir__ + "/../../gems/rake/test/test_rake_file_list.rb"

class Rep
  attr_reader :passed
  def initialize; @passed = 0; end
  def prerecord(*); end
  def record(r); @passed += 1 if r.passed?; end
end

rep = Rep.new
TestRakeFileList.runnable_methods.sort.each do |n|
  begin
    Minitest::Runnable.run_one_method(TestRakeFileList, n, rep)
  rescue Exception
  end
end
puts "TestRakeFileList: p=#{rep.passed}"  #=> TestRakeFileList: p=29
