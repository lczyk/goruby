#!/usr/bin/env bash
# Fetch ruby gems listed in testdata/gems.lock into testdata/gems/<name>/.
# Skips entries already present; delete a directory (or run `make gems-clean`)
# to force a refetch. Used by integration tests; never run by `go test`.

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
lock="$script_dir/gems.lock"
dest="$script_dir/gems"

if [[ ! -f "$lock" ]]; then
    echo "missing $lock" >&2
    exit 1
fi

mkdir -p "$dest"

while IFS=$'\t' read -r name url ref || [[ -n "${name:-}" ]]; do
    case "$name" in
        ''|\#*) continue ;;
    esac
    if [[ -z "${url:-}" || -z "${ref:-}" ]]; then
        echo "malformed entry: $name" >&2
        exit 1
    fi
    target="$dest/$name"
    if [[ -d "$target" ]]; then
        echo "  $name @ $ref (skip, exists)"
        continue
    fi
    echo "  $name <- $url @ $ref"
    if ! git clone --depth 1 --branch "$ref" --quiet "$url" "$target" 2>/dev/null; then
        # Fall back to full clone + checkout for refs git can't resolve as a branch/tag.
        rm -rf "$target"
        git clone --quiet "$url" "$target"
        git -C "$target" checkout --quiet "$ref"
    fi
    # Drop .git so the file walker can't accidentally descend into pack files
    # and so the fixture is a pure source snapshot.
    rm -rf "$target/.git"
done < "$lock"
