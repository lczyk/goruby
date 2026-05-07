#!/usr/bin/env bash
# Fetch MRI Ruby source tarballs listed in testdata/rubies.lock into
# testdata/rubies/<version>/. From each tarball we extract:
#   test/ruby/  -- MRI language test suite
#   lib/        -- Ruby standard library
#
# Cached: skips versions already present. Delete a directory (or run
# `make rubies-clean`) to force a refetch.
#
# Never run by `go test`; invoked via `make rubies` or directly.

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
lock="$script_dir/rubies.lock"
dest="$script_dir/rubies"
cache="$script_dir/.rubies-cache"  # raw tarballs, so we don't re-download

if [[ ! -f "$lock" ]]; then
    echo "missing $lock" >&2
    exit 1
fi

mkdir -p "$dest" "$cache"

# -- check deps --------------------------------------------------
missing_deps=()
for dep in curl tar; do
    command -v "$dep" >/dev/null 2>&1 || missing_deps+=("$dep")
done
if ((${#missing_deps[@]} > 0)); then
    echo "missing deps: ${missing_deps[*]}" >&2
    exit 1
fi

# prefer sha256sum (coreutils) over shasum (perl, macOS) -- both work
if command -v sha256sum >/dev/null 2>&1; then
    sha_cmd=(sha256sum --check --status)
elif command -v shasum >/dev/null 2>&1; then
    sha_cmd() { echo "$1  $2" | shasum -a 256 -c -s; }
else
    echo "missing sha256sum / shasum" >&2
    exit 1
fi

# -- helpers -----------------------------------------------------
_download() {
    local url="$1" out="$2"
    echo "    downloading $url"
    curl -fsSL --retry 2 -o "$out" "$url"
}

_verify() {
    local tarball="$1" expected="$2"
    if [[ "$expected" == "sha256:"* ]]; then
        if "${sha_cmd[@]}" "${expected#sha256:}  $tarball" 2>/dev/null; then
            return 0
        fi
        return 1
    fi
    echo "    (no checksum for $tarball; skip verify)" >&2
    return 0
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

    if [[ -d "$target" ]] && [[ -n "$(ls -A "$target" 2>/dev/null)" ]]; then
        echo "  $version (skip, exists)"
        ((skipped++))
        continue
    fi

    echo "  $version <- $url"

    # Determine tarball filename from url.
    tarball_name="${url##*/}"
    tarball="$cache/$tarball_name"

    # Download if not cached.
    if [[ ! -f "$tarball" ]]; then
        _download "$url" "$tarball"
    else
        echo "    (cached)"
    fi

    # Verify checksum if provided.
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

    # Extract into a temp dir, then copy test/ruby/ + lib/ into the target.
    tmp="$(mktemp -d)"
    trap 'rm -rf "$tmp"' EXIT

    echo "    extracting"
    tar -xf "$tarball" -C "$tmp"

    # The tarball extracts as ruby-<version>/.
    src="$tmp/ruby-$version"
    if [[ ! -d "$src" ]]; then
        # Some older tarballs might use a different naming convention.
        src="$(find "$tmp" -maxdepth 1 -type d -name 'ruby-*' | head -1)"
    fi
    if [[ ! -d "$src" ]]; then
        echo "    could not find ruby-$version/ in tarball" >&2
        exit 1
    fi

    mkdir -p "$target"

    # Copy test/ruby/ (language test suite).
    if [[ -d "$src/test/ruby" ]]; then
        cp -R "$src/test/ruby" "$target/test"
        echo "    -> test/ruby ($(find "$target/test" -name '*.rb' | wc -l) .rb files)"
    else
        echo "    warning: no test/ruby/ in $version" >&2
    fi

    # Copy lib/ (standard library).
    if [[ -d "$src/lib" ]]; then
        cp -R "$src/lib" "$target/lib"
        echo "    -> lib ($(find "$target/lib" -name '*.rb' | wc -l) .rb files)"
    else
        echo "    warning: no lib/ in $version" >&2
    fi

    rm -rf "$tmp"
    trap - EXIT

    ((fetched++))
done < "$lock"

echo ""
echo "rubies: $total in lock, $skipped skipped, $fetched fetched"
