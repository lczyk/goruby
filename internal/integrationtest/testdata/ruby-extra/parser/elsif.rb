# elsif edge cases

# basic elsif chain
if x == 1
  1
elsif x == 2
  2
elsif x == 3
  3
else
  4
end

# elsif with then keyword
if x == 1 then
  1
elsif x == 2 then
  2
end

# elsif with semicolons
if x == 1; 1
elsif x == 2; 2
elsif x == 3; 3
end

# elsif without trailing else
if x == 1
  1
elsif x == 2
  2
end

# nested elsif chains
if a
  if b
    1
  elsif c
    2
  end
elsif d
  3
end

# return in elsif body
def classify(x)
  if x < 0
    :negative
  elsif x == 0
    :zero
  else
    :positive
  end
end

# break in elsif body (inside loop)
while true
  if cond1
    break :a
  elsif cond2
    break :b
  else
    break :c
  end
end
