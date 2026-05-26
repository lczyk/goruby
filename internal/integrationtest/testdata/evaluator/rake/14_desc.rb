# minversion: 2.6
# rake desc + task introspection.
#
# `desc "..."` before a task definition attaches a comment that
# shows up via Rake::Task#comment. Exercises rake's @last_description
# state machine + add_description plumbing.

$LOAD_PATH.unshift __dir__ + "/../../gems/rake/lib"

require "rake"
Rake::TaskManager.record_task_metadata = true

desc "Greet the user"
task :hello do
  puts "hi"
end

desc "Bid farewell"
task :bye do
  puts "bye"
end

# task without desc -- comment should be nil.
task :silent do
end

puts Rake::Task[:hello].comment      #=> Greet the user
puts Rake::Task[:bye].comment        #=> Bid farewell
puts Rake::Task[:silent].comment.nil? #=> true
