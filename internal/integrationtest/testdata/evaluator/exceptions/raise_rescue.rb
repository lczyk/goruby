begin
  raise "boom"
rescue => e
  puts e.message       #=> boom
end

begin
  raise ArgumentError, "bad arg"
rescue ArgumentError => e
  puts "got: #{e.message}"   #=> got: bad arg
end

def safe_div(a, b)
  begin
    a / b
  rescue ZeroDivisionError
    nil
  end
end

p safe_div(10, 2)      #=> 5
p safe_div(10, 0)      #=> nil

ran = false
begin
  raise "x"
rescue
  # swallow
ensure
  ran = true
end
puts ran               #=> true
