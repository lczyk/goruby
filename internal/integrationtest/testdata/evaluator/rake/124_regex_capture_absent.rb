# A capture group that doesn't participate in the match now sets the
# corresponding global ($1, $2, ...) to nil. Previously goruby surfaced
# the empty-string default from Go's regexp.FindStringSubmatch, which
# made `while $2` loops spin forever on optional trailing captures.

/a(b)?(c)?/ =~ "ac"
p $1                                        #=> nil
p $2                                        #=> "c"

/a(b)?(c)?/ =~ "abc"
p $1                                        #=> "b"
p $2                                        #=> "c"

# A do/while loop that exits when a tail capture is absent now
# terminates (was an infinite loop). Rake's parse_task_string uses
# this exact shape to tokenise `name[a,b,c]`.
$LOAD_PATH.unshift __dir__ + "/../../gems/rake/lib"
require "rake"
app = Rake::Application.new
name, args = app.parse_task_string("name[one,two,three]")
puts name                                   #=> name
p args                                      #=> ["one", "two", "three"]

# Pin downstream rake suite that previously hung.
$LOAD_PATH.unshift __dir__ + "/../../gems/minitest/lib"
require "minitest"
require "minitest/test"
require_relative __dir__ + "/../../gems/rake/test/helper.rb"
require_relative __dir__ + "/../../gems/rake/test/test_rake_task_argument_parsing.rb"

class Rep
  attr_reader :passed
  def initialize; @passed = 0; end
  def prerecord(*); end
  def record(r); @passed += 1 if r.passed?; end
end

rep = Rep.new
TestRakeTaskArgumentParsing.runnable_methods.sort.each do |n|
  begin
    Minitest::Runnable.run_one_method(TestRakeTaskArgumentParsing, n, rep)
  rescue Exception
  end
end
puts "TestRakeTaskArgumentParsing: p=#{rep.passed}"   #=> TestRakeTaskArgumentParsing: p=13
