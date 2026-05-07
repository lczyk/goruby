# Character literal and escape sequence adversarial examples.

# Simple character literals
?a
?b
?Z
?0
?9

# Escaped character literals
?\n
?\t
?\r
?\f
?\v
?\s
?\e
?\b
?\a

# Multi-char escape in character literal
?\C-x      # control-x
?\C-\M-x   # control-meta-x
?\M-x      # meta-x
?\M-\C-x   # meta-control-x (equivalent to \C-\M-x)
?\cX       # control-X (lowercase c variant)

# Unicode escape in character literal
?\u{2603}   # snowman
?\u{1F600}  # emoji

# Hex escape in character literal
?\x41       # 'A'
?\xFF       # 255

# Octal escape in character literal
?\o{101}    # 'A'
?\o40       # space (3 octal digits)

# Backslash-escaped punctuation in char literal
?\?
?\.
?\+

# Space as character literal
?\s
?\       # space after ? -- valid, produces " "

# String escape sequences
"\t tab \n newline \r return \f formfeed"
"\v vertical \b backspace \a bell \e escape \s space"
"\\ backslash \" double-quote"
"\x41 \x4A"            # hex escapes
"\u{2603}"              # unicode snowman
"\u{1F600 1F601}"       # multiple unicode chars
"\o{101} \o{102}"       # octal escape with braces
"\C-x \C-\\"            # control char, control-backslash
"\M-x \M-\\"            # meta char, meta-backslash
"\C-\M-x \M-\C-x"       # control-meta combinations
"\cX \c?"               # control with lowercase c
"\x41-\x5A"             # hex range in string

# Single-quoted string -- only \\ and \' are escapes
'can\'t escape here'
'backslash \\ only'

# Escape at end of input (lexer should survive)
# "unterminated \<NEWLINE> -- this is line continuation

# Backslash-newline in string (line continuation)
"hello \
world"

# Mixed escapes and interpolation
"tab \t, interp #{x}, unicode \u{2603}, hex \x41"

# Escaped newline in single-quoted string -- literal backslash then newline
'line \
still here'
