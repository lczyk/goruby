# patches for ruby 2.0.x on modern toolchains

ruby 2.0.0 was released in 2013 and last patched in 2016. several
places in the codebase rely on implicit function declarations that
worked under older C standards but are rejected by modern clang.

## thread-implicit-decl.patch

`thread.c` line 4835 calls `rb_frame_last_func()`, which is defined
in `eval.c` but never declared in any header. older compilers accepted
this as an implicit `int`-returning function; clang 16+ (C99+ mode)
treats calls to undeclared functions as hard errors.

the fix adds a forward declaration at the top of `thread.c`, after the
existing includes. the sister function `rb_frame_this_func()` is
already declared in `include/ruby/intern.h`, but `rb_frame_last_func`
was apparently left out of the public API headers -- it's only used
internally.
