# Two small additions:
# - instance_variable_set / get also work on Class objects (MRI: a
#   Class is an Object so it can carry module-level @ivars).
# - Kernel#sleep accepts a numeric duration and returns the rounded-
#   down seconds. Goruby doesn't actually block; rake's multitask
#   test calls sleep purely to interleave threads.

class Foo
end

Foo.instance_variable_set(:@kept, 42)
p Foo.instance_variable_get(:@kept)         #=> 42

# Direct ivar read inside a class body also sees the value.
class Foo
  @persisted = "yes"
  def self.persisted; @persisted; end
end
p Foo.persisted                             #=> "yes"

# Kernel#sleep with no arg / Integer / Float -- never blocks; returns
# the rounded duration.
p sleep                                      #=> 0
p sleep(1)                                   #=> 1
p sleep(0.5)                                 #=> 0
