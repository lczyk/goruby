# minversion: 2.6
# Exercise the rake task description / comment machinery. desc
# sets a sticky description that the next task definition picks
# up. Multi-line descriptions accumulate. Task#comment trims to
# the first sentence; Task#full_comment renders everything.

$LOAD_PATH.unshift __dir__ + "/../../gems/rake/lib"

require "rake"
Rake::TaskManager.record_task_metadata = true

desc "Build the thing. Quickly."
task :build do
end

t = Rake::Task[:build]
puts t.comment                           #=> Build the thing
puts t.full_comment                      #=> Build the thing. Quickly.

# Accumulate via add_description.
t.add_description("Second line.")
puts t.full_comment.include?("Build the thing")    #=> true
puts t.full_comment.include?("Second line")        #=> true
puts t.comment                                     #=> Build the thing / Second line

# Duplicate description is not appended.
before = t.full_comment
t.add_description("Second line.")
puts t.full_comment == before            #=> true

# Whitespace-only descriptions are dropped.
t.add_description("   ")
t.add_description("")
puts t.full_comment == before            #=> true

# Tasks with no description return nil from #comment / #full_comment.
task :silent do
end
puts Rake::Task[:silent].comment.nil?    #=> true
