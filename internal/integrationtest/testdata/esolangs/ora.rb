# Ora interpreter
# source: https://esolangs.org/wiki/Ora
# language author: User:zeotrope
# licence: CC0 (esolangs.org wiki content)
class TwoD
  attr_reader :prog, :len, :current, :bf
  attr_accessor :pos, :direction, :bpos, :buffer

  def initialize(string)
    @prog = string.split("\n").collect {|x| x.split("")}
    @len = @prog.length
    @direction = :right
    @bpos = 0
    @buffer = [0]
    @bf = []
    locate_start
    read_current
  end

  def gen_bf(i)
    @bf << i
  end

  def print_bf
    puts ""
    @bf.each {|x| print x}
    puts ""
  end

  def locate_start
    @pos = [0, 0]
    (0..@len-1).each do |i|
      case
      when @prog[i].include?("$") then
        @pos = [i, @prog[i].index("$")]
      when @prog[i].include?("u") then
        @pos = [i, @prog[i].index("u")]
        @direction = :up
      when @prog[i].include?("d") then
        @pos = [i, @prog[i].index("d")]
        @direction = :down
      when @prog[i].include?("l") then
        @pos = [i, @prog[i].index("l")]
        @direction = :left
      when @prog[i].include?("r") then
        @pos = [i, @prog[i].index("r")]
        @direction = :right
      end
    end
  end

  def read_current
    @current = @prog[@pos[0]][@pos[1]]
  end

  def buffer_right
    if (@bpos += 1) >= @buffer.length
      @buffer << 0
    end
    gen_bf(">")
  end

  def buffer_left
    if (@bpos -= 1) < 0
      @buffer = [0]+@buffer
    end
    gen_bf("<")
  end

  def step
    case @direction
    when :up    then @pos[0] -= 1
    when :down  then @pos[0] += 1
    when :right then @pos[1] += 1
    when :left  then @pos[1] -= 1
    end
    read_current
    eval_cmd
  end

  def eval_spaces
    case @direction
    when :up    then buffer_right
    when :down  then buffer_left
    when :left  then @buffer[@bpos] -= 1; gen_bf("-")
    when :right then @buffer[@bpos] += 1; gen_bf("+")
    end
  end

  def eval_cmd
    case @current
    when "/" then
      case @direction
      when :up    then @direction = :right
      when :down  then @direction = :left
      when :left  then @direction = :down
      when :right then @direction = :up
      end
    when "\\" then
      case @direction
      when :up    then @direction = :left
      when :down  then @direction = :right
      when :left  then @direction = :up
      when :right then @direction = :down
      end
    when "C" then
      case @direction
      when :up    then @direction = :right
      when :down  then @direction = :left
      when :left  then @direction = :up
      when :right then @direction = :down
      end
    when "A" then
      case @direction
      when :up    then @direction = :left
      when :down  then @direction = :right
      when :left  then @direction = :down
      when :right then @direction = :up
      end
    when "X" then
      if @buffer[@bpos] != 0
        @current = "\\"
      else
        @current = "/"
      end
      eval_cmd
    when "x" then
      if @buffer[@bpos] != 0
        @current = "/"
      else
        @current = "\\"
      end
      eval_cmd
    when "u" then @direction = :up
    when "d" then @direction = :down
    when "r" then @direction = :right
    when "l" then @direction = :left
    when "|" then
        case @direction
        when :left  then @direction = :right
        when :right then @direction = :left
        end
    when "-" then
      case @direction
      when :up    then @direction = :down
      when :down  then @direction = :up
      end
    when ".", " " then eval_spaces
    when "@" then disp; print_bf; exit(1)
    end
  end

  def disp
    puts "\tbuffer #{$test.buffer}\tpos #{$test.pos}\tcurrent #{$test.current}\tdirection #{$test.direction}"
  end

  def run
    disp
    step
  end

  def print_prog
    (0..@len-1).each do |i|
      (0..@prog[i].length).each do |j|
        if ([i, j] == @pos)
          print "\033[44;37;5m#{@current} \033[0m"
        else
          print @prog[i][j]
        end
      end
      print "\n"
    end
    STDOUT.flush
    sleep(0.3)
    system("clear")
  end
end

$test = TwoD.new(File.read(ARGV[0]))
$steps = ARGV[1]
system("clear")

if $steps.nil?
  loop do
    $test.print_prog
    $test.disp
    $test.step
  end
else
  $steps.to_i.times do |x|
    $test.print_prog
    $test.disp
    $test.step
  end
end
