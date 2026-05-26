# minversion: 2.6
# Pin minitest's assert_output -- relies on $stdout reassignment
# capturing via StringIO + raise-with-instance for the
# UnexpectedError wrap path.

$LOAD_PATH.unshift __dir__ + "/../../gems/minitest/lib"

require "minitest"
require "minitest/assertions"

class P
  include Minitest::Assertions
  attr_accessor :assertions
  def initialize
    @assertions = 0
  end
end

p = P.new

# Pass: stdout matches expected.
p.assert_output("hello\n") { puts "hello" }
puts "pass ok"                              #=> pass ok

# Fail: stdout differs.
begin
  p.assert_output("expected\n") { puts "actual" }
rescue Minitest::Assertion
  puts "fail ok"                            #=> fail ok
end

# Pattern-match form (Regexp).
p.assert_output(/world/) { puts "hello world" }
puts "regex ok"                             #=> regex ok

# stderr capture also routes through $stderr swap.
# (We don't yet test stderr because $stderr reassignment isn't wired
#  to kernel print/warn; pass nil for stderr to skip the check.)
p.assert_output("x\n") { puts "x" }
puts "done"                                 #=> done
