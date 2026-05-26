# minversion: 2.6
# rung 2 -- load rake.rb itself via $LOAD_PATH.
#
# anchor $LOAD_PATH entry on __dir__ so the lookup is stable regardless
# of cwd. then `require "rake"` resolves into the fetched gem tree, and
# the transitive requires inside rake (rake/ext/string -> rake/ext/core
# etc.) also go through $LOAD_PATH rather than the requirer's dir.

$LOAD_PATH.unshift __dir__ + "/../../gems/rake/lib"

require "rake"

puts Rake::VERSION  #=> 13.2.1
