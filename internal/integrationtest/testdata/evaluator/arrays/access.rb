a = [10, 20, 30, 40]
puts a[0]          #=> 10
puts a[3]          #=> 40
puts a[-1]         #=> 40
puts a[-2]         #=> 30
p a[1, 2]          #=> [20, 30]
p a[1..2]          #=> [20, 30]

a[0] = 99
puts a[0]          #=> 99

a << 50
puts a.length      #=> 5
puts a.last        #=> 50
