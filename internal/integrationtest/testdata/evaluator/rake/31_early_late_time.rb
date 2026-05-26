# minversion: 2.6
# Exercise Rake::EARLY and Rake::LATE -- singleton sentinel
# timestamps used internally by rake's prereq mtime checks.

$LOAD_PATH.unshift __dir__ + "/../../gems/rake/lib"

require "rake/early_time"
require "rake/late_time"

# Singleton identity -- repeated lookup yields the same instance.
puts Rake::EarlyTime.instance.equal?(Rake::EARLY)   #=> true
puts Rake::LateTime.instance.equal?(Rake::LATE)     #=> true

# to_s.
puts Rake::EARLY.to_s                                #=> <EARLY TIME>
puts Rake::LATE.to_s                                 #=> <LATE TIME>

# Comparable derivations: EARLY < anything, LATE > anything.
puts(Rake::EARLY < 0)                                #=> true
puts(Rake::EARLY < "any string")                     #=> true
puts(Rake::LATE > 0)                                 #=> true
puts(Rake::LATE > "any string")                      #=> true

# Direct <=> matches the documented contract.
puts(Rake::EARLY <=> 42)                             #=> -1
puts(Rake::LATE  <=> 42)                             #=> 1
