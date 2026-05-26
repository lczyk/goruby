# minversion: 2.6
# Pin Warning module stub. MRI uses Warning.warn for deprecation
# messages; Warning[:experimental] / Warning[:deprecated] toggle
# category visibility.

require "stringio"

# Warning.warn routes through stderr (capturable via $stderr swap).
old = $stderr
$stderr = StringIO.new
Warning.warn "test deprecation"
captured = $stderr.string
$stderr = old
puts captured.include?("test deprecation")    #=> true

# Warning[] returns false for unknown categories.
puts Warning[:experimental]                   #=> false
puts Warning[:deprecated]                     #=> false

# Sugar form Warning[:x] = v also works.
Warning[:experimental] = true
puts "set ok"                                 #=> set ok
