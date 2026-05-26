# super inside a module method walks the include chain to find the
# next implementation. M2 includes M1; B includes M2 and M1's foo
# reaches via the include chain.
module M1
  def greet
    "m1"
  end
end

class C
  include M1
  def greet
    "c-" + super
  end
end

puts C.new.greet                          #=> c-m1
