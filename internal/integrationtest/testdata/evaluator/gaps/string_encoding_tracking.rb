s = "abc"
puts s.encoding
puts s.force_encoding("ASCII-8BIT").encoding
puts s.force_encoding("US-ASCII").encoding

bytes = "\xC3\xA9".dup.force_encoding("ASCII-8BIT")
puts bytes.encoding
puts bytes.bytesize
