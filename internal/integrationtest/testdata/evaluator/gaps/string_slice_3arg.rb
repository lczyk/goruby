# String#slice / String#[] takes (regex, capture_group_index)
s = "hello world"
puts s.slice(/(\w+) (\w+)/, 1)
puts s.slice(/(\w+) (\w+)/, 2)
