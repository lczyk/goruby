# minversion: 2.6
# Pin IO constant + STDOUT/STDERR ancestors include it.

# IO is defined.
puts defined?(IO).nil?                        #=> false
puts IO.is_a?(Class)                          #=> true

# STDOUT / STDERR ancestors include IO.
puts STDOUT.ancestors.include?(IO)            #=> true
puts STDERR.ancestors.include?(IO)            #=> true

# STDOUT methods still work after the IO superclass wiring.
require "stringio"
old = $stdout
$stdout = StringIO.new
STDOUT.puts "echo"
captured = $stdout.string
$stdout = old
# (Note: STDOUT.puts bypasses $stdout reassignment; it goes to
#  real env.Stdout. So captured may be empty. Just check no
#  exception fired.)
puts "ok"                                     #=> ok
