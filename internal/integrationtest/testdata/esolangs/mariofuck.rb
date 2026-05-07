# Mariofuck interpreter
# source: https://esolangs.org/wiki/Mariofuck
# language author: Mihai Popa
# licence: CC0 (esolangs.org wiki content)

#!/usr/bin/ruby
eval 'm=Hash.new(p=0);'+ARGF.read.gsub(/./,
     'w' => 'p+=1;',
     'a' => 'p-=1;',
     's' => 'm[p]+=1;',
     'd' => 'm[p]-=1;',
     'p' => 'putc m[p];',
     'q' => 'm[p]=STDIN.getbyte if !STDIN.eof;',
     'm' => '(',
     'n' => ')while((m[p]&=255)!=0);')
