# flip-flop operator -- a dark corner of Ruby
# inclusive (..) and exclusive (...) range in conditionals
# ruby -c confirms valid syntax

i = 0
while i < 20
  if (i == 5)..(i == 10)
    puts "5-10: #{i}"
  end
  if (i == 12)...(i == 15)
    puts "12-14: #{i}"
  end
  i += 1
end

# flip-flop in modifier form
j = 0
while j < 10
  puts "range: #{j}" if (j == 3)..(j == 7)
  j += 1
end
