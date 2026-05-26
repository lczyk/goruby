# minversion: 2.6
# Pin Proc#to_proc + the method-as-block idiom. Common across the
# gem ecosystem: pass a captured method or proc through to_proc-
# accepting APIs via &.

class Greeter
  def initialize(prefix)
    @prefix = prefix
  end

  def shout(name)
    "#{@prefix} #{name.upcase}!"
  end
end

g = Greeter.new("hey")
m = g.method(:shout)

# Direct call.
puts m.call("world")                        #=> hey WORLD!

# Method via &-capture as a block.
names = ["alice", "bob"]
puts names.map(&m).inspect                  #=> ["hey ALICE!", "hey BOB!"]

# Proc#to_proc returns self.
p = proc { |x| x * 2 }
puts p.to_proc.equal?(p)                    #=> true

# Symbol#to_proc still works in tandem.
puts ["a", "b"].map(&:upcase).inspect       #=> ["A", "B"]
