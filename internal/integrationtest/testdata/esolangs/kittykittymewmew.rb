# KittyKittyMewMew interpreter
# source: https://esolangs.org/wiki/KittyKittyMewMew
# language author: User:ryanninjasheep
# licence: CC0 (esolangs.org wiki content)
C = {/M/ => "pu",/O/ => ' g',%r,W, => "ets",/E/=>%%ts%,/.*[^OpuWtMsEge\t \n]/=>"puts 'invalid code'#"}
def run_code code
  eval(C.to_a.inject(code){|x,y| x.gsub y[0],y[1]})
end
