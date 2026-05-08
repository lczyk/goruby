#!/usr/bin/env bash
# Verify all downloaded test fixtures against downloaded ruby binaries.
#
# Each .rb fixture must be valid Ruby under at least one of the installed
# ruby versions. Fixtures using version-specific syntax (e.g. 3.0+ endless
# methods) may fail on older rubies -- that's expected.
#
# Set NO_COLOR=1 to disable terminal colors.
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

# -- colors ---------------------------------------------------------
if [[ -n "${NO_COLOR:-}" ]]; then
    RED='' GREEN='' YELLOW='' CYAN='' BOLD='' RST=''
else
    RED=$'\033[31m' GREEN=$'\033[32m' YELLOW=$'\033[33m'
    CYAN=$'\033[36m' BOLD=$'\033[1m' RST=$'\033[0m'
fi

# -- collect ruby binaries ------------------------------------------
rubies=()
ruby_names=()
shopt -s nullglob
for ruby_path in "$versions_dir"/*/bin/ruby; do
    [[ -x "$ruby_path" ]] || continue
    rubies+=("$ruby_path")
    ver_dir="$(dirname "$(dirname "$ruby_path")")"
    ruby_names+=("$(basename "$ver_dir")")
done

if ((${#rubies[@]} == 0)); then
    echo "no ruby binaries found" >&2
    exit 1
fi

echo "rubies:"
for i in "${!rubies[@]}"; do
    printf "  ${CYAN}%s${RST}  %s\n" "${ruby_names[$i]}" "$("${rubies[$i]}" --version | head -1)"
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
total=${#rb_files[@]}
checked=0
all_pass=0        # passes under every ruby
any_pass=0        # passes under >=1 but not all
all_fail=0        # fails under every ruby
all_fail_files=() # files failing under all rubies

# Per-ruby counters: parallel indexed arrays matching rubies[] order.
ruby_fail_counts=()
ruby_check_counts=()
for i in "${!rubies[@]}"; do
    ruby_fail_counts[$i]=0
    ruby_check_counts[$i]=0
done

for f in "${rb_files[@]}"; do
    ((checked++))
    rel="${f#$repo_root/}"

    # Build per-ruby status for this file.
    statuses=()
    pass_count=0
    fail_count=0
    first_error=""

    for i in "${!rubies[@]}"; do
        ruby_path="${rubies[$i]}"
        ((ruby_check_counts[$i]++))
        if output=$("$ruby_path" -c "$f" 2>&1); then
            statuses+=("${GREEN}OK${RST}")
            ((pass_count++))
        else
            statuses+=("${YELLOW}--${RST}")
            ((fail_count++))
            ((ruby_fail_counts[$i]++))
            [[ -z "$first_error" ]] && first_error="$output"
        fi
    done

    # Print one compact line per file.
    printf "  "
    for i in "${!rubies[@]}"; do
        printf "[%s] " "${statuses[$i]}"
    done
    printf "%s" "$rel"

    if ((fail_count == ${#rubies[@]})); then
        # Fails under all rubies -- real problem.
        ((all_fail++))
        all_fail_files+=("$rel")
        err=$(echo "$first_error" | head -1)
        printf "  ${RED}FAIL${RST}\n        ${RED}%s${RST}\n" "$err"
    elif ((pass_count == ${#rubies[@]})); then
        # Passes under all rubies.
        ((all_pass++))
        printf "\n"
    else
        # Passes under some, fails under others -- version-specific.
        ((any_pass++))
        printf "\n"
    fi
done

# -- report ---------------------------------------------------------
echo ""
printf "${BOLD}report${RST}\n"
echo "  files checked: $total"

# Per-ruby summary.
for i in "${!ruby_names[@]}"; do
    name="${ruby_names[$i]}"
    checked_n="${ruby_check_counts[$i]}"
    failed_n="${ruby_fail_counts[$i]}"
    passed_n=$((checked_n - failed_n))
    if ((failed_n > 0)); then
        printf "  ${CYAN}%s${RST}: %d/%d pass, ${YELLOW}%d fail${RST}\n" \
            "$name" "$passed_n" "$checked_n" "$failed_n"
    else
        printf "  ${CYAN}%s${RST}: %d/%d pass, 0 fail\n" \
            "$name" "$passed_n" "$checked_n"
    fi
done

# File-level summary.
printf "\n"
printf "  ${GREEN}all pass${RST}: %d files (valid under every ruby)\n" "$all_pass"
if ((any_pass > 0)); then
    printf "  ${YELLOW}partial${RST}:  %d files (version-specific syntax, expected)\n" "$any_pass"
fi
if ((all_fail > 0)); then
    printf "  ${RED}all fail${RST}: %d files (not valid under any ruby):\n" "$all_fail"
    for f in "${all_fail_files[@]}"; do
        printf "    ${RED}%s${RST}\n" "$f"
    done
fi

echo ""

if ((all_fail > 0)); then
    printf "${RED}${BOLD}FAIL${RST}: %d fixtures are not valid Ruby under any installed version\n" "$all_fail"
    exit 1
fi

printf "${GREEN}${BOLD}OK${RST}: all fixtures pass syntax check under at least one ruby version\n"
