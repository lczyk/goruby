# minversion: 2.6
# End-to-end Rakefile-shaped exercise. Builds a small task graph
# with namespaces, file tasks, prereqs, parameterised tasks, and
# multitask, then drives the graph from a single top-level invoke
# and asserts the resulting trace.

$LOAD_PATH.unshift __dir__ + "/../../gems/rake/lib"

require "rake"

trace = []

namespace :build do
  task :prep do
    trace << "prep"
  end

  desc "Compile sources"
  task compile: :prep do
    trace << "compile"
  end

  task :link, [:target] => :compile do |_t, args|
    trace << "link(#{args.target})"
  end
end

namespace :test do
  task unit: "build:link" do
    trace << "unit"
  end

  task integration: "build:link" do
    trace << "integration"
  end

  multitask all: [:unit, :integration] do
    trace << "all"
  end
end

task default: "test:all"

# Invoke the default chain, passing a target arg through.
Rake::Task["build:link"].invoke("release")

# Re-enable :unit / :integration so all reruns them too. multitask
# runs in goruby with a serial fallback so the order is stable.
Rake::Task["test:all"].invoke

puts trace.join(",")
                #=> prep,compile,link(release),unit,integration,all

# Task name introspection survives the run.
puts Rake::Task["build:link"].full_comment.nil?   #=> true
puts Rake::Task["test:all"].class.to_s            #=> Rake::MultiTask
