# range literal expressions -- inclusive and exclusive

# basic ranges
r1 = 1..10
r2 = 1...10

# range in case/when
x = 5
case x
when 1..3
  'low'
when 4..7
  'mid'
when 8..10
  'high'
end

# range with variables
lo = 1
hi = 10
r = lo..hi
r = lo...hi

# range as method arg (no parens)
def takes_range(r)
end
takes_range(1..5)
takes_range(1...5)

# range in array indexing
arr = [10, 20, 30, 40, 50]
arr[1..3]
arr[1...3]

# beginless range (right operand only)
..5
...5

# endless range -- only .. works (Ruby 2.6+); ... (exclusive-end) is
# not valid for endless ranges -- there is no end to exclude.
1..

