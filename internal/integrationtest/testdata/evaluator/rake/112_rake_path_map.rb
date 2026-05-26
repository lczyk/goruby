# Pin the rake-pathmap primitives directly: File.split + File.extname
# (dotfile semantics) + File::SEPARATOR/ALT_SEPARATOR + regex capture
# globals ($1) via case/=~. Avoids cross-fixture corpus state.

# File.split returns [dirname, basename].
p File.split("a/b/c")                       #=> ["a/b", "c"]
p File.split("only")                        #=> [".", "only"]
p File.split("/")                           #=> ["/", "/"]

# File.extname follows MRI dotfile rules: leading-dot files have no ext.
p File.extname("abc")                       #=> ""
p File.extname("abc.rb")                    #=> ".rb"
p File.extname(".depends")                  #=> ""
p File.extname("dir/.depends")              #=> ""

# Separator constants are present.
puts File::SEPARATOR                        #=> /
p File::ALT_SEPARATOR                       #=> nil

# $1 is populated after =~ and via case/when regex.
"%2d" =~ /%(-?\d+)d/
puts $1                                     #=> 2

case "%-3d"
when /%(-?\d+)d/
  puts $1                                   #=> -3
end

# Exercise rake's String#pathmap via the prod consumer.
$LOAD_PATH.unshift __dir__ + "/../../gems/rake/lib"
require "rake/ext/string"
puts "this/is/a/dir/abc.rb".pathmap("%2d")  #=> this/is
puts "this/is/a/dir/abc.rb".pathmap("%-1d") #=> dir
puts ".depends".pathmap("%x")               #=>
puts "abc.rb".pathmap("%x")                 #=> .rb
