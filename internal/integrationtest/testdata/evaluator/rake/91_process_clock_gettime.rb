# minversion: 2.6
# Pin Process.clock_gettime clock_id + unit handling.

# Default unit is float_second.
t = Process.clock_gettime(:CLOCK_MONOTONIC)
puts t.is_a?(Float)                           #=> true
puts t > 0                                    #=> true

# Realtime kind also returns Float.
puts Process.clock_gettime(:CLOCK_REALTIME).is_a?(Float)
                                              #=> true

# Unit options.
puts Process.clock_gettime(:CLOCK_MONOTONIC, :nanosecond).is_a?(Integer)
                                              #=> true
puts Process.clock_gettime(:CLOCK_MONOTONIC, :microsecond).is_a?(Integer)
                                              #=> true
puts Process.clock_gettime(:CLOCK_MONOTONIC, :millisecond).is_a?(Integer)
                                              #=> true
puts Process.clock_gettime(:CLOCK_MONOTONIC, :second).is_a?(Integer)
                                              #=> true
puts Process.clock_gettime(:CLOCK_MONOTONIC, :float_microsecond).is_a?(Float)
                                              #=> true

# Monotonic clock is non-decreasing.
t1 = Process.clock_gettime(:CLOCK_MONOTONIC)
t2 = Process.clock_gettime(:CLOCK_MONOTONIC)
puts t2 >= t1                                 #=> true
