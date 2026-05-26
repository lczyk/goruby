# minversion: 2.6
# Exercise the Process module shim. rake and minitest both use
# Process.pid for log tagging and Process.clock_gettime for
# elapsed-time reporting.

puts Process.pid.is_a?(Integer)                 #=> true
puts Process.pid > 0                            #=> true

# Repeated calls return the same pid -- the process doesn't fork.
p1 = Process.pid
p2 = Process.pid
puts p1 == p2                                   #=> true

# clock_gettime returns a Float that monotonically increases.
t1 = Process.clock_gettime(:any_clock_id)
t2 = Process.clock_gettime(:any_clock_id)
puts t1.is_a?(Float)                            #=> true
puts t2 >= t1                                   #=> true
