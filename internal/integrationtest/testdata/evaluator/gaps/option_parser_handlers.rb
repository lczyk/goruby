require 'optparse'

options = {}
parser = OptionParser.new do |opts|
  opts.banner = "Usage: test [opts]"
  opts.on("-v", "--verbose", "be verbose") do
    options[:verbose] = true
  end
  opts.on("-n NAME", "--name=NAME", "set name") do |n|
    options[:name] = n
  end
end

argv = ["-v", "--name=alice", "rest"]
parser.parse!(argv)
puts options[:verbose]
puts options[:name]
puts argv.inspect
