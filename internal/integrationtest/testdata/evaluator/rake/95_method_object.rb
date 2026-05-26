# minversion: 2.6
# Pin distinct Method class for Object#method(:name) return value.
# Previously returned a Proc tagged as Proc; now reads as Method.

class Greeter
  def initialize(prefix)
    @prefix = prefix
  end
  def hi(name)
    "#{@prefix} #{name}"
  end
end

g = Greeter.new("hello")
m = g.method(:hi)

# Class introspection.
puts m.class                                #=> Method

# Method#name returns Symbol.
puts m.name                                 #=> hi
puts m.name.class                           #=> Symbol

# Method#receiver returns the bound receiver.
puts m.receiver.equal?(g)                   #=> true

# Method#call dispatches.
puts m.call("world")                        #=> hello world

# Method-as-block via &m still works (Method->to_proc is implicit).
puts ["a", "b"].map(&m).inspect             #=> ["hello a", "hello b"]

# Method#to_proc returns a Proc (not Method).
puts m.to_proc.class                        #=> Proc
