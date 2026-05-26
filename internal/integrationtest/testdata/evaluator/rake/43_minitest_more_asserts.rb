# minversion: 2.6
# Exercise the wider minitest assertion surface beyond what
# 23_minitest_asserts.rb covered: assert_in_delta, assert_match,
# assert_kind_of, assert_instance_of, assert_operator,
# assert_respond_to, assert_same.

$LOAD_PATH.unshift __dir__ + "/../../gems/minitest/lib"

require "minitest/assertions"

class Probe
  include Minitest::Assertions
  attr_accessor :assertions
  def initialize
    @assertions = 0
  end
end

p = Probe.new

# assert_in_delta -- floating-point equality with tolerance.
p.assert_in_delta(1.0, 1.0001, 0.01)
puts "in_delta ok"                          #=> in_delta ok

# assert_match -- regex against string.
p.assert_match(/hello/, "well, hello world")
puts "match ok"                             #=> match ok

# assert_kind_of -- is_a? equivalent.
p.assert_kind_of(Numeric, 42)
puts "kind_of ok"                           #=> kind_of ok

# assert_instance_of -- exact class.
p.assert_instance_of(Integer, 42)
puts "instance_of ok"                       #=> instance_of ok

# assert_respond_to.
p.assert_respond_to("hi", :upcase)
puts "respond_to ok"                        #=> respond_to ok

# assert_operator -- generic operator assertion via __send__.
p.assert_operator(5, :<, 10)
puts "operator ok"                          #=> operator ok
p.assert_operator(10.5, :>, 5)
puts "operator float ok"                    #=> operator float ok

# assert_same -- equal? (object identity).
a = "shared"
b = a
p.assert_same(a, b)
puts "same ok"                              #=> same ok

# refute_match -- the negation path.
p.refute_match(/xyz/, "abcdef")
puts "refute_match ok"                      #=> refute_match ok

# Total assertions count (each assert / refute bumps @assertions).
puts "assertions: " + p.assertions.to_s     #=> assertions: 11
