# minversion: 3.4
# Pin Marshal.dump / load real round-trip for primitives.
# Format isn't MRI-compatible but goruby-internal round-trip works.

# Primitives.
[nil, true, false, 0, 1, -1, 12345, 0.5, 3.14, "hello", "", :sym, :foo].each do |v|
  raw = Marshal.dump(v)
  back = Marshal.load(raw)
  raise "fail #{v.inspect}" unless back == v
end
puts "primitives ok"                          #=> primitives ok

# Arrays + nested.
a = [1, "two", :three, [4, 5], nil]
raw = Marshal.dump(a)
puts Marshal.load(raw).inspect                #=> [1, "two", :three, [4, 5], nil]

# Hashes.
h = {a: 1, b: "two", c: [3, 4]}
raw = Marshal.dump(h)
puts Marshal.load(raw).inspect                #=> {a: 1, b: "two", c: [3, 4]}

# Round-trip identity (== not equal?).
v = [1, 2, 3]
roundtripped = Marshal.load(Marshal.dump(v))
puts roundtripped == v                        #=> true
puts roundtripped.equal?(v)                   #=> false
