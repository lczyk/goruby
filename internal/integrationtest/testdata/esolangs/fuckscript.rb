# Fuckscript interpreter
# source: https://esolangs.org/wiki/Fuckscript
# language author: Josh Schiavone
# licence: CC0 (esolangs.org wiki content)

#!/usr/bin/ruby
eval 'm=Hash.new(p=0);'+ARGF.read.gsub(
       /\w+|./,
       'right' => 'p+=1;',
       'left' => 'p-=1;',
       'up' => 'm[p]+=1;',
       'down' => 'm[p]-=1;',
       'out' => 'putc m[p];',
       'in' => 'm[p]=STDIN.getbyte if !STDIN.eof;',
       'fuck' => '(',
       'shit' => ')while((m[p]&=255)!=0);')
