# minversion: 2.6
# Exercise rake's error classes -- TaskArgumentError (trivial subclass
# of ArgumentError) and RuleRecursionOverflowError (StandardError
# subclass that tracks a chain of attempted rule targets and
# extends #message to render them).

$LOAD_PATH.unshift __dir__ + "/../../gems/rake/lib"

require "rake/task_argument_error"
require "rake/rule_recursion_overflow_error"

# TaskArgumentError is just an ArgumentError tag.
e = Rake::TaskArgumentError.new("bad arg")
puts e.is_a?(ArgumentError)        #=> true
puts e.message                     #=> bad arg

begin
  raise Rake::TaskArgumentError, "boom"
rescue ArgumentError => err
  puts err.class                            #=> Rake::TaskArgumentError
  puts err.message                          #=> boom
end

# RuleRecursionOverflowError tracks a target chain and renders it.
re = Rake::RuleRecursionOverflowError.new("rule cycle")
re.add_target("a.o")
re.add_target("a.c")
puts re.message                    #=> rule cycle: [a.c => a.o]

# Empty target chain still renders cleanly.
re2 = Rake::RuleRecursionOverflowError.new("init only")
puts re2.message                   #=> init only: []
