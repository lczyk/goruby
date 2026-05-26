# minversion: 2.6
# Exercise define_method's self-rebinding contract. The block body
# runs with self == the dispatch receiver, so @ivar reads access
# the per-instance state. Closure lookups for free variables still
# go through the block's lexical scope. Minitest::Mock's proxy
# methods rely on this.

class Probe
  def initialize(label)
    @label = label
    @log = []
  end

  attr_reader :log

  outer_marker = "set-at-class-body-time"

  %i[describe announce].each do |name|
    method_name = name
    define_method method_name do |arg|
      # @ivars resolve against self (the Probe instance).
      @log << "#{method_name}(#{arg})=#{@label}"
      # Closure variable from the outer iteration captures by
      # reference; reads should still resolve.
      "#{outer_marker}-#{method_name}"
    end
  end
end

p = Probe.new("alpha")
puts p.describe(:thing)                   #=> set-at-class-body-time-describe
puts p.announce(42)                       #=> set-at-class-body-time-announce
puts p.log.join("|")                      #=> describe(thing)=alpha|announce(42)=alpha

# Independent instance has its own ivars.
q = Probe.new("beta")
q.describe(:other)
puts q.log.join("|")                      #=> describe(other)=beta
puts p.log.join("|")                      #=> describe(thing)=alpha|announce(42)=alpha
