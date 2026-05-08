# exception handling: full combinations

# begin/rescue/ensure/else
begin
  x = 1
rescue StandardError => e
  puts e
rescue RuntimeError
  puts "runtime"
rescue
  puts "default"
else
  puts "no error"
ensure
  x = nil
end

# rescue modifier (single line)
x = may_fail rescue default_value

# rescue in method body without explicit begin
def foo
  may_fail
rescue StandardError => e
  handle_error(e)
end

# bare rescue in method
def bar
  raise "oops"
rescue
  nil
end

# rescue with retry pattern
def with_retry
  attempt ||= 0
  attempt += 1
  raise "fail"
rescue
  retry if attempt < 3
end

# nested rescue blocks
begin
  begin
    raise "inner"
  rescue
    puts "caught inner"
  end
rescue
  puts "caught outer"
end

# ensure without rescue
begin
  allocate_resource
ensure
  cleanup_resource
end

# rescue with exception class and no variable
begin
  risky_operation
rescue RuntimeError
  handle
end

# rescue with multiple exception classes
begin
  work
rescue IOError, SystemCallError => e
  handle(e)
end

# begin/rescue/else/ensure (full four-part)
begin
  attempt
rescue SomeError => e
  recover(e)
else
  no_error
ensure
  cleanup
end

# rescue with semicolons
begin; 1; rescue; 2; rescue X; 3; end

# rescue in class body
class C
  def foo; end
rescue
  nil
end
