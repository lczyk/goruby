# Array#product: cartesian product of self with one or more other
# arrays. Array#combination(n): all unordered n-element subsets.
# Array#permutation(n): all ordered n-element arrangements. No-block
# forms used here -- block forms still need separate coverage.
p [1,2].product([3,4])                    #=> [[1, 3], [1, 4], [2, 3], [2, 4]]
p [1,2].product([3], [4,5])               #=> [[1, 3, 4], [1, 3, 5], [2, 3, 4], [2, 3, 5]]

p [1,2,3].combination(2).to_a             #=> [[1, 2], [1, 3], [2, 3]]
p [1,2,3,4].combination(3).to_a           #=> [[1, 2, 3], [1, 2, 4], [1, 3, 4], [2, 3, 4]]
p [1,2,3].combination(0).to_a             #=> [[]]

p [1,2,3].permutation(2).to_a             #=> [[1, 2], [1, 3], [2, 1], [2, 3], [3, 1], [3, 2]]
p [1,2,3].permutation.to_a                #=> [[1, 2, 3], [1, 3, 2], [2, 1, 3], [2, 3, 1], [3, 1, 2], [3, 2, 1]]
