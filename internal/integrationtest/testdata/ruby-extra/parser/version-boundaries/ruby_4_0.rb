# syntax introduced in ruby 4.0 (fails 3.x, passes 4.0+)
#
# boundary: 3.4 -> 4.0
#
# changes exercised:
#   - leading logical operators as line continuation
#     (||, &&, and, or at start of line continue previous expression)

# --- leading || as line continuation ---
a = nil
b = a
|| 42

# --- leading && as line continuation ---
c = true
&& false

# --- leading `or` as line continuation ---
d = nil
or 42

# --- leading `and` as line continuation ---
e = true
and false

# --- chained leading || ---
f = nil
|| nil
|| nil
|| 99
