# Brainfoctal interpreter
# source: https://esolangs.org/wiki/Brainfoctal
# licence: CC0 (esolangs.org wiki content)

#!/usr/bin/env ruby
eval 'm=Hash.new(p=0);'+ARGF.read.gsub(/[^0-9]/,"").to_i.to_s(8).gsub(/./,
     '1' => 'p+=1;',
     '2' => 'p-=1;',
     '3' => 'm[p]+=1;',
     '4' => 'm[p]-=1;',
     '5' => 'putc m[p];',
     '6' => 'm[p]=STDIN.getbyte if !STDIN.eof;',
     '7' => '(',
     '0' => ')while((m[p]&=255)!=0);')
