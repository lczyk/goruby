# Hash pattern with **rest capture. Roundtrip prints (**rest) with parens
# which doesn't re-parse as a valid hash pattern entry.
case x
in {a: a, **rest}
  rest
end
