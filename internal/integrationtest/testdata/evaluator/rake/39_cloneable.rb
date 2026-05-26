# minversion: 2.6
# Exercise Rake::Cloneable -- a mixin that overrides initialize_copy
# to deep-clone instance variables. Used by TaskLib subclasses so
# the same task definition can be cloned cleanly.

$LOAD_PATH.unshift __dir__ + "/../../gems/rake/lib"

require "rake/cloneable"

class Bag
  include Rake::Cloneable
  attr_accessor :items, :label

  def initialize
    @items = []
    @label = "default"
  end
end

a = Bag.new
a.items << "x"
a.items << "y"
a.label = "first"

b = a.clone

# Top-level instance vars copied.
puts b.label                         #=> first

# The @items array is deep-cloned -- mutating b.items does not
# leak back to a.items.
b.items << "z"
puts a.items.length                  #=> 2
puts b.items.length                  #=> 3
puts a.items.include?("z")           #=> false

# Strings clone too (Cloneable handles "value = src_value.clone rescue src_value").
b.label = b.label + "!"
puts a.label                         #=> first
puts b.label                         #=> first!
