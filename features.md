# supported features

reference checklist of ruby syntax recognised by the lexer + parser. an
`[x]` means an `*ast.<Node>` is produced for the construct; `[ ]` means
the parser errors or otherwise fails to round-trip the source.

scope is **front-end only** -- after the interpreter / evaluator removal
this fork tracks lexing and parsing, not runtime semantics. anything that
is purely a runtime concern (method dispatch, object-model invariants,
hash-key equality, etc.) is intentionally absent.

derived from the original goruby README; trimmed and re-framed for the
parser-library scope. flip checkboxes as features land in `parser/`.

## syntax

- [x] functions
	- [x] with parens
	- [x] without parens
	- [x] return keyword
	- [x] default values for parameters
	- [x] keyword arguments
	- [x] block arguments
	- [x] hash as last argument without braces
- [x] function calls
	- [x] with parens
	- [x] without parens
	- [x] with block arguments
- [x] conditionals
	- [x] if
	- [x] if/else
	- [x] if/elsif/else
	- [x] ternary `? :`
	- [x] unless
	- [x] unless/else
	- [x] case/when
- [x] short-circuit operators (`||`, `&&`)
- [x] control flow
	- [x] for loop
	- [x] while loop
	- [x] until loop
	- [x] break
	- [x] next
	- [x] redo
	- [ ] flip flop

## literals

- [x] integers
	- [x] decimal `1234`
	- [x] underscores `1_234`
	- [x] explicit decimal `0d170`, `0D170`
	- [x] octal `0252`, `0o252`, `0O252`
	- [x] hexadecimal `0xaa`, `0xAa`, `0xAA`, `0Xaa`, `0XAa`, `0XaA`
	- [x] binary `0b10101010`, `0B10101010`
- [x] floats
	- [x] `12.34`
	- [x] `1234e-2`
	- [x] `1.234E1`
	- [x] underscores `2.2_22`
- [x] booleans (`true`, `false`)
- [x] nil
- [x] strings
	- [x] double quoted
	- [x] single quoted
	- [x] character literals (`?\n`, `?a`, ...)
	- [x] `%q{}`
	- [x] `%Q{}`
	- [x] heredoc
		- [x] `<<EOF`
		- [x] indented `<<-EOF`
		- [x] squiggly `<<~`
		- [x] quoted heredoc (single, double, backtick)
	- [x] escaped characters (full `\a` ... `\C-?` set)
	- [x] interpolation `#{}`
	- [x] automatic concatenation
- [x] arrays
	- [x] array literal `[1, 2]`
	- [x] array indexing `arr[2]`
	- [x] splat `*arr`
	- [x] array decomposition
	- [x] implicit array assignment
	- [x] `%w{}` (string array)
	- [x] `%i{}` (symbol array)
- [x] hashes
	- [x] `=>` notation
	- [x] `key:` shorthand
	- [x] indexing `hash[:foo]`
- [x] symbols
	- [x] `:symbol`
	- [x] `:"symbol"`
	- [x] `:"symbol"` with interpolation
	- [x] `:'symbol'`
	- [x] `%s{symbol}`
- [x] regexp
	- [x] `/regex/`
	- [x] `%r{regex}`
- [x] ranges
	- [x] `..` inclusive
	- [x] `...` exclusive
- [x] procs / lambdas
	- [x] `do ... end` / `{ ... }` blocks
	- [x] `->` lambda literal

## variables

- [x] local assignments
- [x] globals (`$var`)
- [x] instance variables (`@var`)
- [x] class variables (`@@var`)
- [x] constants (capitalised idents)
- [x] scope operator `::`

## operators

- [x] arithmetic: `+`, `-`, `*`, `/`, `%`
- [x] unary: `!`, unary `-`
- [x] comparison: `<`, `>`, `<=`, `>=`, `==`, `!=`, `<=>`
- [x] `**` (pow)
- [x] `&` (and), `|` (or), `^` (xor)
- [x] `>>` (right shift)
- [x] `<<` (left shift / append)
- [x] `===` (case equality)
- [x] `=~`, `!~` (pattern match / not match)
- [x] assignment operators
	- [x] `+=`, `-=`, `*=`, `/=`, `%=`
	- [x] `**=`, `&=`, `|=`, `^=`, `<<=`, `>>=`, `||=`, `&&=`

## error handling

- [x] begin/rescue
- [x] ensure
- [x] retry

## classes + modules

- [x] class definition
- [x] inheritance (`class X < Y`)
- [x] instance methods
- [x] class methods (`def self.foo`)
- [x] singleton classes / eigenclass (`class << expr`)
- [x] visibility keywords (`private`, `protected`, `public`) -- parsed as identifiers / method calls
- [x] assignment methods (`def foo=`)
- [x] modules

## misc

- [x] comments (`#`)
- [x] UTF-8 source support (lexer accepts non-ASCII letters via
      `unicode.IsLetter`; magic encoding comments honoured per MRI)
	- [x] Unicode identifiers (`αβ`, `метод`, `Établissement`, `اختبر!`,
	      `ภาษา` -- letters from any script; non-letter codepoints
	      like emoji are rejected as `ILLEGAL` to match MRI)
	- [x] Unicode symbols (`:αβ`, `:"метод"`, etc -- same letter rule)
	- [x] BOM stripping on source input -- a leading UTF-8 BOM is
	      transparently skipped before tokenization, matching MRI
	      1.9 through 4.0. Mid-source U+FEFF is also accepted as a
	      valid identifier letter, again matching MRI.
