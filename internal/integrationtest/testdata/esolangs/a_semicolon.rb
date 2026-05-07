# A:; interpreter
# source: https://esolangs.org/wiki/A:;
# language author: User:OriginalOldMan
# licence: CC0 (esolangs.org wiki content)
print Dir.pwd
print ">>"
file = File.open(gets.delete("\n"), "rb")
program = file.read
file.close
program = program.gsub("\\n","\n")
$stmts = program.split(';')
$stmc = 0
$vars = [0,0,0,0,0,0,0,0,0,0,0,0]
def getAdress(char)
   case char
       when 'j'
       return 0
       when 'l'
       return 1
       when 'b'
       return 2
       when 'o'
       return 3
       when 'c'
       return 4
       when 'q'
       return 5
       when 'r'
       return 6
       when 't'
       return 7
       when 'u'
       return 8
       when 'v'
       return 9
       when 'w'
       return 10
       when 'x'
       return 11
       end
   end
def run(stmt)
   tokens = stmt.split(':')
   case tokens[0]
       when 'p'
       print $vars[getAdress(tokens[1])]
       when 'i'
       tvar = gets
       $vars[getAdress(tokens[1])] = tvar[0..-2]
       when 'n'
       $vars[getAdress(tokens[1])] = Integer(gets)
       when 'g'
       $stmc = tokens[1].to_f - 1
       when '?'
       case tokens[2]
           when '='
           if $vars[getAdress(tokens[1])].to_s != $vars[getAdress(tokens[3])].to_s
               $stmc = $stmc + tokens[4].to_f
           end
           when '<'
           if $vars[getAdress(tokens[1])].to_f >= $vars[getAdress(tokens[3])].to_f
               $stmc = $stmc + tokens[4].to_f
               end
           when '>'
           if $vars[getAdress(tokens[1])].to_f <= $vars[getAdress(tokens[3])].to_f
               $stmc = $stmc + tokens[4].to_f
           end
       end
       when 'a'
           $vars[getAdress(tokens[1])] = $vars[getAdress(tokens[1])].to_f + $vars[getAdress(tokens[2])].to_f
       when 's'
       $vars[getAdress(tokens[1])] = $vars[getAdress(tokens[1])].to_f - $vars[getAdress(tokens[2])].to_f
       when 'm'
       $vars[getAdress(tokens[1])] = $vars[getAdress(tokens[1])].to_f * $vars[getAdress(tokens[2])].to_f
       when 'd'
       $vars[getAdress(tokens[1])] = $vars[getAdress(tokens[1])].to_f / $vars[getAdress(tokens[2])].to_f
       when 'k'
       abort()
       else
       $vars[getAdress(tokens[0])]= tokens[1]
       end
end
while $stmc < $stmts.count
   run($stmts[$stmc])
   $stmc = $stmc + 1
end
