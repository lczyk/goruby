# minversion: 2.6
# rung 9 -- Rake.application init + top_level.
#
# Skips Application#run wrap so we don't need a real Rakefile -- the
# task is defined inline. init parses ARGV (no flags here, just task
# names); top_level invokes each named task in order.
#
# Exercises: Application#init, Application#top_level, the named-task
# invocation chain.

$LOAD_PATH.unshift __dir__ + "/../../gems/rake/lib"

require "rake"

task :greet do
  puts "hello from rake"
end

Rake.application.init("rake", ["greet"])
Rake.application.top_level  #=> hello from rake
