# class << obj ... end opens obj's singleton class. methods defined
# inside attach to obj alone, not to obj.class.
obj = Object.new
class << obj
  def greet
    "hello from singleton class"
  end
end
puts obj.greet                            #=> hello from singleton class

other = Object.new
begin
  other.greet
  puts "no error"
rescue NoMethodError
  puts "expected: NoMethodError"          #=> expected: NoMethodError
end
