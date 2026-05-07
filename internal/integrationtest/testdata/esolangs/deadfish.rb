# Deadfish interpreter
# source: https://esolangs.org/wiki/Deadfish/Implementations_(M-Z)
# language author: Jonathan Todd Skinner
# impl author: Abe Voelker
# licence: CC0 (esolangs.org wiki content)

#!/usr/bin/env ruby

n = 0
while true
  print '>> '
  gets.chomp.each_char do |c|
    n = 0 if [-1, 256].include?(n)
    case c
      when 'd' then n -= 1
      when 'i' then n += 1
      when 'o' then puts n
      when 's' then n *= n
    end
  end
end
