# Arsel interpreter
# source: https://esolangs.org/wiki/Arsel
# language author: User:Kirbyiseatinghumanmeat
# licence: CC0 (esolangs.org wiki content)
class Arsel
 def initialize(script)
   @script = script
   @pointer = 0
   @board = [
     # Alphabetical chars
     "a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k",
     "l", "m", "n", "o", "p", "q", "r", "s", "t", "u", "v",
     "w", "x", "y", "z",
     # Numbers
     "1", "2", "3", "4", "5", "6", "7", "8", "9", "0",
     " "
   ]
   @pos = 0
 end
 def run()
   while @pos < @script.length
     current = @script[@pos]
     if current == "+"
       @pointer += 1
       @pos += 1
     elsif current == "0"
       print @board[@pointer]
       @pos += 1
     elsif current == "<"
       @pointer = 0
       @pos += 1
     else @pos += 1
     end
   end
 end
end
arsel = Arsel.new(File.read(ARGV[0]))
arsel.run
