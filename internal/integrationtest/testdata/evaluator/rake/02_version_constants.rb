# minversion: 2.6
# rung 1b -- module-scoped constants set via destructuring assignment.
#
# `MAJOR, MINOR, BUILD, *OTHER = Rake::VERSION.split "."` inside
# `module Rake::Version` ends up unwiring the constants in the
# evaluator -- multiple-assignment LHS doesn't recognise constant
# identifiers, so MAJOR/MINOR/BUILD stay undefined. NameError on first
# read below.
#
# remove this driver from evaluator.skip once the evaluator handles
# CONSTANT, CONSTANT2 = ... at module scope.

require_relative "../../gems/rake/lib/rake/version"

puts Rake::Version::MAJOR              #=> 13
puts Rake::Version::MINOR              #=> 2
puts Rake::Version::BUILD              #=> 1
puts Rake::Version::NUMBERS.length     #=> 3
puts Rake::Version::NUMBERS.join(",")  #=> 13,2,1
