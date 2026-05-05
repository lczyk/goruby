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
	- [ ] keyword arguments
	- [x] block arguments
	- [ ] hash as last argument without braces
- [x] function calls
	- [x] with parens
	- [x] without parens
	- [x] with block arguments
- [ ] conditionals
	- [x] if
	- [x] if/else
	- [ ] if/elif/else
	- [x] tenary `? :`
	- [x] unless
	- [x] unless/else
	- [ ] case
- [x] short-circuit operators (`||`, `&&`)
- [ ] control flow
	- [ ] for loop
	- [x] while loop
	- [ ] until loop
	- [ ] break
	- [ ] next
	- [ ] redo
	- [ ] flip flop

## literals

- [ ] integers
	- [x] decimal `1234`
	- [x] underscores `1_234`
	- [ ] explicit decimal `0d170`, `0D170`
	- [ ] octal `0252`, `0o252`, `0O252`
	- [ ] hexadecimal `0xaa`, `0xAa`, `0xAA`, `0Xaa`, `0XAa`, `0XaA`
	- [ ] binary `0b10101010`, `0B10101010`
- [ ] floats
	- [ ] `12.34`
	- [ ] `1234e-2`
	- [ ] `1.234E1`
	- [ ] underscores `2.2_22`
- [x] booleans (`true`, `false`)
- [x] nil
- [ ] strings
	- [x] double quoted
	- [x] single quoted
	- [x] character literals (`?\n`, `?a`, ...)
	- [ ] `%q{}`
	- [ ] `%Q{}`
	- [ ] heredoc
		- [ ] `<<EOF`
		- [ ] indented `<<-EOF`
		- [ ] squiggly `<<~`
		- [ ] quoted heredoc (single, double, backtick)
	- [ ] escaped characters (full `\a` ... `\C-?` set)
	- [ ] interpolation `#{}`
	- [ ] automatic concatenation
- [ ] arrays
	- [x] array literal `[1, 2]`
	- [x] array indexing `arr[2]`
	- [ ] splat `*arr`
	- [ ] array decomposition
	- [ ] implicit array assignment
	- [ ] `%w{}` (string array)
	- [ ] `%i{}` (symbol array)
- [ ] hashes
	- [x] `=>` notation
	- [ ] `key:` shorthand
	- [x] indexing `hash[:foo]`
- [ ] symbols
	- [x] `:symbol`
	- [x] `:"symbol"`
	- [ ] `:"symbol"` with interpolation
	- [x] `:'symbol'`
	- [ ] `%s{symbol}`
- [ ] regexp
	- [ ] `/regex/`
	- [ ] `%r{regex}`
- [ ] ranges
	- [ ] `..` inclusive
	- [ ] `...` exclusive
- [ ] procs / lambdas
	- [x] `do ... end` / `{ ... }` blocks
	- [ ] `->` lambda literal

## variables

- [x] local assignments
- [x] globals (`$var`)
- [x] instance variables (`@var`)
- [ ] class variables (`@@var`) -- lexer tokenises as `CLASS_VAR`; parser does not consume yet
- [x] constants (capitalised idents)
- [x] scope operator `::`

## operators

- [x] arithmetic: `+`, `-`, `*`, `/`, `%`
- [x] unary: `!`, unary `-`
- [x] comparison: `<`, `>`, `<=`, `>=`, `==`, `!=`, `<=>`
- [ ] `**` (pow)
- [ ] `&` (and), `|` (or), `^` (xor)
- [ ] `>>` (right shift)
- [ ] `<<` (left shift / append)
- [ ] `===` (case equality)
- [ ] `=~`, `!~` (pattern match / not match)
- [ ] assignment operators
	- [x] `+=`, `-=`, `*=`, `/=`, `%=`
	- [ ] `**=`, `&=`, `|=`, `^=`, `<<=`, `>>=`, `||=`, `&&=`

## error handling

- [x] begin/rescue
- [ ] ensure
- [ ] retry

## classes + modules

- [x] class definition
- [x] inheritance (`class X < Y`)
- [x] instance methods
- [x] class methods (`def self.foo`)
- [x] singleton classes / eigenclass (`class << expr`)
- [ ] visibility keywords (`private`, `protected`, `public`) -- parsed as identifiers / calls; not specially recognised
- [ ] assignment methods (`def foo=`)
- [x] modules

## misc

- [x] comments (`#`)
- [ ] full UTF-8 source support
	- [ ] Unicode identifiers
	- [ ] Unicode symbols
