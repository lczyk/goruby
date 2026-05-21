class Foo
  def pub
    secret
  end
  private
  def secret
    "shh"
  end
end

f = Foo.new
puts f.pub
begin
  f.secret
  puts "no error"
rescue NoMethodError => e
  puts "expected: #{e.class}"
end
