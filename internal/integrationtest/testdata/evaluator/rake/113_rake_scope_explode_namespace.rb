# minversion: 2.6
# Pin three more rake test files end-to-end:
#   - test_rake_scope (7 tests, Scope linked-list path semantics)
#   - test_rake_path_map_explode (1 test, String#pathmap_explode)
#   - test_rake_name_space (4 tests, NameSpace + TaskManager)

$LOAD_PATH.unshift __dir__ + "/../../gems/minitest/lib"
$LOAD_PATH.unshift __dir__ + "/../../gems/rake/lib"

require "minitest"
require "minitest/test"
require_relative __dir__ + "/../../gems/rake/test/helper.rb"
require_relative __dir__ + "/../../gems/rake/test/test_rake_scope.rb"
require_relative __dir__ + "/../../gems/rake/test/test_rake_path_map_explode.rb"
require_relative __dir__ + "/../../gems/rake/test/test_rake_name_space.rb"

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

[TestRakeScope, TestRakePathMapExplode, TestRakeNameSpace].each do |k|
  rep = Rep.new
  k.runnable_methods.sort.each do |n|
    Minitest::Runnable.run_one_method(k, n, rep)
  end
  puts "#{k}: p=#{rep.passed} f=#{rep.failed} e=#{rep.errored}"
end
#=> TestRakeScope: p=7 f=0 e=0
#=> TestRakePathMapExplode: p=1 f=0 e=0
#=> TestRakeNameSpace: p=4 f=0 e=0
