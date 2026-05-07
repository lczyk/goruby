# fuckbeEs interpreter
# source: https://esolangs.org/wiki/FuckbeEs
# language author: slackerSnail
# licence: CC0 (esolangs.org wiki content)

#!/usr/bin/ruby
eval 'm=Hash.new(p=0);'+ARGF.read.gsub(
       /./,
       'f' => 'p-=1;',
       'u' => 'p+=1;',
       'c' => 'm[p]+=1;',
       'k' => 'm[p]-=1;',
       'b' => 'putc m[p];',
       'e' => 'm[p]=STDIN.getbyte if !STDIN.eof;',
       'E' => '(',
       's' => ')while((m[p]&=255)!=0);')
