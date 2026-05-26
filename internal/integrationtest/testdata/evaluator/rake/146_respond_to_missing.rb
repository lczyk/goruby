# respond_to? now consults respond_to_missing? before falling back
# to the dispatch probe. Without this, a class that defines
# method_missing has respond_to? always return true (since the
# probe successfully dispatches via method_missing).

class Mock
  def respond_to_missing?(name, include_private = false)
    name == :known
  end
  def method_missing(name, *a)
    "got #{name}"
  end
end

m = Mock.new
p m.respond_to?(:known)                     #=> true
p m.respond_to?(:unknown)                   #=> false
p m.known                                   #=> "got known"
p m.unknown                                 #=> "got unknown"
