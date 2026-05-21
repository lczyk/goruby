h = {a: 1, b: 2, c: 3}
for k, v in h
  puts "#{k}=#{v}"
end

class Counter
  include Enumerable
  def each
    yield 1
    yield 2
  end
end

for n in Counter.new
  puts n
end
