# Paren-less call as class superclass. MRI's grammar accepts a
# command-style call (no parens around args) in the superclass slot;
# the canonical example is DelegateClass from the stdlib delegate
# library. Goruby's parser had to be relaxed to thread parseExpression
# back into a command-call when the parsed superclass is an Identifier
# followed by a paren-less arg.
#
# See parser fix accompanying this fixture. Originally surfaced by
# rasel/lib/rasel.rb in the gem corpus.

require "delegate"

class FromConst < DelegateClass Rational
  def kind; "rational"; end
end

class FromCall < DelegateClass(Integer)
  def kind; "integer"; end
end

# Multi-arg paren-less call as superclass. Synthetic but on-grammar:
# any method returning a Class is fair game in the superclass slot,
# and command-call args extend through commas.
def Tuple(*types)
  Struct.new(*types.map(&:name).map(&:to_sym))
end

class Pair < Tuple Integer, String
  def shape; "pair"; end
end

# Sanity: the body parses + dispatches normally despite the unusual
# header shape.
puts FromConst.new(Rational(1, 2)).kind
puts FromCall.new(7).kind
puts Pair.new(1, "x").shape
