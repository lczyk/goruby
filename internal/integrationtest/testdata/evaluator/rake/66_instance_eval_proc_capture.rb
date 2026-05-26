# minversion: 2.6
# Pin instance_eval with a &-captured block. Common Builder-pattern
# shape: a class method takes a block via &b, then forwards it to
# instance_eval(&b) to run the block's body against a fresh instance.

class Builder
  def self.build(&blk)
    instance = new
    instance.instance_eval(&blk) if blk
    instance
  end

  def configure(name)
    @name = name
  end

  def tag(label)
    @tags ||= []
    @tags << label
  end

  attr_reader :name, :tags
end

b = Builder.build do
  configure("alpha")
  tag(:fast)
  tag(:reliable)
end

puts b.name                                 #=> alpha
puts b.tags.length                          #=> 2
puts b.tags.inspect                         #=> [:fast, :reliable]

# Empty block-arg case -- no instance_eval, returns instance untouched.
b2 = Builder.build
puts b2.name.nil?                           #=> true

# Closure capture survives the &-forward.
outer = "hello"
b3 = Builder.build do
  configure(outer + "-world")
end
puts b3.name                                #=> hello-world
