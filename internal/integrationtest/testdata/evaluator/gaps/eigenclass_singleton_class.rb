# obj.singleton_class returns the per-object singleton class. it's
# distinct from obj.class. defining a method on the singleton class
# is equivalent to defining a singleton method on obj.
obj = Object.new
sc = obj.singleton_class
puts sc.class                             #=> Class
puts (sc == obj.class)                    #=> false

sc.define_method(:hi) { "via singleton_class" }
puts obj.hi                               #=> via singleton_class

other = Object.new
puts other.respond_to?(:hi)               #=> false
