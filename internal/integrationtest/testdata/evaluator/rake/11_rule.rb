# minversion: 2.6
# rung 11 -- rule pattern matching.
#
# `rule ".o" => ".c"` declares a synthesis rule: any task ending in
# `.o` whose `.c` source exists gets a body that runs the block.
# Exercises Rake::Task#create_rule + rule lookup in
# enhance_with_matching_rule during invoke.

$LOAD_PATH.unshift __dir__ + "/../../gems/rake/lib"

require "rake"

dir = Dir.mktmpdir
src_path = File.join(dir, "x.c")
dst_path = File.join(dir, "x.o")

File.write(src_path, "source")

rule ".o" => ".c" do |t|
  File.write(t.name, "compiled from " + File.read(t.source))
end

Rake::Task[dst_path].invoke

puts File.read(dst_path)  #=> compiled from source
