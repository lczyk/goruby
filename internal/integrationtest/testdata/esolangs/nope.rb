# Nope! interpreter
# source: https://esolangs.org/wiki/Nope!
# language author: User:None1
# licence: CC0 (esolangs.org wiki content)

#!/usr/bin/ruby
eval 'm=Hash.new(p=0);'+ARGF.read
    .gsub(/Nope[.!]|./i,Hash.new{|_, k| (k.length>1)?k[-1]:''; }).gsub(/\n/,'')
    .gsub(/\.+!|./,
         '!' => 'm[p]+=1;',
         '.!' => 'm[p]-=1;',
         '..!' => 'p-=1;',
         '...!' => 'p+=1;',
         '....!' => 'm[p]=STDIN.getbyte if !STDIN.eof;',
         '.....!' => 'putc m[p];',
         '......!' => '(',
         '.......!' => ')while((m[p]&=255)!=0);'
         )
