# Identifier and variable adversarial examples.

# Method names with ? and ! suffixes
valid?
invalid!
run!
nil?
empty?
any? { |x| x > 0 }
foo!

# Constants
Foo
FOO
FOO_BAR
FooBar
F
A1
A_1

# Global variables
$var
$VAR
$_var
$0
$1
$9
$!
$?
$.
$,
$;
$:
$"
$'
$~

# Invalid-looking globals that are valid
$<
$>
$=
$&
$*
$+
$-
$/
$\\
$`

# Instance variables
@foo
@bar_baz
@a
@_x

# Class variables
@@foo
@@bar_baz
@@class_variable
@@a
@@_x

# Method definition with various name forms
def foo; end
def foo?; end
def foo!; end
def foo=; end
def ==; end
def <=>; end
def []; end
def []=; end
def +@; end      # unary plus
def -@; end      # unary minus
def self.foo; end
def Foo.bar; end

# Keyword-like method names -- these are NOT keywords in method position
def class; end    # valid method name
def module; end
def def; end
def end; end

# Identifiers that look like keywords but aren't
self_worth = 5
classy = "foo"
module_name = "bar"
definitely_not = true
ended = false
begin_transaction
rescue_me
while_loop

# Constants with scope operator
Foo::Bar
Foo::Bar::Baz
::TopLevel    # absolute constant path

# Unicode identifiers (if supported)
# canada = 1
# pi = 3.14

# __LINE__ and __FILE__ and __ENCODING__ keywords
__LINE__
__FILE__
__ENCODING__

# Edge: identifier immediately followed by non-ident char
x+y
x-y
x*y
x/y
x%y
x&y

# @@ at start but not class var (invalid -- but lexer should survive)
# This would be: AT then AT then IDENT -- but actually @@identifier is CLASS_VAR
