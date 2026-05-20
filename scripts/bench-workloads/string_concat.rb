# String concat + interpolation. Builds N strings of ~20 fragments
# each so per-string and per-fragment allocation both show.
10000.times do
  buf = ""
  30.times do |i|
    buf = buf + "i=#{i};"
  end
  buf.length
end
