# .rubies/ -- MRI ruby toolchain for syntax verification

this directory builds MRI ruby interpreters from source so the project
can verify that test fixtures are valid ruby. the parser targets
multiple ruby versions, so we need actual ruby binaries to confirm
what's valid under which version.

each ruby is compiled to a minimal install -- just enough for
`ruby -c <file>` (syntax check). extensions, docs, and gems are
skipped.

## what's here

```
rubies.lock        pinned versions, source URLs, sha256 checksums
fetch_rubies.sh    downloads + compiles everything in rubies.lock
verify.sh          runs `ruby -c` on all test fixtures against all rubies
patches/           quilt-style patches for old rubies on modern toolchains
  <major.minor>/
    series         patch application order
    *.patch        individual patches
    README.md      why each patch exists
makefile           local targets: rubies, verify, clean
versions/          compiled rubies (gitignored)
cache/             downloaded tarballs (gitignored)
build/             temporary build dirs (gitignored, cleaned after install)
```

## building

```
make rubies          # from repo root, or:
make -C .rubies      # from anywhere
```

builds every version in `rubies.lock` that doesn't already have a
binary in `versions/`. existing binaries are skipped, so re-running is
cheap.

**rough build time:** ~15-25 minutes for a full build of all 12
versions on an M-series mac (arm64). individual rubies take 1-3
minutes each. subsequent runs finish in seconds (cached).

### prerequisites

- C compiler (`cc` -- clang or gcc)
- `make`, `curl`, `tar`, `patch`
- `shasum` or `sha256sum` (for checksum verification)
- standard C library headers (`stdlib.h`, `math.h`, etc.)

currently tested on macOS (arm64, apple clang). linux and other
arches/compilers are planned but not yet tested -- expect additional
patches may be needed, especially for the older rubies (1.9, 2.0).

### known issues

old rubies (<= 2.0) don't compile cleanly on modern toolchains. the
`patches/` directory contains fixes for specific compilation errors.
see each patch directory's `README.md` for details on what broke and
why.

## verifying fixtures

```
make rubies-verify   # from repo root
```

checks every `.rb` test fixture against all installed rubies. each
file must parse successfully under at least one version. the report
shows which files pass/fail under which rubies.

## adding a new ruby version

1. find the release on https://www.ruby-lang.org/en/downloads/releases/
2. add a line to `rubies.lock`: `<version>\t<tar.gz-url>\t<sha256:hash>`
3. run `make rubies`
4. if it fails to build, create a patch in `patches/<major.minor>/`
   and add it to the `series` file
