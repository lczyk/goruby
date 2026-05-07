# Simplified Emmental interpreter
# source: https://esolangs.org/wiki/Simplified_Emmental
# licence: CC0 (esolangs.org wiki content)

#!/usr/bin/ruby
eval 'm=[];c=0;'+ARGF.read.gsub(/[0-9#.]/,Hash.new{|_, k|"c=c*10+#{k};"}.merge({'#'=>'m<<c;c=0;','.'=>'putc c;c=m.pop;'}))
