# assignment edge cases: multi-assignment, splat, decomposition

# multi-assignment with splat
a, *b = 1, 2, 3
a, *b, c = 1, 2, 3, 4
*a, b = 1, 2, 3

# multi-assignment with trailing comma
a, = 1, 2, 3
a, b, = 1, 2

# nested multi-assignment (array decomposition)
(a, b), c = [[1, 2], 3]
(a, (b, c)) = [1, [2, 3]]

# assignment to index (single and two-arg forms)
arr = []
arr[0] = 1
arr[0, 2] = 3, 4

# assignment to various target types
@ivar = 1
$global = 2
@@cvar = 3
Const = 4

# setter method call
obj.x = 5
obj[] = 1, 2

# conditional assignment
x ||= compute_default
x &&= ensure_set

# assignment in expression context
puts (x = 5)
puts y = 6

# parallel assignment with mismatched arities
a, b = 1
a, b = 1, 2, 3
