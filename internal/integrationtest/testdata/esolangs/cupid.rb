# Cupid interpreter
# source: https://esolangs.org/wiki/Cupid
# language author: Shane Torbert
# licence: CC0 (esolangs.org wiki content)
eval 'm=Hash.new(p=0);'+ARGF.read.gsub(/../,
     '>>' => 'p+=1;',
     '<<' => 'p-=1;',
     '>-' => 'm[p]+=1;',
     '-<' => 'm[p]-=1;',
     '->' => 'putc m[p];',
     '<-' => 'm[p]=STDIN.getbyte if !STDIN.eof;',
     '--' => '(',
     '<>' => ')while((m[p]&=255)!=0);',
     '><' => 'p m;')
