# flip-flop operator -- a dark corner of Ruby.
# In conditional position (if / unless / while / until / ternary / modifier),
# `expr1..expr2` and `expr1...expr2` are *not* Range literals -- they are
# stateful flip-flop predicates. State turns on when expr1 is true and stays
# on until expr2 is true (inclusive `..`) or flips off on the same tick
# (exclusive `...`).
#
# Supported by every MRI 1.9 through 4.0. 2.6 emits a deprecation warning
# under `-w`; un-deprecated from 2.7 onward.

# ---- if block ----
i = 0
while i < 20
  if (i == 5)..(i == 10)
    puts "incl: #{i}"
  end
  if (i == 12)...(i == 15)
    puts "excl: #{i}"
  end
  i += 1
end

# ---- modifier if ----
j = 0
while j < 10
  puts "mod-if: #{j}" if (j == 3)..(j == 7)
  j += 1
end

# ---- modifier unless ----
k = 0
while k < 10
  puts "mod-unless: #{k}" unless (k == 2)..(k == 6)
  k += 1
end

# ---- unless block ----
m = 0
while m < 10
  unless (m == 1)..(m == 4)
    puts "unless-block: #{m}"
  end
  m += 1
end

# ---- ternary ----
n = 0
while n < 8
  s = ((n == 2)..(n == 5)) ? "in" : "out"
  puts "ternary #{n}: #{s}"
  n += 1
end

# ---- while predicate ----
p = 0
while (p == 0)..(p == 3)
  puts "while-pred: #{p}"
  p += 1
end

# ---- until predicate ----
q = 0
until (q == 2)...(q == 4)
  puts "until-pred: #{q}"
  q += 1
  break if q > 10
end

# ---- mixed endpoint expressions ----
arr = (1..15).to_a
arr.each do |x|
  if (x * 2 == 6)..(x.even? && x > 8)
    puts "mixed: #{x}"
  end
end

# ---- two flip-flops in one boolean expression ----
t = 0
while t < 20
  if ((t == 1)..(t == 3)) || ((t == 10)..(t == 12))
    puts "or-of-ff: #{t}"
  end
  t += 1
end

# ---- flip-flop in an else branch ----
u = 0
while u < 12
  if u < 0
    puts "neg"
  else
    if (u == 4)...(u == 8)
      puts "else-ff: #{u}"
    end
  end
  u += 1
end

# ---- flip-flop with begin/end body ----
v = 0
while v < 8
  if (v == 2)..(v == 5)
    begin
      puts "begin: #{v}"
    end
  end
  v += 1
end

# ---- negated flip-flop ----
w = 0
while w < 10
  if !((w == 3)..(w == 6))
    puts "neg-ff: #{w}"
  end
  w += 1
end
