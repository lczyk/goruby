obj = Object.new
def obj.hello
  "hi from singleton"
end
puts obj.hello

other = Object.new
begin
  other.hello
rescue NoMethodError => e
  puts "expected: #{e.class}"
end
