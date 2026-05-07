# CopyPasta Language interpreter
# source: https://esolangs.org/wiki/CopyPasta_Language/implementation.rb
# language author: User:Rerednaw
# impl author: User:Conor O'Brien
# licence: CC0 (esolangs.org wiki content)

def fatal_error(error, code=1)
    STDERR.puts "CopyPasta Language: #{error}"
    exit! code
end

def copy_pasta(program, split_semi_colons=false)
    lines = split_semi_colons ? program.split(";") : program.lines
    clipboard = ""
    until lines.empty?
        line = lines.shift
        case line.chomp
            when "Copy"
                arg = lines.shift
                fatal_error "No lines to Copy" if arg.nil?
                clipboard = arg
            when "CopyFile"
                arg = lines.shift
                fatal_error "No file name to CopyFile" if arg.nil?
                clipboard = if arg.chomp == "TheFuckingCode"
                    program
                else
                    fatal_error "No such file `#{arg}`" unless File.exists? arg
                    File.read arg
                end
            when "Duplicate"
                arg = lines.shift
                fatal_error "Invalid number to Duplicate by: #{arg}" unless /^\d+\s*$/ === arg
                clipboard = clipboard * (1 + arg.to_i)
            when "Pasta!"
                return clipboard
            else
                fatal_error "Invalid instruction: #{line}"
        end
    end
    return ""
end

program = ARGV[0]
split_semi_colons = false

if program == "-e" || program.downcase == "/e"
    program = ARGV[1]
    split_semi_colons = true
else
    fatal_error "Files must have the .copypasta extension" unless program.end_with? ".copypasta"
    begin
        program = File.read program
    rescue Errno::ENOENT
        fatal_error "No such file #{program}"
    end
end

print copy_pasta program, split_semi_colons
