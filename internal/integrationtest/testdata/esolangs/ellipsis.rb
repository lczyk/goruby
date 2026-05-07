# Ellipsis compiler (to brainfuck)
# source: https://esolangs.org/wiki/Ellipsis
# language author: Bradley Grzesiak
# licence: CC0 (esolangs.org wiki content)
(File.size(ARGV[0])/3).to_s(8).chars.map{|x| print %w(> < + - . , [ ])[x.to_i]}
