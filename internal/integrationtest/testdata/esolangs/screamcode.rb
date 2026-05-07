# SCREAMCODE interpreter
# source: https://esolangs.org/wiki/SCREAMCODE
# language author: Baguette
# licence: CC0 (esolangs.org wiki content)

#!/usr/bin/env ruby
eval 'm=Hash.new(p=0);'+ARGF.read.gsub(
    /[!-~]+|./,
    'AAAH' => 'p+=1;',
    'AAAAGH' => 'p-=1;',
    'FUCK' => 'm[p]+=1;',
    'SHIT' => 'm[p]-=1;',
    '!!!!!!' => 'putc m[p];',
    'WHAT?!' => 'm[p]=STDIN.getbyte if !STDIN.eof;',
    'OW' => '(',
    'OWIE' => ')while((m[p]&=255)!=0);')
