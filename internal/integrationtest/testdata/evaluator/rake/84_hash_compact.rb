# minversion: 3.4
# Pin Hash#compact and Hash#transform_keys.

# compact drops nil-valued entries.
h = {a: 1, b: nil, c: 2, d: nil}
puts h.compact.inspect                        #=> {a: 1, c: 2}
# Original unchanged.
puts h.length                                 #=> 4

# Empty / no-nil cases.
puts({}.compact.inspect)                      #=> {}
puts({x: 1, y: 2}.compact.inspect)            #=> {x: 1, y: 2}

# transform_keys with block.
puts({a: 1, b: 2}.transform_keys { |k| k.to_s }.inspect)
                                              #=> {"a" => 1, "b" => 2}
puts({1 => "a", 2 => "b"}.transform_keys { |k| k * 10 }.inspect)
                                              #=> {10 => "a", 20 => "b"}
