# minversion: 2.6
# rung 4 -- prereq order.
#
# task :b => :a means b depends on a. Invoking :b should run :a first,
# then :b. Exercises rake's dependency graph + topological order.

$LOAD_PATH.unshift __dir__ + "/../../gems/rake/lib"

require "rake"

task :a do
  puts "a"
end

task :b => :a do
  puts "b"
end

Rake::Task[:b].invoke
# expected stdout:
#=> a
#=> b
