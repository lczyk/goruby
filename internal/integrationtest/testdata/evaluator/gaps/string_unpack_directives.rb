puts "\x00\x00\x00\x2A".unpack("N").inspect
puts "\x2A\x00".unpack("v").inspect
puts "\x00\x00\x2A\x00".unpack("V").inspect
puts "hello".unpack("H*").inspect
