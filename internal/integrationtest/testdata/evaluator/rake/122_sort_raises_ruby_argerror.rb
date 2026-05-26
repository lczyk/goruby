# Array#sort and friends now raise a rescuable Ruby ArgumentError
# (was a Go-side error that escaped rescue). Confirms the failure
# routes through the normal exception machinery so minitest's
# assert_raises -- and per-test rescues in driver shells -- can
# catch.

class Wack
  def initialize(n); @n = n; end
  def <=>(other); nil; end
end

xs = [Wack.new(1), Wack.new(2)]
begin
  xs.sort
  puts "missed"
rescue ArgumentError => e
  puts "caught: #{e.message}"               #=> caught: comparison of Wack with Wack failed
end

# Mixed Integer / String still raises rescuable.
begin
  [1, "two"].sort
  puts "missed"
rescue ArgumentError => e
  puts "caught: #{e.message}"               #=> caught: comparison of String with Integer failed
end

# Pin TestRakeTask subset that does not blow up: 36/51 land green.
$LOAD_PATH.unshift __dir__ + "/../../gems/minitest/lib"
$LOAD_PATH.unshift __dir__ + "/../../gems/rake/lib"
require "minitest"
require "minitest/test"
require_relative __dir__ + "/../../gems/rake/test/helper.rb"
require_relative __dir__ + "/../../gems/rake/test/test_rake_task.rb"

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
TestRakeTask.runnable_methods.sort.each do |n|
  begin
    Minitest::Runnable.run_one_method(TestRakeTask, n, rep)
  rescue Exception
  end
end
puts "TestRakeTask: p=#{rep.passed}"        #=> TestRakeTask: p=50
