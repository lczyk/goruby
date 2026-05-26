# minversion: 2.6
# Pin two more rake suites end-to-end:
#   - test_rake_extension (3 tests, rake_extension warning behaviour)
#   - test_rake_file_list_path_map (2 tests, FileList#pathmap mapping)

$LOAD_PATH.unshift __dir__ + "/../../gems/minitest/lib"
$LOAD_PATH.unshift __dir__ + "/../../gems/rake/lib"

require "minitest"
require "minitest/test"
require_relative __dir__ + "/../../gems/rake/test/helper.rb"
require_relative __dir__ + "/../../gems/rake/test/test_rake_extension.rb"
require_relative __dir__ + "/../../gems/rake/test/test_rake_file_list_path_map.rb"

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

[TestRakeExtension, TestRakeFileListPathMap].each do |k|
  rep = Rep.new
  k.runnable_methods.sort.each do |n|
    Minitest::Runnable.run_one_method(k, n, rep)
  end
  puts "#{k}: p=#{rep.passed} f=#{rep.failed} e=#{rep.errored}"
end
#=> TestRakeExtension: p=3 f=0 e=0
#=> TestRakeFileListPathMap: p=2 f=0 e=0
