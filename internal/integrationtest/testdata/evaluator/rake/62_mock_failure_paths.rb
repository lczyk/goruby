# minversion: 2.6
# Pin Minitest::Mock's failure paths now that raise + Kernel-method
# dispatch from method_missing works (closed in iter 71).

$LOAD_PATH.unshift __dir__ + "/../../gems/minitest/lib"

require "minitest/mock"

# Unmocked method raises NoMethodError via method_missing.
m = Minitest::Mock.new
begin
  m.unexpected
rescue NoMethodError => e
  puts "noMethod"                              #=> noMethod
end

# Verify on uncalled expectations raises MockExpectationError.
m2 = Minitest::Mock.new
m2.expect(:foo, 42)
begin
  m2.verify
rescue MockExpectationError
  puts "verify"                                #=> verify
end

# Wrong-arg call raises MockExpectationError.
m3 = Minitest::Mock.new
m3.expect(:add, 5, [2, 3])
begin
  m3.add(1, 1)
rescue MockExpectationError
  puts "wrongArgs"                             #=> wrongArgs
end

# More expects than calls -- verify fails.
m4 = Minitest::Mock.new
m4.expect(:run, true)
m4.expect(:run, true)
m4.run
begin
  m4.verify
rescue MockExpectationError
  puts "underCalled"                           #=> underCalled
end
