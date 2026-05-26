# Kernel#caller now returns a backtrace shape per-method-frame:
# "<file>:<line>:in `<method>'". Line numbers are stubbed (0) since
# goruby's call frames don't track them yet, but file + method name
# are real. rake's find_location greps this output for the dsl frame.

class Demo
  def outer
    inner
  end

  def inner
    caller
  end
end

frames = Demo.new.outer
p frames.length >= 1                        #=> true
p frames.any? { |f| f.include?("outer") }   #=> true

# Top-level caller is empty (no method frame).
p caller                                    #=> []
