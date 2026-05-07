# unusual method call patterns

# call on literal
"hello".upcase
42.to_s
[1,2,3].length

# call with block on literal
[1,2,3].map { |x| x * 2 }
[1,2,3].map do |x|
  x * 2
end

# chained calls with newlines between dots
result = object
  .method1
  .method2(arg1, arg2)
  .method3 { |x| x }

# safe navigation (&.) chained
obj&.first&.upcase&.strip

# call with no args but parens
foo()

# call with trailing comma in args
foo(1, 2, 3,)

# call with splat
foo(*args)
foo(1, *args, 2)

# call with double splat
foo(**kwargs)

# method on self with implicit receiver
self.foo
self.class.new

# scoped call
Foo::Bar.baz
::TopLevel.method

# call on global, ivar, cvar
$stdout.puts "hi"
@obj.method
@@obj.method

# call with complex block params
method_with_block { |a, *b, c:, d: 1, &blk| }

# setter call on context
obj.x = 5
self.value = 42
