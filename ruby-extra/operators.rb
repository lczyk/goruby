# bitwise, shift, pow, case-equality, pattern-match operators
# these operators exist as tokens but have limited parser test coverage

# bitwise AND, OR, XOR
a = 0xff & 0x0f
b = 0xf0 | 0x0f
c = 0xff ^ 0x0f
d = ~42

# shift left, shift right
e = 1 << 4
f = 256 >> 4

# pow
g = 2 ** 8

# case equality ===
result = (1..10) === 5
result = String === "hello"
result = /foo/ === "foobar"

# pattern match =~
re = /hello/ =~ "hello world"
re = "hello world" =~ /hello/

# not-match !~
nm = /foo/ !~ "hello"
nm = "hello" !~ /foo/

# combined assignment operators (extended set)
x = 1
x **= 2
x &= 0xff
x |= 0x0f
x ^= 0xff
x <<= 2
x >>= 2
x ||= "default"
x &&= "must be set"
