# minversion: 2.6
# Exercise Rake::PseudoStatus -- the stand-in for Process::Status that
# rake hands back when a subprocess returns nil. Tests the encoding
# convention to_i << 8 and the predicate methods.

$LOAD_PATH.unshift __dir__ + "/../../gems/rake/lib"

require "rake/pseudo_status"

s = Rake::PseudoStatus.new(0)
puts s.exitstatus       #=> 0
puts s.to_i             #=> 0
puts s.stopped?         #=> false
puts s.exited?          #=> true

s2 = Rake::PseudoStatus.new(1)
puts s2.exitstatus      #=> 1
puts s2.to_i            #=> 256
puts(s2 >> 8)           #=> 1

s3 = Rake::PseudoStatus.new(7)
puts s3.to_i            #=> 1792
puts(s3 >> 8)           #=> 7

# Default arg is 0.
puts Rake::PseudoStatus.new.exitstatus    #=> 0
