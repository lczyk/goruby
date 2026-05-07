# Alarm Clock Radio interpreter
# source: https://esolangs.org/wiki/Alarm_Clock_Radio/implementation.rb
# language author: User:Deschutron
# impl author: User:Conor O'Brien
# licence: CC0 (esolangs.org wiki content)

#!/usr/bin/ruby
require 'optparse'

# because Notepad++ freaks out with "%="
def clamp(val, max)
    val % max
end

class AlarmClockRadio
    OP_HR       = ">"
    OP_MIN      = "+"
    OP_LOOP     = "["
    OP_RETZ     = "]"
    OP_READ     = ","
    OP_WRITE    = "."
    OP_SNOOZE   = "@"
    OP_PAD      = "}"
    def AlarmClockRadio.tokens(code)
        code.scan(/((\[+.>,@\])\2*|\[\[\]\})/).map(&:first)
    end

    def initialize(program, padsize = 60, init_pad = 60, cell_max = 256)
        @memory = [0] * init_pad
        @padsize = padsize
        @code = AlarmClockRadio::tokens program
        @ptr = 0
        @i = 0
        @cell_max = cell_max
        @loops = {}

        preprocess_loops
    end

    def preprocess_loops
        locs = []
        (0...@code.size).each { |i|
            op = @code[i]
            if op == OP_LOOP
                locs << i
            elsif op == OP_RETZ
                src = locs.pop
                @loops[src] = i
                @loops[i] = src - 1
                # to compensate for @i += 1
            end
        }
    end

    def pad
        left = @memory.index &:nonzero?
        right = @memory.rindex &:nonzero?
        dist = right - left rescue 0
        dist += 2
        # inclusive distance??
        @memory = @memory[0...dist]
        @memory.concat [0] * @padsize
    end

    def step
        cur = @code[@i]
        head = cur[0]
        repetend = cur.size

        case head
            when OP_HR
                @ptr += repetend
                @ptr = clamp @ptr, @memory.size

            when OP_MIN
                @memory[@ptr] += repetend
                @memory[@ptr] = clamp @memory[@ptr], @cell_max

            when OP_LOOP
                if @memory[@ptr].zero?
                    @i = @loops[@i]
                end

            when OP_RETZ
                unless @memory[@ptr].zero?
                    @i = @loops[@i]
                end

            when OP_READ
                repetend.times {
                    @memory[@ptr] = STDIN.getc.ord
                }

            when OP_WRITE
                STDOUT.putc @memory[@ptr] * repetend

            when OP_SNOOZE
                sleep @memory[@ptr] * repetend

            when OP_PAD
                pad
        end

        @i += 1
    end

    def running?
        @i < @code.size
    end

    def run
        step while running?
    end

    def debug
        p @memory
    end
end

options = {
    init_cell_size: 60,
    init_pad_size: 60,
    arguments: false,
    cell_max: 256,
    debug: false,
}
NAME = File.basename __FILE__
parser = OptionParser.new { |opts|
    opts.banner = "Usage: #{NAME} [options]"

    opts.on("-O", "--out-cell-size=INT", Integer, "Set initial cell size (PAD)") { |v|
        options[:init_cell_size] = v
    }

    opts.on("-o", "--out-pad-size=INT", Integer, "Set initial pad size (PADSIZE)") { |v|
        p v
        options[:init_pad_size] = v
    }

    opts.on("-m", "--max=INT", Integer, "Set cell max (default: 256)") { |v|
        options[:cell_max] = v
    }

    opts.on("-a", "--arguments", "Read from command line arguments instead") { |v|
        options[:arguments] = v
    }

    opts.on("-d", "--debug", "Debugs final state") { |v|
        options[:debug] = v
    }
}
parser.parse!

if ARGV.empty?
    STDERR.puts parser
    exit
end

source = ARGV.first

if options[:arguments]
    source = ARGV.join
else
    source = File.read source
end

a = AlarmClockRadio.new(
    source,
    options[:init_pad_size],
    options[:init_cell_size],
    options[:cell_max],
)
a.run
a.debug if options[:debug]
