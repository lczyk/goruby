# minversion: 2.6
# Pin Class.inherited hook + super-chain resolution for the
# subclass-registry pattern minitest's Runnable uses.

REGISTRY = []

class Tracker
  def self.inherited(klass)
    REGISTRY << klass
    super  # delegate to Object.inherited noop
  end
end

class Worker < Tracker; end
class Builder < Tracker; end
class SpecialWorker < Worker; end

puts REGISTRY.length                          #=> 3
puts REGISTRY.map { |k| k.name }.sort.join(",")
                                              #=> Builder,SpecialWorker,Worker

# Multiple levels of inherited hooks compose.
TRACE = []
class MidLayer < Tracker
  def self.inherited(klass)
    TRACE << "mid"
    super
  end
end

class Below < MidLayer; end

puts TRACE.inspect                            #=> ["mid"]
puts REGISTRY.include?(Below)                 #=> true
puts REGISTRY.include?(String)                #=> false
