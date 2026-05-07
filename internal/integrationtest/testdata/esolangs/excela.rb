# excela interpreter
# source: https://esolangs.org/wiki/Excela
# licence: CC0 (esolangs.org wiki content)

#!/usr/bin/ruby
include Math
class Inputer
  def initialize() @ar=[] end
  def [](a) (@ar<<STDIN.getc while a>=@ar.size ); @ar[a] end
end

class Outputer
  def initialize() @ar=[] ; @op=0 end
  def [](a) @ar[a] end
  def []=(a,v) raise "CRITICAL ERROR trying to ReWrItE oUtPuT" if a<@op
    @ar[a]=v;((STDOUT.putc @ar[@op];@op+=1)while @ar[@op]) end
end

value=eval(IO.read(ARGV[1]))
Ring=binding
Rulez=IO.read(ARGV[0]).split("\n")
Input=Inputer.new
Output=Outputer.new
change=0

while change
  change=false
  Rulez.each(){|r|
    begin
      raise "CRUCIAL ERROR -not assigned" if !(/(.*[^!=<>])=[^=]/===r)
      rb=eval("["+$1+"]",Ring)
      eval(r,Ring)
      change||=(eval("["+$1+"]",Ring)!=rb)
    rescue
      puts "something bad with #{r}"
    end
  }
end
