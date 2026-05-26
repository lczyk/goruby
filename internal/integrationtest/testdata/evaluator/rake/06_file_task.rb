# minversion: 2.6
# rung 5 -- file task.
#
# `file` defines a Rake::FileTask. invoking runs the block w/ a
# task object; t.name is the target, t.source is the first prereq.
# exercises FileTask machinery + task block args.

$LOAD_PATH.unshift __dir__ + "/../../gems/rake/lib"

require "rake"
require "tmpdir"

dir = Dir.mktmpdir
src_path = File.join(dir, "in.txt")
dst_path = File.join(dir, "out.txt")

File.write(src_path, "hello")

file dst_path => src_path do |t|
  File.write(t.name, File.read(t.source).upcase)
end

Rake::Task[dst_path].invoke

puts File.read(dst_path)  #=> HELLO
