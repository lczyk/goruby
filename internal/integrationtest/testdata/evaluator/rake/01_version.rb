# minversion: 2.6
# rung 1 -- cheapest possible rake probe.
#
# loads only rake/version.rb (not the full rake.rb chain) and reads the
# top-level Rake::VERSION constant. exercises:
#   - require_relative across dirs into the fetched gem tree
#   - `module Rake; end` reopen pattern
#   - frozen-string-literal magic comment tolerance
#   - constant lookup across module nesting
#
# split-and-splat into module-scoped constants (Rake::Version::MAJOR etc)
# is covered by 02_version_constants.rb separately, since it currently
# fails in the evaluator (skip-listed).
#
# if THIS fails, every other rake driver will fail too. fix this first.

require_relative "../../gems/rake/lib/rake/version"

puts Rake::VERSION  #=> 13.2.1
