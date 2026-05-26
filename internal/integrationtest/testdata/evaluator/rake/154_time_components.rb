# Time component accessors: year, month / mon, day / mday, hour, min,
# sec, wday, yday, usec, nsec. All return Integer.

t = Time.now
p t.year.is_a?(Integer)                     #=> true
p t.month.is_a?(Integer)                    #=> true
p t.mon == t.month                          #=> true
p t.day.is_a?(Integer)                      #=> true
p t.mday == t.day                           #=> true
p t.hour.is_a?(Integer)                     #=> true
p t.min.is_a?(Integer)                      #=> true
p t.sec.is_a?(Integer)                      #=> true
p t.wday.is_a?(Integer)                     #=> true
p t.yday.is_a?(Integer)                     #=> true
