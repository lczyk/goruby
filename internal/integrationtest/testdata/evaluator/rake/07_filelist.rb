# minversion: 2.6
# rung 6 -- FileList glob.
#
# Rake::FileList["<dir>/*.rb"] resolves the glob lazily; .to_a forces
# it and returns an Array of matched paths. Exercises Dir.glob with
# a real pattern + Rake::FileList's Array-like surface.
#
# Uses an absolute glob (rather than chdir + relative) so the
# process-wide cwd doesn't shift and bleed into other tests in the
# same go test process.

$LOAD_PATH.unshift __dir__ + "/../../gems/rake/lib"

require "rake"

dir = Dir.mktmpdir
File.write(File.join(dir, "a.rb"), "")
File.write(File.join(dir, "b.rb"), "")
File.write(File.join(dir, "c.txt"), "")

# Use full-path glob and post-process to just the basenames so the
# fixture output is stable across runs (Dir.mktmpdir picks a fresh
# path each invocation).
fl = FileList[File.join(dir, "*.rb")]
basenames = fl.to_a.map { |p| File.basename(p) }.sort
puts basenames.join(",")  #=> a.rb,b.rb
