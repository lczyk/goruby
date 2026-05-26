# minversion: 2.6
# Drill into the Minitest result objects directly: name, klass,
# assertions count, failures collection. Pins the introspection
# surface that custom reporters / CI integrations rely on.

$LOAD_PATH.unshift __dir__ + "/../../gems/minitest/lib"

require "minitest"
require "minitest/test"

class WidgetTest < Minitest::Test
  def test_pass
    assert true
  end

  def test_fail
    assert_equal 1, 2
  end

  def test_error
    raise StandardError, "kaboom"
  end
end

class Capture
  attr_reader :results
  def initialize; @results = []; end
  def prerecord(klass, name); end
  def record(r); @results << r; end
end

cap = Capture.new
WidgetTest.runnable_methods.sort.each do |name|
  Minitest::Runnable.run_one_method(WidgetTest, name, cap)
end

# Sort so the output order is stable.
ordered = cap.results.sort_by(&:name)

puts ordered[0].name                        #=> test_error
puts ordered[0].class_name                  #=> WidgetTest
puts ordered[0].error?                      #=> true
puts ordered[0].failures.length             #=> 1

puts ordered[1].name                        #=> test_fail
puts ordered[1].passed?                     #=> false
puts ordered[1].error?                      #=> false
puts ordered[1].assertions                  #=> 1

puts ordered[2].name                        #=> test_pass
puts ordered[2].passed?                     #=> true
puts ordered[2].error?                      #=> false
puts ordered[2].assertions                  #=> 1
puts ordered[2].failures.length             #=> 0
