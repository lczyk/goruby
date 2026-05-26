# minversion: 2.6
# rung 10 -- multitask.
#
# `multitask :both => [:a, :b]` declares a task whose prereqs may
# run concurrently. We ship a serial fallback (Thread.new inline-
# invokes; Queue / Monitor#new_cond noop synchronisation), so the
# observable behaviour is deterministic declared-order execution.

$LOAD_PATH.unshift __dir__ + "/../../gems/rake/lib"

require "rake"

task :a do
  puts "a"
end

task :b do
  puts "b"
end

multitask :both => [:a, :b]

Rake::Task[:both].invoke
#=> a
#=> b
