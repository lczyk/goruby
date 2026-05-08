#!/usr/bin/env bash
# Verify all downloaded test fixtures against downloaded ruby binaries.
#
# Each .rb fixture must be valid Ruby under at least one of the installed
# ruby versions. Fixtures using version-specific syntax (e.g. 3.0+ endless
# methods) may fail on older rubies -- that's expected.
#
# Not part of `make test`; run via `make rubies-verify` or directly.
#
# Requires: `make rubies` and `make gems` to have been run first.

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/.." && pwd)"
versions_dir="$script_dir/versions"

if [[ ! -d "$versions_dir" ]] || [[ -z "$(ls -A "$versions_dir" 2>/dev/null)" ]]; then
    echo "no ruby versions found in $versions_dir" >&2
    echo "run 'make rubies' first" >&2
    exit 1
fi

# -- collect ruby binaries ------------------------------------------
rubies=()
shopt -s nullglob
for ruby_path in "$versions_dir"/*/bin/ruby; do
    [[ -x "$ruby_path" ]] || continue
    rubies+=("$ruby_path")
done

if ((${#rubies[@]} == 0)); then
    echo "no ruby binaries found" >&2
    exit 1
fi

echo "rubies:"
for r in "${rubies[@]}"; do
    ver_dir="$(dirname "$(dirname "$r")")"
    echo "  $(basename "$ver_dir")  ($("$r" --version | head -1))"
done
echo ""

# -- fixture directories to scan ------------------------------------
fixture_dirs=(
    "$repo_root/internal/integrationtest/testdata/gems"
    "$repo_root/internal/integrationtest/testdata/mri-tests"
    "$repo_root/internal/integrationtest/testdata/esolangs"
    "$repo_root/internal/integrationtest/testdata/ruby-extra/parser"
)

rb_files=()
for dir in "${fixture_dirs[@]}"; do
    if [[ -d "$dir" ]]; then
        while IFS= read -r -d '' f; do
            rb_files+=("$f")
        done < <(find "$dir" -name '*.rb' -print0 2>/dev/null)
    fi
done

if ((${#rb_files[@]} == 0)); then
    echo "no .rb files found in fixture directories" >&2
    exit 1
fi

echo "fixtures: ${#rb_files[@]} .rb files"
echo ""

# -- verify each file against all rubies ----------------------------
failures=0
pass_somewhere=0

for f in "${rb_files[@]}"; do
    rel="${f#$repo_root/}"
    passed_any=false
    last_error=""

    for ruby_path in "${rubies[@]}"; do
        if output=$("$ruby_path" -c "$f" 2>&1); then
            passed_any=true
            break
        else
            last_error="$output"
        fi
    done

    if ! $passed_any; then
        ((failures++))
        first_line=$(echo "$last_error" | head -1)
        printf "FAIL  %s\n      %s\n" "$rel" "$first_line"
    else
        ((pass_somewhere++))
    fi
done

echo ""
echo "  $pass_somewhere pass (under >=1 ruby), $failures fail (under all rubies)"

if ((failures > 0)); then
    echo ""
    echo "FAIL: $failures fixtures are not valid Ruby under any installed version"
    exit 1
fi

echo "OK: all fixtures pass syntax check under at least one ruby version"
