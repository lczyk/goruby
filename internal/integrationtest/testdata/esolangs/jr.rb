# JR interpreter
# source: https://esolangs.org/wiki/JR
# language and impl author: Ethan
# licence: CC0 (esolangs.org wiki content)

#!/usr/bin/ruby
s=ARGF.read;eval 'm=Hash.new(p=0);'+s.gsub(/./,
     '[' => 'm[p]-=1;',
     ']' => 'm[p]+=1;',
     ';' => 'm[p]*=m[p];',
     '.' => 'print m[p].to_s;',
     ',' => 'putc m[p];',
     '@' => 'm[p]=0;',
     '~' => 'print "\033[2J\033[H";',
     '>' => 'p+=1;',
     '<' => 'p-=1;',
     '!' => 'print s;',
)
