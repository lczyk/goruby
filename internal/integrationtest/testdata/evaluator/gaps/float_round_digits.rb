# Float#ceil(n) / floor(n) / round(n) with a digit-count arg should
# round to n decimal places, not collapse to an integer.
puts 1.456.ceil(2)                        #=> 1.46
puts 1.456.floor(2)                       #=> 1.45
puts 1.456.round(2)                       #=> 1.46
puts 1.4.ceil                             #=> 2
puts 1.4.floor                            #=> 1
puts 1.5.round                            #=> 2
puts 1.234.ceil(0)                        #=> 2
puts 1.234.floor(0)                       #=> 1
