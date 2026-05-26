# def self.foo and "class << self; def foo; end; end" both install
# class methods. they should be visible via .methods listing and
# callable identically.
class Widget
  def self.via_self
    "via def self"
  end

  class << self
    def via_shovel
      "via class << self"
    end
  end
end
puts Widget.via_self                      #=> via def self
puts Widget.via_shovel                    #=> via class << self
p Widget.singleton_methods.sort           #=> [:via_self, :via_shovel]
