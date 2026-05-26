# File.stat returns a Stat-shaped object with mtime / size /
# directory? / file?. Rake's support/file_creation helper uses
# this to read mtimes before invoking tasks.

require "tempfile"

t = Tempfile.new("goruby-stat")
t.write("hi")
t.close
s = File.stat(t.path)
p s.mtime.is_a?(Time)                       #=> true
p s.size.is_a?(Integer)                     #=> true
p s.file?                                   #=> true
p s.directory?                              #=> false

require "tmpdir"
d = File.stat(Dir.tmpdir)
p d.directory?                              #=> true
p d.file?                                   #=> false
