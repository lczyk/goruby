# minversion: 3.1
# hash literal value omission `{ a: }`. introduced 3.1.
a = 1
b = 2
p({ a:, b: })    #=> {:a=>1, :b=>2}
