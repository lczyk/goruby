# minversion: 2.6
# rung 8 -- parameterised tasks.
#
# task :greet, [:name] do |t, args|
#   puts "hi #{args[:name]}"
# end
# Rake::Task[:greet].invoke("world")
#
# Exercises Rake::TaskArguments: declared arg-names + invoke-time
# values get bound on a TaskArguments instance and passed as the
# second block param.

$LOAD_PATH.unshift __dir__ + "/../../gems/rake/lib"

require "rake"

task :greet, [:name] do |t, args|
  puts "hi #{args[:name]}"
end

Rake::Task[:greet].invoke("world")  #=> hi world
