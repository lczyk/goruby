# minversion: 2.6
# Pin Tempfile real fs backing -- block-form opens a real temp
# file, puts buffer, flush writes to disk, block return unlinks.

require "tempfile"

# Block form: write + read via path.
content_seen = nil
Tempfile.open("test") do |t|
  t.puts "first line"
  t.puts "second line"
  t.flush
  content_seen = File.read(t.path)
end
puts content_seen.lines.length              #=> 2
puts content_seen.include?("first line")    #=> true

# Path follows the prefix.
Tempfile.open("greet") do |t|
  puts t.path.include?("greet")             #=> true
end

# Mock minitest's diff-fallback pattern.
expect = "alpha\nbeta\n"
actual = "alpha\ngamma\n"
diff_out = ""
Tempfile.open("expect") do |a|
  a.puts expect
  a.flush
  Tempfile.open("butwas") do |b|
    b.puts actual
    b.flush
    diff_out = File.read(a.path) + "||" + File.read(b.path)
  end
end
puts diff_out.include?("alpha")             #=> true
puts diff_out.include?("gamma")             #=> true
