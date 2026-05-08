# syntax introduced in ruby 2.5 (fails 2.4, passes 2.5+)
#
# boundary: 2.4 -> 2.5
# note: 2.4 had no parser-relevant syntax changes, so this spans 2.3 -> 2.5
#
# changes exercised:
#   - rescue/else/ensure inside do/end blocks (previously only begin/method bodies)

# rescue in do/end block
[1, 2, 3].each do |x|
  x / 0
rescue ZeroDivisionError
  nil
end

# rescue + ensure in do/end block
[1].map do |x|
  x.to_s
rescue => e
  e.message
ensure
  nil
end

# rescue + else + ensure in do/end block
[1].select do |x|
  x > 0
rescue
  false
else
  true
ensure
  nil
end

# nested rescue in do/end
[1].each do |x|
  begin
    x / 0
  rescue
    nil
  end
rescue
  nil
end

# rescue in lambda do/end
f = lambda do |x|
  x / 0
rescue ZeroDivisionError
  -1
end
