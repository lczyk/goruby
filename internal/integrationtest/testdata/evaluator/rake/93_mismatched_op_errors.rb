# minversion: 2.6
# Pin numeric operator dispatch on mismatched types -- raises
# rescuable Ruby ArgumentError instead of internal evaluator error.
# == returns false; <=> returns nil per MRI.

# Integer comparison with String.
begin
  5.send(:<, "x")
rescue ArgumentError => e
  puts "int-< raised"                         #=> int-< raised
end

begin
  5.send(:+, "x")
rescue ArgumentError
  puts "int-+ raised"                         #=> int-+ raised
end

# == returns false (no raise).
puts 5.send(:==, "x")                         #=> false
puts 1.0.send(:==, :sym)                      #=> false

# <=> returns nil.
puts 5.send(:<=>, "x").nil?                   #=> true
puts 1.0.send(:<=>, :sym).nil?                #=> true

# Float mismatch raises too.
begin
  1.5.send(:<, "x")
rescue ArgumentError
  puts "float raised"                         #=> float raised
end
