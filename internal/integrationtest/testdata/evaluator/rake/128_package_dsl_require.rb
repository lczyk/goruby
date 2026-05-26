# minversion: 2.6
# Pin three more rake suites:
#   - test_rake_dsl (4/4) -- exercises the bare top-level rake DSL
#   - test_rake_package_task (5/7) -- PackageTask init + name resolution
#   - test_rake_require (1/3) -- task auto-import via Rake.application.add_loader

$LOAD_PATH.unshift __dir__ + "/../../gems/minitest/lib"
$LOAD_PATH.unshift __dir__ + "/../../gems/rake/lib"

require "minitest"
require "minitest/test"
require_relative __dir__ + "/../../gems/rake/test/helper.rb"
require_relative __dir__ + "/../../gems/rake/test/test_rake_dsl.rb"
require_relative __dir__ + "/../../gems/rake/test/test_rake_package_task.rb"
require_relative __dir__ + "/../../gems/rake/test/test_rake_require.rb"

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

[TestRakeDsl, TestRakePackageTask, TestRakeRequire].each do |k|
  rep = Rep.new
  k.runnable_methods.sort.each do |n|
    begin
      Minitest::Runnable.run_one_method(k, n, rep)
    rescue Exception
    end
  end
  puts "#{k}: p=#{rep.passed} f=#{rep.failed} e=#{rep.errored}"
end
#=> TestRakeDsl: p=4 f=0 e=0
#=> TestRakePackageTask: p=6 f=1 e=0
#=> TestRakeRequire: p=1 f=0 e=2
