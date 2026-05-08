# patches for ruby 1.9.x on modern toolchains

ruby 1.9.3 was released in 2011 and last patched in 2014. it predates
the C99 strict mode that modern clang (16+, shipping with Xcode 16 /
macOS 15+) now enforces by default.

## missing-math-h.patch

`missing/finite.c` calls `isnan()` and `isinf()` without including
`<math.h>`. older compilers inferred the declaration implicitly; clang
16+ treats implicit function declarations as hard errors under
`-Wimplicit-function-declaration` (promoted to error in C99+ mode).

the fix adds `#include <math.h>` after the existing `ruby/missing.h`
include. ruby 2.0.0 already ships with this include.
