# Tick interpreter
# source: https://esolangs.org/wiki/Tick
# licence: CC0 (esolangs.org wiki content)
eval 'm=Hash.new(p=0);'+ARGF.read.gsub(
        /./,
        '>' => 'p+=1;',
        '<' => 'p-=1;',
        '+' => 'm[p]+=1;',
        '*' => 'putc m[p];')
