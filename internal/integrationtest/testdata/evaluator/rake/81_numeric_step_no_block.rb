# minversion: 2.6
# Pin Integer#step / Float#step no-block forms. Previously erred
# with "step requires a block".

# Integer#step.
puts 1.step(5).to_a.inspect                #=> [1, 2, 3, 4, 5]
puts 0.step(10, 2).to_a.inspect            #=> [0, 2, 4, 6, 8, 10]
puts 10.step(1, -2).to_a.inspect           #=> [10, 8, 6, 4, 2]

# Float#step.
puts (0.0).step(2.0, 0.5).to_a.inspect     #=> [0.0, 0.5, 1.0, 1.5, 2.0]
puts (1.0).step(3.0, 1.0).to_a.inspect     #=> [1.0, 2.0, 3.0]

# Block form still works.
out = []
1.step(3) { |x| out << x }
puts out.inspect                           #=> [1, 2, 3]
