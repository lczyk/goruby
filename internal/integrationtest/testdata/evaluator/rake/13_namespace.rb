# minversion: 2.6
# rung 4b -- namespaces.
#
# `namespace :build do ... end` scopes the tasks defined inside.
# `Rake::Task["build:go"].invoke` selects via the fully-qualified
# name. Exercises Rake::TaskManager's @scope stack + LinkedList
# scope path composition.

$LOAD_PATH.unshift __dir__ + "/../../gems/rake/lib"

require "rake"

namespace :build do
  task :go do
    puts "go"
  end

  task :rs do
    puts "rs"
  end
end

task :top do
  puts "top"
end

Rake::Task["build:go"].invoke
Rake::Task["build:rs"].invoke
Rake::Task["top"].invoke
# expected stdout:
#=> go
#=> rs
#=> top
