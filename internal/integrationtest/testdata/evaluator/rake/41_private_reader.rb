# minversion: 2.6
# Exercise Rake::PrivateReader -- include hook that extends the
# including class with ClassMethods, gaining the `private_reader`
# DSL that combines attr_reader + private in one declaration.

$LOAD_PATH.unshift __dir__ + "/../../gems/rake/lib"

require "rake/private_reader"

class Config
  include Rake::PrivateReader

  private_reader :host, :port

  def initialize(host, port)
    @host = host
    @port = port
  end

  def url
    "#{host}:#{port}"
  end
end

c = Config.new("localhost", 8080)

# Public method that uses the private readers internally.
puts c.url                                   #=> localhost:8080

# Direct call on the private reader from outside raises.
begin
  c.host
  puts "should have raised"
rescue NoMethodError => e
  puts "raised"                              #=> raised
end

# The reader is still callable internally via self -- exercised
# above via url. Confirm the second attr too.
begin
  c.port
  puts "should have raised"
rescue NoMethodError
  puts "raised"                              #=> raised
end
