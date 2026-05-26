# minversion: 2.6
# Pin the %p (inspect) and %c (char) format verbs. Minitest's
# Mock formats expected/actual call diffs using
# `fmt % [sym, args]` with %p; without %p support, every
# wrong-arg / unmet-expectation path crashes.

puts "%p" % [42]                              #=> 42
puts "%p" % [[1, 2, 3]]                       #=> [1, 2, 3]
h = "%p" % [{a: 1, b: 2}]
puts h.include?(":a") || h.include?("a:")     #=> true
puts "%p" % ["hi"]                            #=> "hi"
puts "%p" % [:sym]                            #=> :sym
puts "%p" % [nil]                             #=> nil
puts "method %p called with %p" % [:add, [1, 2]]
                                              #=> method :add called with [1, 2]

# %c -- codepoint to char.
puts "%c" % [65]                              #=> A
puts "%c" % [97]                              #=> a
