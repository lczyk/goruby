# NoMethodError must propagate as a Ruby exception (rescuable),
# not as a Go-level evaluator error that aborts the run.
# Covers a handful of dispatch sites that historically raised
# unrescuably.

# Array missing method
begin
  [1,2,3].xyzzy
  puts "no raise"
rescue NoMethodError => e
  puts "array: #{e.class}"                #=> array: NoMethodError
end

# Hash missing method
begin
  {a: 1}.xyzzy
rescue NoMethodError => e
  puts "hash: #{e.class}"                 #=> hash: NoMethodError
end

# Instance receiver missing method (block-aware path)
class Foo
end
begin
  Foo.new.xyzzy { |x| x }
rescue NoMethodError => e
  puts "block: #{e.class}"                #=> block: NoMethodError
end

# Integer missing method (number receivers)
begin
  42.xyzzy
rescue NoMethodError => e
  puts "int: #{e.class}"                  #=> int: NoMethodError
end

# Bare top-level missing function
begin
  no_such_function(1)
rescue NoMethodError => e
  puts "kernel: #{e.class}"               #=> kernel: NoMethodError
end
