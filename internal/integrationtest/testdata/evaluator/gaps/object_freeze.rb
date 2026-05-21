s = "hello".freeze
puts s.frozen?
begin
  s << " world"
  puts "no error"
rescue => e
  puts "#{e.class}: #{e.message}"
end

a = [1, 2, 3].freeze
puts a.frozen?
begin
  a << 4
rescue => e
  puts e.class
end
