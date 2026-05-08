#!/usr/bin/env bash
# Fetch and compile MRI Ruby versions listed in .rubies/rubies.lock.
#
# Each version is compiled from source and installed into:
#   .rubies/versions/<version>/bin/ruby
#
# Cached: skips versions whose ruby binary already exists.
# Delete a directory (or run `make rubies-clean`) to force a rebuild.
#
# Never run by `go test`; invoked via `make rubies` or directly.

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
lock="$script_dir/rubies.lock"
dest="$script_dir/versions"
cache="$script_dir/cache"   # raw tarballs
build="$script_dir/build"   # build directories (cleaned after install)

if [[ ! -f "$lock" ]]; then
    echo "missing $lock" >&2
    exit 1
fi

mkdir -p "$dest" "$cache" "$build"

# -- check deps --------------------------------------------------
missing_deps=()
for dep in curl tar; do
    command -v "$dep" >/dev/null 2>&1 || missing_deps+=("$dep")
done
for dep in cc make; do
    command -v "$dep" >/dev/null 2>&1 || missing_deps+=("$dep")
done
if ((${#missing_deps[@]} > 0)); then
    echo "missing deps: ${missing_deps[*]}" >&2
    exit 1
fi

# prefer shasum over sha256sum -- the macOS sha256sum is not GNU coreutils
# and doesn't support --check. shasum -a 256 -c works everywhere.
if command -v shasum >/dev/null 2>&1; then
    _sha_verify() { echo "$1  $2" | shasum -a 256 -c -s; }
elif command -v sha256sum >/dev/null 2>&1; then
    _sha_verify() { echo "$1  $2" | sha256sum --check --status; }
else
    echo "missing shasum / sha256sum" >&2
    exit 1
fi

# Number of parallel jobs for make.
NPROC=$(sysctl -n hw.ncpu 2>/dev/null || nproc 2>/dev/null || echo 4)

# -- helpers -----------------------------------------------------
_download() {
    local url="$1" out="$2"
    echo "    downloading $url"
    curl -fsSL --retry 2 -o "$out" "$url"
}

_verify() {
    local tarball="$1" expected="$2"
    if [[ "$expected" == "sha256:"* ]]; then
        if _sha_verify "${expected#sha256:}" "$tarball" 2>/dev/null; then
            return 0
        fi
        return 1
    fi
    echo "    (no checksum for $tarball; skip verify)" >&2
    return 0
}

# -- apply quilt patches for old rubies ----------------------------
_apply_patches() {
    local src="$1" version="$2"
    local major_minor

    # Derive major.minor (e.g. "1.9" from "1.9.3-p551", "2.0" from "2.0.0-p648").
    major_minor="${version%.*}"
    major_minor="${major_minor%-*}"

    # Look for a patches directory matching this version.
    local patch_dir="$script_dir/patches/$major_minor"
    if [[ ! -d "$patch_dir" ]]; then
        return 0
    fi

    local series="$patch_dir/series"
    if [[ ! -f "$series" ]]; then
        return 0
    fi

    echo "    applying patches from patches/$major_minor/"
    while IFS= read -r patchfile; do
        case "$patchfile" in ''|\#*) continue ;; esac
        local pf="$patch_dir/$patchfile"
        if [[ ! -f "$pf" ]]; then
            echo "    WARNING: patch file $pf not found, skipping" >&2
            continue
        fi
        if ! patch -d "$src" -p1 --quiet < "$pf"; then
            echo "    ERROR: failed to apply $patchfile" >&2
            exit 1
        fi
        echo "      applied $patchfile"
    done < "$series"
}

# -- main --------------------------------------------------------
total=0
skipped=0
fetched=0

while IFS=$'\t' read -r version url checksum || [[ -n "${version:-}" ]]; do
    case "$version" in
        ''|\#*) continue ;;
    esac
    if [[ -z "${url:-}" ]]; then
        echo "malformed entry: $version (missing url)" >&2
        exit 1
    fi

    ((total++))
    target="$dest/$version"

    # If the ruby binary already exists, skip.
    if [[ -x "$target/bin/ruby" ]]; then
        echo "  $version (skip, $( "$target/bin/ruby" --version | head -1))"
        ((skipped++))
        continue
    fi

    echo "  $version <- $url"

    # Download tarball.
    tarball_name="${url##*/}"
    tarball="$cache/$tarball_name"

    if [[ ! -f "$tarball" ]]; then
        _download "$url" "$tarball"
    else
        echo "    (cached tarball)"
    fi

    # Verify checksum.
    if [[ -n "${checksum:-}" ]]; then
        if ! _verify "$tarball" "$checksum"; then
            echo "    checksum mismatch; re-downloading"
            _download "$url" "$tarball"
            _verify "$tarball" "$checksum" || {
                echo "    checksum still mismatched; aborting" >&2
                exit 1
            }
        fi
    fi

    # Extract into a per-version build directory.
    build_dir="$build/$version"
    rm -rf "$build_dir"
    mkdir -p "$build_dir"

    echo "    extracting"
    tar -xf "$tarball" -C "$build_dir"

    src="$(find "$build_dir" -maxdepth 1 -type d -name 'ruby-*' | head -1)"
    if [[ ! -d "$src" ]]; then
        echo "    could not find ruby-$version/ in tarball" >&2
        exit 1
    fi

    # Apply quilt-style patches for old rubies on modern toolchains.
    _apply_patches "$src" "$version"

    # Compile a minimal ruby -- we only need `ruby -c` for syntax checks.
    # Disable extensions that don't build on modern platforms (fiddle, openssl).
    echo "    configuring"
    (
        cd "$src"
        ./configure \
            --prefix="$target" \
            --disable-install-doc \
            --without-gmp \
            --without-fiddle \
            --without-openssl \
            --quiet
    ) >/dev/null

    # Build just the ruby binary, not extensions. Extensions often fail
    # on modern toolchains for old Ruby versions and we don't need them.
    echo "    building (-j$NPROC)"
    make -C "$src" ruby -j"$NPROC" >/dev/null

    # Try the full install first; if extensions fail, install just the binary.
    echo "    installing"
    if ! make -C "$src" install-nodoc >/dev/null 2>&1; then
        # Extensions failed -- install the core ruby binary manually.
        mkdir -p "$target/bin"
        cp "$src/ruby" "$target/bin/ruby"
        # Also copy libruby so `ruby -c` can link.
        mkdir -p "$target/lib"
        cp "$src/libruby."* "$target/lib/" 2>/dev/null || true
    fi

    # Verify the binary works.
    if [[ ! -x "$target/bin/ruby" ]]; then
        echo "    ERROR: build did not produce $target/bin/ruby" >&2
        exit 1
    fi
    echo "    -> $( "$target/bin/ruby" --version | head -1)"

    # Clean up build directory to save space.
    rm -rf "$build_dir"

    ((fetched++))
done < "$lock"

echo ""
echo "rubies: $total in lock, $skipped skipped, $fetched built"
