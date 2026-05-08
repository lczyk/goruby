# for, until, modifier loops, and jump expressions

# for loop -- all forms
for i in [1, 2, 3] do
  puts i
end

for i in 1..5
  break if i > 3
  puts i
end

# for with multiple variables
for a, b in [[1,2],[3,4]]
  puts "#{a}, #{b}"
end

# until loop
x = 0
until x == 5
  x += 1
end

# until modifier
x += 1 until x == 10

# while modifier
x -= 1 while x > 0

# until with begin block
begin
  x += 1
end until x == 20

# while with begin block
begin
  x -= 1
end while x > 0

# jump expressions: break, next, redo, retry
result = [1,2,3].each do |n|
  break n if n == 2
end

result = [1,2,3].map do |n|
  next n * 2 if n == 2
  n
end

# retry (syntax-only test -- never actually retries)
begin
  raise "test"
rescue
  retry if false
end

# bare break, next, redo, retry
break
next
redo
retry
