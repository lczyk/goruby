# Top-level ruby identity constants. Rake's clean.rb / cpu_counter.rb
# probe these before deciding which fallback to take.

p RUBY_VERSION.is_a?(String)              #=> true
p RUBY_VERSION.empty?                     #=> false
p RUBY_ENGINE                             #=> "goruby"
p RUBY_ENGINE_VERSION.is_a?(String)       #=> true
p RUBY_PLATFORM.is_a?(String)             #=> true
p RUBY_RELEASE_DATE.is_a?(String)         #=> true
p RUBY_DESCRIPTION.is_a?(String)          #=> true

# Object.const_defined? sees the constants (rake probes this way).
p Object.const_defined?(:RUBY_ENGINE)     #=> true
p Object.const_defined?(:RUBY_VERSION)    #=> true
