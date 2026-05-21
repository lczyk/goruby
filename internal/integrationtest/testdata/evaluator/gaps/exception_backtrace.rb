def deep
  raise "boom"
end

def shallow
  deep
end

begin
  shallow
rescue => e
  bt = e.backtrace
  puts bt.is_a?(Array)
  puts bt.size > 0
  puts bt.first.include?("deep")
end
