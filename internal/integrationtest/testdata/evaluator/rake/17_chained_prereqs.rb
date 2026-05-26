# minversion: 2.6
# rake task w/ chained prereqs forming a diamond:
#
#   :final  -> :left  -> :base
#           -> :right -> :base
#
# :base should run exactly once (rake memoises @already_invoked).

$LOAD_PATH.unshift __dir__ + "/../../gems/rake/lib"

require "rake"

task :base do
  puts "base"
end

task :left => :base do
  puts "left"
end

task :right => :base do
  puts "right"
end

task :final => [:left, :right] do
  puts "final"
end

Rake::Task[:final].invoke
# expected stdout: base runs once, then left, right, final
#=> base
#=> left
#=> right
#=> final
