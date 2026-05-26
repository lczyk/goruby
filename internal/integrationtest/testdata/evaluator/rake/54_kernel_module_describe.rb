# minversion: 2.6
# Pin the Kernel-reopening pattern + Thread.current fiber-locals.
# Both are needed to load minitest/spec. Common DSL shape across
# the gem ecosystem.

# Reopen Kernel to add a top-level helper, then call it with a
# block from the top-level scope.
module Kernel
  def describe(name, &block)
    @descriptions ||= []
    @descriptions << name
    Thread.current[:active_describe] = name
    block.call
    Thread.current[:active_describe] = nil
  end

  def active_describe
    Thread.current[:active_describe]
  end

  def descriptions
    @descriptions || []
  end
end

result = []
describe "outer" do
  result << "in:" + active_describe
end
result << "after:" + active_describe.to_s

puts result.join(",")                       #=> in:outer,after:

# Nested describes -- Thread.current[] reads survive the
# block-call boundary.
describe "again" do
  result << "in:" + active_describe
end
puts result.last                            #=> in:again

# Total descriptions tracked via top-level @ivar.
puts descriptions.length                    #=> 2
