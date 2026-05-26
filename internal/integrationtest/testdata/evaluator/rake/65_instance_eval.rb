# minversion: 2.6
# Pin Object#instance_eval -- rebinds self to the receiver while
# preserving lexical closure of the block. Common DSL primitive
# (the builder pattern, Rake's own DSL, RSpec, etc.).

class Probe
  attr_reader :traced
  def initialize
    @traced = []
  end

  def log(msg)
    @traced << msg
  end
end

p = Probe.new

# Single-statement form.
p.instance_eval { @flag = true }
puts p.instance_variable_get(:@flag)         #=> true

# Multi-statement do/end form.
p.instance_eval do
  log "hello"
  log "world"
  @count = traced.length
end

puts p.traced.join(",")                       #=> hello,world
puts p.instance_variable_get(:@count)         #=> 2

# Closure over outer locals: the block sees outer scope.
outer_label = "from-outside"
p.instance_eval do
  @label = outer_label
end
puts p.instance_variable_get(:@label)         #=> from-outside

# instance_exec is the same shape; pin it too.
p.instance_exec do
  @exec_ran = true
end
puts p.instance_variable_get(:@exec_ran)      #=> true
