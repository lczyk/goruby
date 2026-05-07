# Skinny pig interpreter
# source: https://esolangs.org/wiki/Skinny_pig
# licence: CC0 (esolangs.org wiki content)
eval '$m=Hash.new($p=0);b=0;'+ARGF.read.gsub(/[a-z][a-z0-9]*|./,
     'drink' => 'b+=1; b=0 if b>6;',
     'eat' => '$m[$p]+=1 if b==0; $p+=1 if b==1; $m[$p]=97 if b==1; $m[$p]+=10 if b==2; $m[$p]+=100 if b==3; $m[$p]-=1 if b==4; putc $m[$p] if b==5; $m[$p]=0 if b==6; $p-=1 if b==6;',
     'stand' => 'putc $m[$p];',
     'poop' => 'putc $m[$p];',
     'scratch' => '$m[$p]=STDIN.getbyte if !STDIN.eof;',
     'pellet' => '(',
     'pellets' => ')while(($m[$p]&=255)!=0);')
puts
p $m
