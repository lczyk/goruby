# minversion: 2.6
# rake task introspection -- listing + name-based lookup.

$LOAD_PATH.unshift __dir__ + "/../../gems/rake/lib"

require "rake"

task :first do
end

task :second do
end

task :third do
end

names = Rake.application.tasks.map(&:name).sort
puts names.join(",")  #=> first,second,third

# Lookup individual tasks via Rake.application[] / Rake::Task[].
puts Rake::Task[:first].name        #=> first
puts Rake.application[:second].name #=> second

# Defined? check.
puts Rake::Task.task_defined?(:first)    #=> true
puts Rake::Task.task_defined?(:missing)  #=> false
