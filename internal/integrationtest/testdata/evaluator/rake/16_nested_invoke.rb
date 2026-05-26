# minversion: 2.6
# rake task body calling another task's invoke explicitly.
#
# Body of :outer invokes :inner directly. Exercises a task's invoke
# being called from inside another task's block (not via prereq).

$LOAD_PATH.unshift __dir__ + "/../../gems/rake/lib"

require "rake"

task :inner do
  puts "inner ran"
end

task :outer do
  puts "outer before"
  Rake::Task[:inner].invoke
  puts "outer after"
end

Rake::Task[:outer].invoke
# expected stdout:
#=> outer before
#=> inner ran
#=> outer after
