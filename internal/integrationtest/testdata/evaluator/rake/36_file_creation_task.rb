# minversion: 2.6
# Exercise Rake::FileCreationTask. Different from FileTask in that
# needed? returns true based purely on existence (mtime is ignored)
# and its timestamp is Rake::EARLY so it never re-triggers downstream
# tasks once created.

$LOAD_PATH.unshift __dir__ + "/../../gems/rake/lib"

require "rake"

# Pick a path that almost certainly doesn't exist.
missing = "/tmp/goruby-fct-test-definitely-not-here-xyzzy.tmp"
File.delete(missing) if File.exist?(missing)

# Path that does exist -- use the running script.
present = __FILE__

trace = []

t_missing = Rake::FileCreationTask.define_task(missing) do
  trace << "missing-action"
end

t_present = Rake::FileCreationTask.define_task(present) do
  trace << "present-action"
end

puts t_missing.needed?                   #=> true
puts t_present.needed?                   #=> false

# timestamp is Rake::EARLY for both -- the FCT marker that pins it
# below any real time so downstream tasks never see it as newer.
puts t_missing.timestamp.equal?(Rake::EARLY)   #=> true
puts t_present.timestamp.equal?(Rake::EARLY)   #=> true

# invoke runs the action when needed?, skips when not (FileTask
# inherited behaviour).
t_missing.invoke
t_present.invoke
puts trace.join(",")                     #=> missing-action
