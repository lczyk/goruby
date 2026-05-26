# minversion: 2.6
# Pin minitest/spec describe loading. Iters 103/106 closed the
# overflow + class_eval(&proc) gaps. Driver retry in full-corpus
# context.

$LOAD_PATH.unshift __dir__ + "/../../gems/minitest/lib"

require "minitest"
require "minitest/spec"

executed = []

describe "math" do
  executed << "outer"
  it "adds" do
    executed << "in"
  end
  describe "nested" do
    executed << "nested"
    it "subs" do
      executed << "nested-in"
    end
  end
end

# describe bodies run at load; it bodies stash as methods.
puts executed.length                          #=> 2
puts executed.include?("outer")               #=> true
puts executed.include?("nested")              #=> true
puts executed.include?("in")                  #=> false
puts executed.include?("nested-in")           #=> false
