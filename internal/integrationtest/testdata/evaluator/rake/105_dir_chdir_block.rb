# minversion: 2.6
# Pin Dir.chdir block form -- returns block's value, restores
# original cwd.

orig = Dir.pwd

# Block form returns block value.
r = Dir.chdir("/tmp") do
  Dir.pwd
end
puts r.start_with?("/")                        #=> true
puts r.end_with?("tmp")                        #=> true

# cwd restored after block.
puts Dir.pwd == orig                           #=> true

# Nested chdir blocks unwind in order.
trace = []
Dir.chdir("/tmp") do
  trace << Dir.pwd
  Dir.chdir("/") do
    trace << Dir.pwd
  end
  trace << Dir.pwd
end
puts trace.length                              #=> 3
puts trace[0].end_with?("tmp")                 #=> true
puts trace[1] == "/"                           #=> true
puts trace[2].end_with?("tmp")                 #=> true
puts Dir.pwd == orig                           #=> true

# No-block form still works and returns 0.
puts Dir.chdir(orig)                           #=> 0
