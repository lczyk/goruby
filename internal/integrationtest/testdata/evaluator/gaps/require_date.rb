require 'date'
d = Date.new(2026, 5, 21)
puts d.year
puts d.month
puts d.day
puts (d + 7).to_s
puts d.strftime("%A")
