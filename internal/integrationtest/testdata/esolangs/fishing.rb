# Fishing interpreter
# source: https://esolangs.org/wiki/Fishing
# language author: User:OriginalOldMan
# licence: CC0 (esolangs.org wiki content)
$pcx = 0
$pcy = 0
$castlen = 0
$stack = [""]
$dir = 0
$movdir = 0
$textmode = false
$cell = 0
def run(program)
e = false
while !e
if program[$pcy][$pcx] == "v"
$dir = 3
elsif program[$pcy][$pcx] == "_"
$movdir = 3
elsif program[$pcy][$pcx] == "|"
$movdir = 1
elsif program[$pcy][$pcx] == "["
$movdir = 0
elsif program[$pcy][$pcx] == "]"
$movdir = 2
elsif program[$pcy][$pcx] == "?"
if $stack[$cell] == $stack[$cell+1]
if program[$pcy-1][$pcx] == "="
$movdir = 1
elsif program[$pcy][$pcx+1] == "="
$movdir = 0
elsif program[$pcy][$pcx-1] == "="
$movdir = 2
elsif program[$pcy+1][$pcx] == "="
$movdir = 3
end
else
if program[$pcy][$pcx+1] == "!"
$movdir = 0
elsif program[$pcy][$pcx-1] == "!"
$movdir = 2
elsif program[$pcy+1][$pcx] == "!"
$movdir = 3
elsif program[$pcy-1][$pcx] == "!"
$movdir = 1
end
end
elsif program[$pcy][$pcx] == "^"
$dir = 1
elsif program[$pcy][$pcx] == ">"
$dir = 0
elsif program[$pcy][$pcx] == "<"
$dir = 2
elsif program[$pcy][$pcx] == "+"
$castlen+=1
elsif program[$pcy][$pcx] == "-"
$castlen -= 1
elsif program[$pcy][$pcx] == "C"
if $textmode == true
if $dir == 0
if program[$pcy][$pcx+$castlen] == "`"
$textmode = false
else
$stack[$cell] += program[$pcy][$pcx+$castlen]
end
elsif $dir == 1
if program[$pcy-$castlen][$pcx] == "`"
$textmode = false
else
$stack[$cell] += program[$pcy-$castlen][$pcx]
end
elsif $dir == 2
if program[$pcy][$pcx-$castlen] == "`"
$textmode = false
else
$stack[$cell] += program[$pcy][$pcx-$castlen]
end
elsif $dir == 3
if program[$pcy+$castlen][$pcx] == "`"
$textmode = false
else
$stack[$cell] += program[$pcy+$castlen][$pcx]
end
end
else
x = ""
if $dir == 0
x = program[$pcy][$pcx+$castlen]
elsif $dir == 1
x =   program[$pcy-$castlen][$pcx]
elsif $dir == 2
x =  program[$pcy][$pcx-$castlen]
elsif $dir == 3
x =  program[$pcy+$castlen][$pcx]
end
if x == "P"
print $stack[$cell]
elsif x == "c"
$stack << $stack[$cell].to_s + $stack[$cell+1].to_s
elsif x == "s"
n = $stack[$cell].to_s.split(//)
lcount = 0
while lcount < n.count
$stack << n[lcount]
lcount+=1
end
elsif x == "l"
$stack << $stack.count
elsif x == "N"
puts $stack[$cell]
elsif x == "`"
$textmode = true
elsif x == "E"
$stack[$cell] = ""
elsif x == "r"
$stack[$cell] = $stack[$cell].reverse
elsif x == "f"
$stack[$cell] = -$stack[$cell]
elsif x == "n"
$stack[$cell] = $stack[$cell].to_f
elsif x == "i"
$stack[$cell]+=1
elsif x == "d"
$stack[$cell]-=1
elsif x == "I"
$stack[$cell] = gets.delete("\n")
elsif x == "S"
$stack[$cell] = $stack[$cell]*$stack[$cell]
elsif x == "p"
$stack[$cell] = $stack[$cell]**$stack[$cell+1]
elsif x == "a"
$stack[$cell]+=$stack[$cell+1]
elsif x == "s"
$stack[$cell]-=$stack[$cell+1]
elsif x == "m"
$stack[$cell]*=$stack[$cell+1]
elsif x == "D"
$stack[$cell]/=$stack[$cell+1]
elsif x == "{"
$cell+=1
if $stack[$cell] == nil
$stack[$cell] = ""
end
elsif x == "}"
$cell-=1
if $stack[$cell] == nil
$stack[$cell] = ""
end
end
end
elsif program[$pcy][$pcx] == "D"
elsif program[$pcy][$pcx] == "="
elsif program[$pcy][$pcx] == "!"
else
puts "Ended at #{$pcy},#{$pcx}"
abort
end
if $movdir == 0
$pcx+=1
elsif $movdir == 1
$pcy-=1
elsif $movdir == 2
$pcx-=1
elsif $movdir == 3
$pcy+=1
end
if  $pcy >= program.count
e = true
end
end
end
run(File.open(gets.delete("\n"),"r").read.split("\n"))
puts "Ended at #{$pcy},#{$pcx}"
