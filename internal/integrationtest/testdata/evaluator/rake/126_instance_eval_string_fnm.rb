# Two small additions:
# - instance_eval(string) parses + evals on recv's singleton class.
# - File::FNM_* glob flags exposed as Integer bitmasks.

# instance_eval string form -- defs install on the recv's singleton.
class Foo; end
obj = Foo.new
obj.instance_eval("def shout; 'BANG'; end")
puts obj.shout                              #=> BANG

# Sibling instance unaffected.
other = Foo.new
begin
  other.shout
  puts "leaked"
rescue NoMethodError
  puts "isolated"                            #=> isolated
end

# File FNM constants are present as Integers.
p File::FNM_NOESCAPE                        #=> 1
p File::FNM_PATHNAME                        #=> 2
p File::FNM_DOTMATCH                        #=> 4
p File::FNM_CASEFOLD                        #=> 8
p File::FNM_EXTGLOB                         #=> 16
