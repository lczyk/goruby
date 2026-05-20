def shout
  yield "hi"
end

shout { |s| puts s.upcase }     #=> HI

def twice
  yield
  yield
end

twice { puts "tick" }           #=> tick
                                #=> tick

def sum_pair
  a = yield 1
  b = yield 2
  a + b
end

puts sum_pair { |x| x * 10 }    #=> 30
