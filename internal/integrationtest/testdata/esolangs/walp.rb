#!/usr/bin/ruby
# Interpreter by Jay Campbell 2008
#  for the WALP esoteric programming language
# Usage: ./walp.rb hihi.walp

$verbose = true          # not much to see otherwise
$sleep = 0.1             # pause between ticks
$clear_screen = 'clear'  # for dos/windows use 'cls'

class Walp

    def initialize
        @grid = Array.new
        @pool = 0
        @start = nil
        @direction = 'right'
        @output = ''
        @biggest = 0
        @rows = 0
    end

    def turn_from(direction, count)
        dirs = ['right','down','left','up','right','down']
        current = dirs.index(direction)
        return dirs[dirs.index(direction) + count]
    end

    def parse( input )

        # measure and store input lines
        input.each do |line|
            parts = line.split(//)
            size = parts.size
            @biggest = size if size > @biggest
            if dollar = parts.index('$') then @pointer = [@rows, dollar] end
            @grid << parts
            @rows += 1
        end
        return 'No starting point.' unless @pointer

        # fill truncated lines
        @grid.each do |line| line << ' ' while @biggest > line.size end

        # run
        loop do
                  @here = @grid[@pointer[0]][@pointer[1]]

               if @here == '!' then break
            elsif @here == '?' then @pool = getc
            elsif @here == '#'
                   if @direction == 'left' then @pool -=1
                elsif @direction == 'right' then @pool += 1
                elsif @direction == 'down' then @output << @pool.chr
                elsif @direction == 'up' then @pool = 0
                  end
            elsif @here == '@' then @direction = turn_from(@direction, 1)
            elsif @here == '/' or
                ((@here ==  'o' or @here == 243.chr) and @pool == 0) or
                ((@here == 'O' or @here == 242.chr) and @pool != 0)
                     @direction = turn_from(@direction, 2)
            end

               if @direction == 'right' then @pointer[1] += 1
            elsif @direction == 'left'  then @pointer[1] -= 1
            elsif @direction == 'up'    then @pointer[0] -= 1
            elsif @direction == 'down'  then @pointer[0] += 1
              end

            @pointer[0] += @biggest if @pointer[0] < 0
            @pointer[0] -= @biggest if @pointer[0] >= @biggest
            @pointer[1] += @rows    if @pointer[1] < 0
            @pointer[1] -= @rows    if @pointer[1] >= @rows
            @pool -= 255 if @pool > 255
            @pool += 255 if @pool < 0

            if $verbose
                system($clear_screen)
                c = 0
                @grid.each do |line|
                    row = line.join
                    if c == @pointer[0] then row[@pointer[1]] = 'X' end
                    c += 1
                    puts row
                end
                puts "Grid:#{@biggest}x#{@rows} Pointer: #{@pointer[0]},#{@pointer[1]}='#{@here}' Pool:#{@pool} Output:#{@output}"
                sleep $sleep
            end
        end

        return @output
    end

end

file = ARGV[0]
input = Array.new
File.open(file, 'r') do |f| while not f.eof? do input << f.gets.chomp end end

walp = Walp.new
print walp.parse(input)

