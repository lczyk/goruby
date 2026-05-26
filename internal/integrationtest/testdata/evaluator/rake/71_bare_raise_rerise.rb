# minversion: 2.6
# Pin bare raise inside a rescue body. Previously produced a
# fresh blank RuntimeError; should re-raise the currently-handled
# exception. Common in rescue chains that log + re-throw.

class MyErr < StandardError; end

logged = []

begin
  begin
    raise MyErr, "first"
  rescue MyErr
    logged << "inner-saw-myerr"
    raise
  end
rescue => e
  puts e.class                        #=> MyErr
  puts e.message                      #=> first
end

# Nested rescue + bare raise inside outer rescue.
begin
  begin
    raise MyErr, "outer-cause"
  rescue MyErr => e1
    begin
      raise StandardError, "inner-cause"
    rescue StandardError
      logged << "nested-handled"
    end
    raise # re-raise the outer MyErr
  end
rescue => e2
  puts e2.class                       #=> MyErr
  puts e2.message                     #=> outer-cause
end

puts logged.inspect                   #=> ["inner-saw-myerr", "nested-handled"]

# String#sub! -- mutating sub. Returns receiver on match; nil on no match.
s = "hello world"
result = s.sub!("world", "ruby")
puts s                                #=> hello ruby
puts result.equal?(s)                 #=> true

t = "abc"
puts t.sub!("xyz", "Q").nil?          #=> true
puts t                                #=> abc
