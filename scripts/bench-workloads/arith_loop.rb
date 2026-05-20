# Integer-infix arithmetic loop. Heavy on Integer#+ / #* / Integer#times.
acc = 0
2000.times do
  inner = 0
  1000.times do |i|
    inner = inner + i * 2 - 1
  end
  acc = acc + inner
end
