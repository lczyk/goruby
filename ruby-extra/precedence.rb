# operator precedence edge cases: and/or/not vs &&/||/!

# and/or/not vs &&/||/! (different precedence levels)
a = true and false  # (a = true) and false -- a gets true
b = true && false    # b = (true && false) -- b gets false
c = (not true)       # parens needed: not has very low precedence
d = !true            # d = (!true)

# rescue modifier precedence
x = may_fail rescue fallback
y = may_fail || fallback

# modifier if/unless with complex logical expressions
do_something if x && y || z
do_other unless a == b rescue nil

# begin/end with modifier
begin
  1
end if true

# ternary nesting (right-associative)
a = b ? (c ? d : e) : (f ? g : h)
a = b ? c ? d : e : f ? g : h

# unary operators
!x
not x
-x
+x

# method call without parens chaining
foo.bar baz, qux
foo.bar(baz).qux
foo.bar baz, foo.bar(qux)

# complex infix chaining
a < b && c > d || e == f && g != h

# mix of and/or with &&/||
result = a && b or c || d
