# minversion: 2.6
# Exercise scoped constant assignment -- `Foo::Bar = value`. minitest
# uses this shape: `Minitest::Expectation = Struct.new :target, :ctx`.
# Goruby previously errored with "unsupported assignment lhs
# *ast.ScopedIdentifier" on these.

module Outer
end

# Bare value assigned at a scoped name.
Outer::TIMEOUT = 30
puts Outer::TIMEOUT                          #=> 30

# Anonymous Class.new at a scoped name -- to_s renders the
# qualified path because the scoped-assign path stamps Name +
# Parent on the otherwise-anonymous class.
Outer::Quux = Class.new
puts Outer::Quux.to_s                        #=> Outer::Quux

# Class.new with an explicit super.
Outer::CustomError = Class.new(StandardError)
puts Outer::CustomError.ancestors[0..2].inspect
                                             #=> [Outer::CustomError, StandardError, Exception]

# Struct shorthand -- the minitest case.
Outer::Pair = Struct.new(:a, :b)
p = Outer::Pair.new(1, 2)
puts p.a                                     #=> 1
puts p.b                                     #=> 2

# Re-assignment overrides.
Outer::TIMEOUT = 60
puts Outer::TIMEOUT                          #=> 60
