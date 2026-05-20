# Hash construction + each_pair iteration + lookup. Hashes are
# allocated fresh per loop so the GC work shows in the timing.
50000.times do
  h = { a: 1, b: 2, c: 3, d: 4, e: 5 }
  total = 0
  h.each_pair { |_k, v| total += v }
  total + h[:c]
end
