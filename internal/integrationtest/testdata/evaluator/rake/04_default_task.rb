# minversion: 2.6
# rung 3 -- minimal DSL exercise.
#
# defines a toplevel task and invokes it via Rake::Task[].
# exercises: DSL mixin into main, block-storage on task defs,
# Rake::Task[] registry lookup, #invoke driving the block.

$LOAD_PATH.unshift __dir__ + "/../../gems/rake/lib"

require "rake"

task :hello do
  puts "hi from rake"
end

Rake::Task[:hello].invoke  #=> hi from rake
