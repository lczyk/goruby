# minversion: 2.6
# Pin String#valid_encoding? real impl (UTF-8 validity).

puts "hello".valid_encoding?                  #=> true
puts "".valid_encoding?                       #=> true
# Valid 2-byte UTF-8 sequence (0xc3 0xa9 -> e-acute).
puts "h\xc3\xa9llo".valid_encoding?           #=> true
# Valid 3-byte UTF-8 sequence (0xe6 0x97 0xa5 -> Japanese sun char).
puts "\xe6\x97\xa5".valid_encoding?           #=> true
# Stray high bytes -- invalid.
puts "\xff\xfe".valid_encoding?               #=> false
# Half a multi-byte sequence -- invalid.
puts "\xc3\x28".valid_encoding?               #=> false
