# Subclasses of Exception (and friends) with a user-defined initialize
# now get that initialize invoked by newExceptionInstance -- including
# via `raise Cls, "msg"`. Previously @message was set but the user
# initialize was skipped, leaving downstream ivars (e.g. @targets in
# Rake::RuleRecursionOverflowError) at nil.

class MyErr < StandardError
  def initialize(*args)
    super
    @targets = []
  end
  def add(t); @targets << t; end
  def list; @targets; end
end

# Direct .new path.
e1 = MyErr.new("hi")
e1.add(:a)
p e1.list                                   #=> [:a]
p e1.message                                #=> "hi"

# raise / rescue path now also runs the user initialize.
begin
  raise MyErr, "boom"
rescue MyErr => e2
  e2.add(:b)
  p e2.list                                 #=> [:b]
  p e2.message                              #=> "boom"
end
