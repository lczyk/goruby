# minversion: 2.6
# Pin raise inside a method_missing body. Previously this looped
# infinitely: bare-name raise dispatched through Send, didn't find
# raise on the receiver, fell into method_missing, which re-raised
# inside the recursive method_missing invocation. Fixed by
# installing kernel builtins on the Kernel module so dispatch
# finds raise before method_missing.

class Mock
  def initialize(name)
    @name = name
  end

  def method_missing(sym, *args)
    raise NoMethodError, "no #{sym} on #{@name}"
  end
end

m = Mock.new("M1")
begin
  m.unknown
  puts "no raise"
rescue NoMethodError => e
  puts e.message                          #=> no unknown on M1
end

# Multiple unknown calls all raise cleanly.
m2 = Mock.new("M2")
begin; m2.something; rescue NoMethodError; puts "raised"; end   #=> raised
begin; m2.something; rescue NoMethodError; puts "raised"; end   #=> raised

# method_missing can also use puts / format directly.
class Tracer
  def method_missing(sym, *args)
    puts "called: #{sym}"
    "stub"
  end
end
t = Tracer.new
puts t.foo                                #=> called: foo
# (foo returns "stub")                    #=> stub
puts t.bar                                #=> called: bar
# (bar returns "stub")                    #=> stub
