# syntax introduced in ruby 2.0 (fails 1.9, passes 2.0+)
#
# boundary: 1.9 -> 2.0
#
# changes exercised:
#   - keyword arguments (optional only; required kwargs are 2.1)
#   - double-splat **kwargs
#   - %i / %I symbol array literals
#   - __dir__ keyword
#   - refinements: using / refine

# keyword arguments: optional
def optional_kw(a: 1, b: 2)
  [a, b]
end

# keyword arguments: with double splat
def kw_splat(a: 1, **rest)
  [a, rest]
end

# %i symbol array literal
a = %i[foo bar baz]
b = %i(one two three)
c = %i{x y z}

# %I symbol array literal (interpolation)
name = "world"
d = %I[hello_#{name} goodbye_#{name}]

# __dir__
_ = __dir__

# refinements
module StringExt
  refine String do
    def shout
      upcase + "!"
    end
  end
end

using StringExt
