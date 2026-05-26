# Kernel#printf now honours $stdout redirection. minitest's
# capture_io reassigns $stdout to a StringIO; previously printf
# wrote past it to the real stdout.

require "stringio"

io = StringIO.new
old = $stdout
$stdout = io
printf("hello %s %d\n", "world", 42)
$stdout = old
p io.string                                  #=> "hello world 42\n"

# puts also honours (regression check -- already worked).
io2 = StringIO.new
old = $stdout
$stdout = io2
puts "captured"
$stdout = old
p io2.string                                 #=> "captured\n"
