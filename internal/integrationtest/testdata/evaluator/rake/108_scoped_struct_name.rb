# minversion: 2.6
# Pin scoped-assign rename for Struct.new / Data.define values.
# Previously Foo::Bar = Struct.new kept the generic StructClass
# name; now the assign path renames to the binding name.

module M
end

M::Pair = Struct.new(:a, :b)
puts M::Pair.name                              #=> M::Pair

# Nested.
module M
  module Inner
  end
end
M::Inner::Triple = Struct.new(:x, :y, :z)
puts M::Inner::Triple.name                     #=> M::Inner::Triple

# Instance use still works.
p = M::Pair.new(1, 2)
puts p.a                                       #=> 1
puts p.b                                       #=> 2
puts p.class.name                              #=> M::Pair
