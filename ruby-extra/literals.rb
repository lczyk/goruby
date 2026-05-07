# number and string literal edge cases

# number literals: hex/binary/octal with underscores
0xFF_FF_FF_FF
0b1010_0101
0o77
0O77
0_777

# decimal prefix
0d99
0D99

# float with exponents and underscores
1.5e+10
1.5E-10
2.2_22
1_000.500_25

# rational and complex suffixes
42r
42i
1.5i
1.5r

# character literals with various escapes
?a
?\n
?\t
?\e
?\s
?\C-a
?\M-a
?\C-\M-a
?\u{41}

# string escapes
"\a\b\e\f\n\r\s\t\v"
"\x41"
"\x41\xA"
"\u{1F600}"
"\C-a"
"\M-a"
"\C-\M-a"
"\ca"

# percent literals with different delimiters
%q|pipe literal|
%q!bang literal!
%q(paren literal)
%q[brace literal]
%q{curly literal}

%Q|interp #{1+2}|
%Q!bang #{1+2}!
%Q(paren #{1+2})
%Q{curly #{1+2}}

%w|word array|
%w!word array!
%w(paren words)
%w[brace words]
%w{curly words}

%i|sym array|
%i[symbol array]

%s|pipe symbol|
%s!bang symbol!
%s(paren sym)

%x|ls -la|
%x!whoami!
%x(echo hi)
