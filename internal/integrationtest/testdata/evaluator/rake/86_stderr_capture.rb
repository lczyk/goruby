# minversion: 2.6
# Pin $stderr capture pattern + Kernel#warn dispatch through it.
# Mirrors iter 78's $stdout capture but for the error stream.

require "stringio"

def capture_stderr
  old = $stderr
  $stderr = StringIO.new
  yield
  $stderr.string
ensure
  $stderr = old
end

out = capture_stderr do
  warn "first warning"
  warn "second warning"
end
puts "lines: " + out.lines.length.to_s        #=> lines: 2
puts out.lines.first.strip                    #=> first warning
puts out.lines.last.strip                     #=> second warning

# Multiple args to warn -- each on its own line.
out2 = capture_stderr do
  warn "a", "b", "c"
end
puts out2.lines.length                        #=> 3

# Empty warn no-op-ish (writes nothing).
out3 = capture_stderr { warn }
puts out3                                     #=>

# minitest assert_output's stderr-half now works.
$LOAD_PATH.unshift __dir__ + "/../../gems/minitest/lib"
require "minitest"
require "minitest/assertions"

class P
  include Minitest::Assertions
  attr_accessor :assertions
  def initialize; @assertions = 0; end
end

p = P.new
p.assert_output(nil, "boom\n") { warn "boom" }
puts "assert_output stderr ok"               #=> assert_output stderr ok
