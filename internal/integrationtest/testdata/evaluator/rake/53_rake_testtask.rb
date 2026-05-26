# minversion: 2.6
# Exercise Rake::TestTask -- the standard test-runner task that
# ships with rake. Common boilerplate in every Rakefile.

$LOAD_PATH.unshift __dir__ + "/../../gems/rake/lib"

require "rake/testtask"

# Configure a test task via the canonical block form.
Rake::TestTask.new(:unit) do |t|
  t.libs << "lib"
  t.libs << "test"
  t.test_files = FileList["test/**/test_*.rb"]
  t.verbose = false
end

# The :unit task lands in the global registry.
puts Rake::Task[:unit].name                  #=> unit
puts Rake::Task.task_defined?("unit")        #=> true

# Multiple TestTask instances co-exist (different names).
Rake::TestTask.new(:integration) do |t|
  t.libs = ["lib"]
  t.test_files = FileList["test/integration/**/test_*.rb"]
end

puts Rake::Task[:integration].name           #=> integration

# Both tasks are reachable from the registry.
names = [:unit, :integration].sort.map(&:to_s).join(",")
puts names                                   #=> integration,unit
