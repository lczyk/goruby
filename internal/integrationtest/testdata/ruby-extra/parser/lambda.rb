# lambda expressions: stabby syntax edge cases

# basic forms
l1 = -> { 42 }
l2 = ->(x) { x * 2 }
l3 = ->(x, y) { x + y }
l4 = -> do
  1 + 2
end

# no parens
l5 = -> { :no_args }

# with default values
l6 = ->(x = 1, y = 2) { x + y }

# with splat
l7 = ->(*args) { args.length }

# with block capture
l8 = ->(&blk) { blk.call }

# with keyword rest
l9 = ->(**kwargs) { kwargs }

# empty body
l10 = -> { }

# lambda as method arg (to_proc conversion)
[1, 2, 3].map(&->(x) { x * 2 })

# lambda called immediately
->(x) { x * 2 }.call(5)

# nested lambdas (closure)
->(x) { ->(y) { x + y } }

