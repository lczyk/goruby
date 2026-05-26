# lstrip / rstrip and the in-place bang variants strip! / upcase! /
# downcase!. Bangs return self on change, nil when unchanged
# (matches MRI). Using .inspect so trailing whitespace is visible in
# the marker line.
puts "  hi  ".lstrip.inspect              #=> "hi  "
puts "  hi  ".rstrip.inspect              #=> "  hi"

s = "  abc  "
puts s.strip!                             #=> abc
puts s                                    #=> abc
puts "abc".strip!.inspect                 #=> nil

t = "abc"
puts t.upcase!                            #=> ABC
puts t                                    #=> ABC
puts "ABC".upcase!.inspect                #=> nil

u = "ABC"
puts u.downcase!                          #=> abc
puts u                                    #=> abc
puts "abc".downcase!.inspect              #=> nil
