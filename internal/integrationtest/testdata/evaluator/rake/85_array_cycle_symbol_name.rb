# minversion: 2.6
# Pin Array#cycle no-block, Symbol#name, Enumerator#first / #take.

# Array#cycle(n) -- materialise n repeats.
puts [1, 2].cycle(3).inspect                  #=> [1, 2, 1, 2, 1, 2]
puts [1, 2, 3].cycle(2).inspect               #=> [1, 2, 3, 1, 2, 3]
puts [].cycle(5).inspect                      #=> []
puts [1, 2].cycle(0).inspect                  #=> []

# Array#cycle (no arg) returns an Enumerator over a finite buffer;
# .first(n) and .take(n) work for moderate n.
puts [1, 2, 3].cycle.first(7).inspect         #=> [1, 2, 3, 1, 2, 3, 1]
puts [1, 2].cycle.take(5).inspect             #=> [1, 2, 1, 2, 1]

# Symbol#name is the symbol's underlying String.
puts :hello.name                              #=> hello
puts :hello.name.class                        #=> String
