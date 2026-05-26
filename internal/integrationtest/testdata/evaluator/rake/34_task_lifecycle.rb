# minversion: 2.6
# Exercise the task lifecycle hooks rake exposes beyond plain
# invoke -- reenable, clear, clear_actions, clear_prerequisites,
# enhance, already_invoked.

$LOAD_PATH.unshift __dir__ + "/../../gems/rake/lib"

require "rake"

trace = []
task :work do
  trace << "work"
end

# First invoke runs the action.
Rake::Task[:work].invoke
puts trace.join(",")                       #=> work
puts Rake::Task[:work].already_invoked     #=> true

# Second invoke is suppressed -- already_invoked guard.
Rake::Task[:work].invoke
puts trace.join(",")                       #=> work

# reenable flips already_invoked so the next invoke runs again.
Rake::Task[:work].reenable
puts Rake::Task[:work].already_invoked     #=> false
Rake::Task[:work].invoke
puts trace.join(",")                       #=> work,work

# enhance appends actions and prereqs.
Rake::Task[:work].enhance do
  trace << "extra"
end
Rake::Task[:work].reenable
Rake::Task[:work].invoke
puts trace.join(",")                       #=> work,work,work,extra

# clear_actions drops the actions but keeps the task object.
Rake::Task[:work].clear_actions
Rake::Task[:work].reenable
Rake::Task[:work].invoke
puts trace.join(",")                       #=> work,work,work,extra
