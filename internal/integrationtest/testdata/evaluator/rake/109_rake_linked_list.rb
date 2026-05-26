# minversion: 2.6
# Pin running rake's test_rake_linked_list end-to-end. The test class
# `include Rake` then names LinkedList bare -- exercises constant
# lookup through included modules.

$LOAD_PATH.unshift __dir__ + "/../../gems/minitest/lib"
$LOAD_PATH.unshift __dir__ + "/../../gems/rake/lib"

require "minitest"
require "minitest/test"
require_relative __dir__ + "/../../gems/rake/test/helper.rb"
require_relative __dir__ + "/../../gems/rake/test/test_rake_linked_list.rb"

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
TestLinkedList.runnable_methods.sort.each do |n|
  Minitest::Runnable.run_one_method(TestLinkedList, n, rep)
end

puts "passed: #{rep.passed}"                  #=> passed: 11
puts "failed: #{rep.failed}"                  #=> failed: 0
puts "errored: #{rep.errored}"                #=> errored: 0
