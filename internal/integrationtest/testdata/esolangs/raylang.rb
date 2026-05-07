# Raylang interpreter
# source: https://esolangs.org/wiki/Raylang
# language author: LonelyFloat
# licence: CC0 (esolangs.org wiki content)

#!/usr/bin/ruby
eval 'm=Hash.new(p=0);'+ARGF.read.gsub(/./,
     'r' => 'p+=1;',
     'R' => 'p-=1;',
     'a' => 'm[p]+=1;',
     'A' => 'm[p]-=1;',
     'y' => 'putc m[p];',
     'Y' => 'm[p]=STDIN.getbyte if !STDIN.eof;',
     'l' => '(',
     'L' => ')while((m[p]&=255)!=0);')
