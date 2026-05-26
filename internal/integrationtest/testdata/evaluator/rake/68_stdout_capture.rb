# minversion: 2.6
# Pin $stdout reassignment with StringIO -- the capture_io idiom
# minitest uses for assert_output and friends.

require "stringio"

def capture_stdout
  old = $stdout
  $stdout = StringIO.new
  yield
  $stdout.string
ensure
  $stdout = old
end

out = capture_stdout do
  puts "hello"
  puts "world"
end

puts "captured length: " + out.length.to_s    #=> captured length: 12
puts out.lines.first.strip                    #=> hello
puts out.lines.last.strip                     #=> world
