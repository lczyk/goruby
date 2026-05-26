# minversion: 2.6
# Exercise Rake.application -- the singleton application object that
# holds the task manager + options. Most rake DSL routines bottom out
# in Rake.application.something_or_other, so it's a useful surface
# to pin.

$LOAD_PATH.unshift __dir__ + "/../../gems/rake/lib"

require "rake"

# A fresh application has no tasks until we define one.
app = Rake.application
puts app.tasks.length                          #=> 0

# Define a couple of tasks and confirm app.tasks sees them.
task :alpha
task :beta do
end

puts app.tasks.length                          #=> 2
puts app.tasks.map(&:name).sort.join(",")      #=> alpha,beta

# Task names lookup via app[name].
puts app[:alpha].name                          #=> alpha
puts app["beta"].name                          #=> beta

# Rake::Task[] is the same dispatch from a different angle.
puts Rake::Task["alpha"].equal?(app[:alpha])   #=> true

# Per-namespace task listing via tasks_in_scope. Empty-scope matches
# only tasks with an empty leading namespace prefix -- nothing here.
namespace :batch do
  task :one
  task :two
end
batch_scope = Rake::Scope.make("batch")
puts app.tasks_in_scope(batch_scope).map(&:name).sort.join(",")
                                               #=> batch:one,batch:two
