# syntax introduced in ruby 2.6 (fails 2.5, passes 2.6+)
#
# boundary: 2.5 -> 2.6
#
# changes exercised:
#   - endless ranges: (1..)
#   - non-ascii constant names

# endless range (no end value)
a = (1..)
b = (0..)
c = ("a"..)

# endless range in case/when
case 50
when (1..10)
  :low
when (11..49)
  :mid
when (50..)
  :high
end

# endless range with step
_ = (1..).lazy.select(&:odd?).first(5)

# endless range in array slice
arr = [1, 2, 3, 4, 5]
_ = arr[2..]

# non-ascii constant names
Ω = 42
Σ = ->(x) { x + 1 }
