# minversion: 2.6
# Pin Module#const_missing hook -- fires for scoped lookups that
# don't resolve.

class AutoLoader
  @loaded = {}
  def self.const_missing(name)
    @loaded[name] = "stub-for-#{name}"
    @loaded[name]
  end
  def self.loaded
    @loaded
  end
end

# Trigger const_missing.
puts AutoLoader::FOO                          #=> stub-for-FOO
puts AutoLoader::BAR                          #=> stub-for-BAR

# Hook captured the names.
puts AutoLoader.loaded.keys.sort.inspect      #=> [:BAR, :FOO]

# Defined constants still resolve normally.
class Settings
  REAL = 42
  def self.const_missing(name)
    "fallback-#{name}"
  end
end
puts Settings::REAL                           #=> 42
puts Settings::MISSING                        #=> fallback-MISSING
