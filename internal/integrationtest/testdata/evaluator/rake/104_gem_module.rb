# minversion: 2.6
# Pin Gem module stub + File.realpath. rake's test/helper.rb uses
# both: `gem "coveralls"` (returns true / wrapped in rescue) and
# `File.realpath(ENV["RUBY"] || Gem.ruby)`.

# Gem module exists with the standard surface.
puts defined?(Gem).nil?                       #=> false
puts Gem.is_a?(Module)                        #=> true

# Gem.ruby returns the interpreter path (some string).
puts Gem.ruby.is_a?(String)                   #=> true

# Gem::LoadError is a class.
puts Gem::LoadError.is_a?(Class)              #=> true

# Kernel#gem returns true (stub).
puts gem("any-gem")                           #=> true

# File.realpath / File.absolute_path work.
puts File.realpath("/tmp").is_a?(String)      #=> true
puts File.absolute_path("test").start_with?("/")
                                              #=> true

# Loaded specs is empty Hash.
puts Gem.loaded_specs.is_a?(Hash)             #=> true
puts Gem.loaded_specs.empty?                  #=> true
