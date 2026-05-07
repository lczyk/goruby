# Number literal adversarial examples.

# Decimal integers
0
1
1234
1_234
1_2_3_4
100_000_000

# Leading-zero decimal -- in Ruby, 0-prefixed numbers are NOT octal unless 0o/0O
01234
09  # not valid octal, but lexer should handle

# Explicit decimal
0d123
0D456
0d1_234

# Octal -- both prefix styles
0o777
0O777
0o7_5_3

# Hexadecimal
0xDEAD_BEEF
0XFF
0xaa
0xAa
0xAA
0x1_2_3

# Binary
0b1010
0B0101
0b1010_0101
0b1111_0000_1010_0101

# Floats -- various formats
12.34
0.5
.5
5.0
1_2.3_4
1.234e5
1.234E-5
1234e-2
1.234E1
2.2_22

# Float with exponent and underscores
1_2.3_4e5_6

# Float with rational/complex suffix
5.5r
5.5i
1.2e3r
1.2e3i

# Integer with rational/complex suffix
5r
5i
0xFFr
0b1010i

# Edge cases -- numbers immediately before operators
5+3
5-3
5*3
5/3
5%3

# Numbers in method calls
foo(5, 3)
[1, 2, 3]

# Negative numbers (unary minus, not part of literal)
-5
-5.5
-0xFF

# Float edge -- trailing dot is method call, not float
5.to_s
5.5.to_s

# Complex literals with various number bases
0xDEAD_BEEFr
0o777i
0b1010r
