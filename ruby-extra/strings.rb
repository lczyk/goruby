# string edge cases: concatenation, quotes, escaping

# automatic concatenation (adjacent string literals)
s1 = "hello " "world"
s2 = 'single ' 'quote ' 'concat'
s3 = "interp #{1}" " more"

# mixed quote styles concatenated
s4 = "double" 'single'
s5 = 'single' "double #{1+2}"

# empty strings (various forms)
s6 = ""
s7 = ''
s8 = %q()
s9 = %Q()
s10 = %()

# multiline strings with raw newlines
s11 = "real
newline"
s12 = 'also real
newline'

# string with hash-like content (not interpolation)
s13 = "# not interpolated"
s14 = "\#{also not interpolated}"

# string with escaped quotes
s15 = "it's \"quoted\""
s16 = 'it\'s \'quoted\''
s17 = %q(it's "quoted" with both)

# string with only escapes
s18 = "\x41\x42\x43"

# string indexing on literal
c = "hello"[1]
c = "hello"[1, 3]
c = "hello"[1..3]

# percent with unusual delimiter
s19 = %q%percent delimited%
