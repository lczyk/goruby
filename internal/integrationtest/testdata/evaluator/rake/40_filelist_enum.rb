# minversion: 2.6
# Exercise Rake::FileList Enumerable-ish operations that go through
# the class_eval delegation to the underlying @items array. The
# class_eval list is built from ARRAY_METHODS = Array.instance_methods
# - Object.instance_methods, so methods like map / select / reject
# get filtered out (Object has them in goruby). FileList recovers
# them via duck-typed Enumerable: any class with its own each gets
# the Object-level derivations.

$LOAD_PATH.unshift __dir__ + "/../../gems/rake/lib"

require "rake"

fl = Rake::FileList.new("foo.rb", "bar.rb", "baz.rb")

# Delegated directly via class_eval (not filtered by ARRAY_METHODS).
puts fl.length                              #=> 3
puts fl.empty?                              #=> false
puts fl.include?("foo.rb")                  #=> true
puts fl.first                               #=> foo.rb
puts fl.last                                #=> baz.rb

# Iteration via FileList#each (delegated).
seen = []
fl.each { |x| seen << x }
puts seen.join(",")                         #=> foo.rb,bar.rb,baz.rb

# Duck-typed Enumerable: map / select / reject route through
# Object's derivations driven by FileList#each.
puts fl.map { |x| x.upcase }.join(",")      #=> FOO.RB,BAR.RB,BAZ.RB
puts fl.select { |x| x.start_with?("b") }.join(",")   #=> bar.rb,baz.rb
puts fl.reject { |x| x.start_with?("b") }.join(",")   #=> foo.rb
puts fl.count { |x| x.end_with?(".rb") }    #=> 3
