# Spoon interpreter
# source: https://esolangs.org/wiki/Spoon
# language author: S. Goodwin
# licence: CC0 (esolangs.org wiki content)
h = {
    '1' => 'm[p]+=1;',
    '000' => 'm[p]-=1;',
    '010' => 'p+=1;',
    '011' => 'p-=1;',
    '0011' => ')while((m[p]&=255)!=0);',
    '00100' => '(',
    '001010' => 'putc m[p];',
    '0010110' => 'm[p]=STDIN.getbyte if !STDIN.eof;',
    '00101110' => 'print "\n",m,"\n";',
    '00101111' => 'exit;'
}

r = Regexp.union(Regexp.union(h.keys.sort{|a,b|b.length<=>a.length}),/./);
eval 'm=Hash.new(p=0);'+ARGF.read.gsub(/[^01]/,'').gsub(r,h);
