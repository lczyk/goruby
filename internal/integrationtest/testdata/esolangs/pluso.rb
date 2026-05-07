# Pluso interpreter
# source: https://esolangs.org/wiki/Pluso
# language author: User:Icepy
# licence: CC0 (esolangs.org wiki content)
eval'c=1;'+ARGF.read.gsub(/./,'p'=>'c=(c+1)%27;','o'=>'putc c!=0?c+64:32;')
