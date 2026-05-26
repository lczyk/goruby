# Array#cycle, #chunk, #slice_when.
# Hash#min_by / #max_by no-block enumerator forms.

# cycle(n) -- iterate over self n times.
r = []
[1, 2].cycle(3) { |x| r << x }
p r                                       #=> [1, 2, 1, 2, 1, 2]

# cycle without arg + early break -- limit via break.
r2 = []
[10, 20].cycle do |x|
  r2 << x
  break if r2.size == 5
end
p r2                                      #=> [10, 20, 10, 20, 10]

# chunk -- group adjacent elements by block value.
p [1, 1, 2, 2, 3].chunk { |x| x }.to_a    #=> [[1, [1, 1]], [2, [2, 2]], [3, [3]]]

# slice_when -- split between adjacent elements where block returns true.
p [1, 2, 4, 5, 7].slice_when { |a, b| b - a > 1 }.to_a
#=> [[1, 2], [4, 5], [7]]

# Hash#min_by / max_by no-block forms.
p({a: 3, b: 1, c: 2}.min_by.to_a          #=> [[:a, 3], [:b, 1], [:c, 2]]
)
