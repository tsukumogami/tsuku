#!/usr/bin/env bash
# R2 Consistency Check Script
#
# Compares golden files between git and R2 to verify consistency during
# the parallel operation period.
#
# Usage:
#   ./scripts/r2-consistency-check.sh [--category <embedded|registry>] [--recipe <name>]
#
# Options:
#   --category <cat>    Only check recipes of the specified category
#   --recipe <name>     Only check the specified recipe
#   --verbose           Show detailed comparison output
#
# Environment Variables:
#   R2_BUCKET_URL           - Required. R2 bucket endpoint URL
#   R2_ACCESS_KEY_ID        - Required. R2 access key ID
#   R2_SECRET_ACCESS_KEY    - Required. R2 secret access key
#
# Exit Codes:
#   0 - All comparable files match
#   1 - Mismatches found
#   2 - Error (missing credentials, etc.)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
GOLDEN_BASE="$REPO_ROOT/testdata/golden/plans"

# Parse arguments
FILTER_CATEGORY=""
FILTER_RECIPE=""
VERBOSE=false

while [[ $# -gt 0 ]]; do
    case "$1" in
        --category) FILTER_CATEGORY="$2"; shift 2 ;;
        --recipe)   FILTER_RECIPE="$2"; shift 2 ;;
        --verbose)  VERBOSE=true; shift ;;
        -h|--help)
            echo "Usage: $0 [--category <embedded|registry>] [--recipe <name>] [--verbose]"
            echo ""
            echo "Compare golden files between git and R2."
            echo ""
            echo "Options:"
            echo "  --category <cat>  Only check embedded or registry recipes"
            echo "  --recipe <name>   Only check the specified recipe"
            echo "  --verbose         Show detailed comparison output"
            exit 0
            ;;
        *)
            echo "Unknown argument: $1" >&2
            exit 2
            ;;
    esac
done

# Validate credentials
if [[ -z "${R2_BUCKET_URL:-}" ]] || [[ -z "${R2_ACCESS_KEY_ID:-}" ]] || [[ -z "${R2_SECRET_ACCESS_KEY:-}" ]]; then
    echo "Error: R2 credentials not set." >&2
    echo "Required: R2_BUCKET_URL, R2_ACCESS_KEY_ID, R2_SECRET_ACCESS_KEY" >&2
    exit 2
fi

# Check for golden directory
if [[ ! -d "$GOLDEN_BASE" ]]; then
    echo "Error: Golden files directory not found: $GOLDEN_BASE" >&2
    exit 2
fi

MATCHES=0
MISMATCHES=0
GIT_ONLY=0
R2_ONLY=0
ERRORS=0

# Create temp directory for R2 downloads
TEMP_DIR=$(mktemp -d)
trap 'rm -rf "$TEMP_DIR"' EXIT

# Parse golden file path to extract components
# Format: v{version}-{platform}.json (git) or v{version}/{platform}.json (R2)
parse_git_filename() {
    local filename="$1"
    local name="${filename%.json}"

    # Find platform by looking for OS marker
    if [[ "$name" =~ -linux- ]]; then
        local after_linux="${name#*-linux-}"
        local first_part=$(echo "$after_linux" | cut -d'-' -f1)
        if [[ "$first_part" =~ ^(debian|rhel|arch|alpine|suse)$ ]]; then
            echo "${name%-linux-*}" "linux-${first_part}-$(echo "$after_linux" | cut -d'-' -f2)"
        else
            echo "${name%-linux-*}" "linux-${first_part}"
        fi
    elif [[ "$name" =~ -darwin- ]]; then
        local after_darwin="${name#*-darwin-}"
        echo "${name%-darwin-*}" "darwin-${after_darwin}"
    else
        echo "" ""
    fi
}

# Compare a single file
compare_file() {
    local git_file="$1"
    local recipe="$2"
    local category="$3"
    local filename=$(basename "$git_file")

    read -r version platform < <(parse_git_filename "$filename")

    if [[ -z "$version" ]] || [[ -z "$platform" ]]; then
        [[ "$VERBOSE" == true ]] && echo "SKIP: Cannot parse $filename"
        return 0
    fi

    # Remove leading 'v' from version for R2 download
    local version_no_v="${version#v}"

    # Download from R2
    local r2_file="$TEMP_DIR/${recipe}_${version_no_v}_${platform}.json"

    if ! "$SCRIPT_DIR/r2-download.sh" --category "$category" "$recipe" "$version_no_v" "$platform" "$r2_file" 2>/dev/null; then
        # File doesn't exist in R2
        [[ "$VERBOSE" == true ]] && echo "GIT_ONLY: $recipe @ $version ($platform)"
        ((GIT_ONLY++))
        return 0
    fi

    # Compare checksums
    local git_hash=$(sha256sum "$git_file" | cut -d' ' -f1)
    local r2_hash=$(sha256sum "$r2_file" | cut -d' ' -f1)

    if [[ "$git_hash" == "$r2_hash" ]]; then
        [[ "$VERBOSE" == true ]] && echo "MATCH: $recipe @ $version ($platform)"
        ((MATCHES++))
    else
        echo "MISMATCH: $recipe @ $version ($platform)"
        echo "  Git:  $git_hash"
        echo "  R2:   $r2_hash"
        ((MISMATCHES++))
    fi
}

# Check embedded recipes
check_embedded() {
    local embedded_dir="$GOLDEN_BASE/embedded"
    [[ ! -d "$embedded_dir" ]] && return

    for recipe_dir in "$embedded_dir"/*/; do
        [[ -d "$recipe_dir" ]] || continue
        local recipe=$(basename "$recipe_dir")

        [[ -n "$FILTER_RECIPE" && "$recipe" != "$FILTER_RECIPE" ]] && continue

        for file in "$recipe_dir"/*.json; do
            [[ -f "$file" ]] || continue
            compare_file "$file" "$recipe" "embedded"
        done
    done
}

# Check registry recipes
check_registry() {
    for letter_dir in "$GOLDEN_BASE"/[a-z]/; do
        [[ -d "$letter_dir" ]] || continue

        for recipe_dir in "$letter_dir"*/; do
            [[ -d "$recipe_dir" ]] || continue
            local recipe=$(basename "$recipe_dir")

            [[ -n "$FILTER_RECIPE" && "$recipe" != "$FILTER_RECIPE" ]] && continue

            for file in "$recipe_dir"/*.json; do
                [[ -f "$file" ]] || continue
                local letter="${recipe:0:1}"
                compare_file "$file" "$recipe" "$letter"
            done
        done
    done
}

# Run checks based on category filter
echo "R2 Consistency Check"
echo "===================="
echo ""

if [[ -z "$FILTER_CATEGORY" ]]; then
    check_embedded
    check_registry
elif [[ "$FILTER_CATEGORY" == "embedded" ]]; then
    check_embedded
elif [[ "$FILTER_CATEGORY" == "registry" ]]; then
    check_registry
else
    echo "Invalid category: $FILTER_CATEGORY" >&2
    exit 2
fi

# Print summary
echo ""
echo "Summary"
echo "-------"
echo "Matches:     $MATCHES"
echo "Mismatches:  $MISMATCHES"
echo "Git only:    $GIT_ONLY"
echo ""

if [[ $MISMATCHES -gt 0 ]]; then
    echo "FAIL: Found $MISMATCHES mismatches"
    exit 1
fi

echo "PASS: All comparable files match"
exit 0
