# minversion: 2.6
# Pin running a real rake test file end-to-end.

$LOAD_PATH.unshift __dir__ + "/../../gems/minitest/lib"
$LOAD_PATH.unshift __dir__ + "/../../gems/rake/lib"

require "minitest"
require "minitest/test"
require_relative __dir__ + "/../../gems/rake/test/helper.rb"
require_relative __dir__ + "/../../gems/rake/test/test_private_reader.rb"

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

rep = Rep.new
TestPrivateAttrs.runnable_methods.sort.each do |n|
  Minitest::Runnable.run_one_method(TestPrivateAttrs, n, rep)
end

puts "passed: #{rep.passed}"                  #=> passed: 2
puts "failed: #{rep.failed}"                  #=> failed: 0
puts "errored: #{rep.errored}"                #=> errored: 0
