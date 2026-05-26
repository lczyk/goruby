# String#=~ now exists as a method-form alias for the infix =~.
# respond_to?(:=~) returns true; minitest's assert_match calls =~
# on the actual string (not via infix), so the method-form must
# exist for the assertion to work.

s = "hello world"
p s =~ /wor/                                #=> 6
p s.=~(/wor/)                               #=> 6
p s.respond_to?(:=~)                        #=> true

# Match sets $~ / $1 globals the same way the infix path does.
s =~ /(\w+) (\w+)/
p $1                                        #=> "hello"
p $2                                        #=> "world"

# Miss returns nil and clears the globals.
p "abc".=~(/z/)                             #=> nil
p $1                                        #=> nil
