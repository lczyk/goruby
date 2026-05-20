x = 2

case x
when 1
  puts "one"
when 2
  puts "two"      #=> two
when 3
  puts "three"
else
  puts "other"
end

label = case x
        when 1 then "a"
        when 2 then "b"
        else        "c"
        end
puts label        #=> b
