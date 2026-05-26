# minversion: 2.6
# Pin StringIO -- in-memory IO buffer. Used by minitest's
# capture_io, plus tons of test setups across the gem ecosystem.

require "stringio"

io = StringIO.new
io.puts "hello"
io.puts "world"
puts io.string                              #=> hello
                                            #=> world

# read after rewind returns the full buffer.
io.rewind
puts io.read                                #=> hello
                                            #=> world

# Pre-seeded buffer.
io2 = StringIO.new("initial")
io2 << " text"
puts io2.string                             #=> initial text

# print vs puts -- print doesn't add a newline.
io3 = StringIO.new
io3.print "a"
io3.print "b"
io3.print "c"
puts io3.string                             #=> abc
