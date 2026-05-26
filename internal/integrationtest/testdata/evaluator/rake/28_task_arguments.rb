# minversion: 2.6
# Exercise Rake::TaskArguments directly. The task-arg machinery
# rake passes to a parameterised task's block.

$LOAD_PATH.unshift __dir__ + "/../../gems/rake/lib"

require "rake"

# Construct a TaskArguments with named args + provided values.
ta = Rake::TaskArguments.new([:name, :greeting], ["world", "hello"])

# Hash-like read by symbol or string name.
puts ta[:name]                            #=> world
puts ta[:greeting]                        #=> hello
puts ta["name"]                           #=> world

# Method-call read by name.
puts ta.name                              #=> world
puts ta.greeting                          #=> hello

# to_hash and to_a.
puts ta.to_hash.sort.map { |k, v| "#{k}=#{v}" }.join(",")  #=> greeting=hello,name=world
