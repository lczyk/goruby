# minversion: 2.6
# Pin scoped-constant inheritance + Module#const_get / #const_set
# / #constants. Common DSL surface for runtime constant lookup.

class Base
  TIMEOUT = 30
  RETRY_LIMIT = 3
end

class Worker < Base
end

# Scoped lookup walks Super chain now.
puts Worker::TIMEOUT                         #=> 30
puts Worker::RETRY_LIMIT                     #=> 3

# const_get works via Symbol and String, inherited too.
puts Base.const_get(:TIMEOUT)                #=> 30
puts Worker.const_get(:TIMEOUT)              #=> 30
puts Base.const_get("RETRY_LIMIT")           #=> 3

# const_set installs.
Base.const_set(:NEW_THING, 42)
puts Base::NEW_THING                         #=> 42
puts Worker.const_get(:NEW_THING)            #=> 42

# constants returns local names (not inherited).
class Settings
  X = 1
  Y = 2
  Z = 3
end
puts Settings.constants.sort.map { |s| s.to_s }.join(",")
                                             #=> X,Y,Z

# const_defined?(name, inherit) honors the inherit flag.
puts Worker.const_defined?(:TIMEOUT)         #=> true
puts Worker.const_defined?(:TIMEOUT, false)  #=> false
