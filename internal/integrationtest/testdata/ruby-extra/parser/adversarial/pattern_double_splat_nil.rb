# Hash pattern with **nil (no-keywords marker; matches only when hash has
# no other keys). Roundtrip prints (**nil) with parens, breaking re-parse.
case x
in {**nil}
  true
end
