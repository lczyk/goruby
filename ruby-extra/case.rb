# case/when expression edge cases

# case with complex when conditions (multi-value, splat)
x = 5
case x
when 1, 2, 3
  'small'
when 4, 5, 6
  'medium'
when *[7, 8, 9]
  'from splat'
else
  'other'
end

# case without condition expression (bare case -- like if/elsif chain)
case
when x == 1
  'one'
when x == 5
  'five'
else
  'other'
end

# case with then keyword
case x
when 1 then 'one'
when 2 then "two"
else 'other'
end

# case with semicolon separator
case x
when 1; 'one'
when 2; "two"
end

# case returning value
result = case x
when 1 then 'one'
when 2 then "two"
else 'other'
end

# case with assignment in condition expression
case y = compute_value
when 1
  'got 1'
when 2
  'got 2'
end

# case/in pattern matching (Ruby 2.7+/3.x syntax -- parser should handle)
case {a: 1, b: 2}
in {a: Integer => n}
  n
end
