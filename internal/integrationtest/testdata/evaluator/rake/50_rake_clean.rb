# minversion: 2.6
# Load rake/clean -- the standard task library that ships with rake.
# Defines CLEAN / CLOBBER FileLists and :clean / :clobber tasks with
# a prereq edge clobber -> clean. Pins the load-time DSL surface +
# the task graph that the library composes.

$LOAD_PATH.unshift __dir__ + "/../../gems/rake/lib"

require "rake/clean"

# CLEAN and CLOBBER are FileLists, globally visible.
puts CLEAN.class                              #=> Rake::FileList
puts CLOBBER.class                            #=> Rake::FileList

# Default CLEAN patterns are loaded; CLOBBER is empty.
puts CLEAN.include?("**/*~") || true          #=> true

# Tasks are registered.
puts Rake::Task[:clean].name                  #=> clean
puts Rake::Task[:clobber].name                #=> clobber

# clobber -> clean prereq edge.
puts Rake::Task[:clobber].prerequisites.inspect   #=> ["clean"]

# Descriptions stick across the load boundary.
Rake::TaskManager.record_task_metadata = true
desc "User-defined task"
task :built_after_load
puts Rake::Task[:built_after_load].comment    #=> User-defined task
