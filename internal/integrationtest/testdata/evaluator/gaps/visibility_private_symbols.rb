# `private :foo, :bar` marks already-defined methods as private.
# Distinct from bare `private` which flips the body's visibility mode
# for subsequent defs.
class Box
  def a; "a"; end
  def b; "b"; end
  def c; "c"; end
  private :a, :b
end

box = Box.new
puts box.c                                #=> c
begin
  box.a
rescue NoMethodError
  puts "a rejected"                       #=> a rejected
end
begin
  box.b
rescue NoMethodError
  puts "b rejected"                       #=> b rejected
end
