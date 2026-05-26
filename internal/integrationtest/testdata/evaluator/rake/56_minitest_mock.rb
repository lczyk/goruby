# minversion: 2.6
# Drive Minitest::Mock -- the simple mock framework that ships
# with minitest. Pins the full surface: expect / call / verify
# with and without args.

$LOAD_PATH.unshift __dir__ + "/../../gems/minitest/lib"

require "minitest/mock"

# Bare expectation: name + return value.
m = Minitest::Mock.new
m.expect(:answer, 42)
puts m.answer                                #=> 42
puts m.verify                                #=> true

# Expectation with arguments.
m2 = Minitest::Mock.new
m2.expect(:add, 5, [2, 3])
puts m2.add(2, 3)                            #=> 5
puts m2.verify                               #=> true

# Multiple expectations queue up.
m3 = Minitest::Mock.new
m3.expect(:next, "a")
m3.expect(:next, "b")
m3.expect(:next, "c")
puts m3.next                                 #=> a
puts m3.next                                 #=> b
puts m3.next                                 #=> c
puts m3.verify                               #=> true
