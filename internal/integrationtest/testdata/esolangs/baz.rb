# Baz interpreter
# source: https://esolangs.org/wiki/Baz
# language author: User:OriginalOldMan
# licence: CC0 (esolangs.org wiki content)
print Dir.pwd
print " $ "
$vars = {}
$c = 0
$vars ["baz"] = "baz"
$vars ["true"] = "true"
$vars ["false"] = "false"
file = File.open(gets.delete("\n"),"r")
$program = file.read
$program = $program.split("\n")
def tokenize(line)
tokens = []
if line.match(/.+=.+/)
       line = line.delete(" ")
       line = line.split("=")
       tokens[1] = line[0]
       tokens[0] = "setvar"
       tokens[2] = line[1]
else
       tokens = line.split(" ")
end
return tokens
end
def exe(line)
if line[0] == "setvar"
       if $vars[line[2]] == "true" or $vars[line[2]] == "false" or $vars[line[2]] == "baz"
       $vars[line[1]] = $vars[line[2]]
       else
       e = $c+1
       abort "YOU ARE WRONG!@#{e}"
       end
elsif line[0] == "show"
       if line.count == 2
       puts $vars[line[1]]
       else
       e = $c+1
       abort "YOU ARE WRONG!@#{e}"
       end
elsif line[0] == "get"
       if line.count == 2
       tvar = gets.delete("\n")
       if tvar == "true" or tvar == "false" or tvar == "baz"
              $vars[line[1]] = tvar
       else
       e = $c+1
       abort "YOU ARE WRONG!@#{e}"
       end
       end
elsif line[0] == "if"
       if line.count == 2
       x =  $vars[line[1]] == "baz" and rand(2) == 1
       if $vars[line[1]] == "false" or x
       l = $c
       b = 0
       while b == 0
       if $program[l] == "endif"
       b=1
       $c = l
       end
       l+=1
       end
       end
       else
       e = $c+1
       abort "YOU ARE WRONG!@#{e}"
       end
elsif line[0] == "if!"
       if line.count == 2
       x = $vars[line[1]] == "baz" and rand(2) == 1
       if $vars[line[1]] == "true" or x
       l = $c
       b = 0
       while b == 0
       if $program[l] == "endif"
       b=1
       $c = l
       end
       l+=1
       end
       end
       else
       e = $c+1
       abort "YOU ARE WRONG!@#{e}"
       end
elsif line[0] == "goto"
if line.count == 2
$c = line[1].to_i - 2
else
e = $c+1
abort("YOU ARE WRONG!@#{e}")
end
elsif line[0] == "end"
abort
elsif line[0] == "endif"
else
e = $c + 1
abort "YOU ARE WRONG!@#{e}"
end
end
def run(program)
while $c < program.count
exe(tokenize(program[$c]))
$c+=1
end
end
run($program)
