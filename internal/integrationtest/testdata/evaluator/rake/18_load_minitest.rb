# minversion: 2.6
# Load minitest under goruby. Exercises a broad slice of evaluator
# surface beyond the rake track -- the cattr_accessor pattern (real
# singleton-class object on Class), undef_method, Etc.nprocessors,
# Thread::Queue, the extended exception hierarchy, and several
# constant-receiver shapes inside Minitest::Assertions.

$LOAD_PATH.unshift __dir__ + "/../../gems/minitest/lib"

require "minitest"
puts Minitest::VERSION  #=> 5.25.4
