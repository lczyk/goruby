# (run via: echo "a\nb\nc" | ruby this.rb)
e = ARGF.each_line
puts e.class
e.each { |line| print line }
