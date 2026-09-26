#!/usr/bin/env bash
# Validate all golden files
# Usage: ./scripts/validate-all-golden.sh [--os <linux|darwin>] [--category <embedded|registry>] [--golden-dir <path>] [--shard <i>/<n>]
#
# Runs validate-golden.sh for each recipe with golden files.
# Reports which recipes failed so you can investigate and selectively regenerate.
#
# Golden files are organized by category:
#   - Embedded recipes: <golden-base>/embedded/<recipe>/
#   - Registry recipes: <golden-base>/<letter>/<recipe>/
#
# Options:
#   --os <os>          Only validate golden files for the specified OS (linux or darwin)
#                      This is useful for platform-specific CI runners.
#   --category <cat>   Only validate recipes of the specified category (embedded or registry)
#                      If not specified, validates both categories.
#   --golden-dir <dir> Use custom golden files directory instead of testdata/golden/plans
#                      Useful for validating against R2-downloaded golden files.
#   --shard <i>/<n>    Only validate every n-th recipe starting at index i (0-based), in
#                      the order the recipes are found. Shards 0/n..(n-1)/n together
#                      cover every recipe exactly once.
#
# Environment Variables:
#   TSUKU_GOLDEN_SOURCE  Select golden file source (passed to validate-golden.sh):
#                        - git (default): Use git-based golden files
#                        - r2: Download from R2 and validate against those
#                        - both: Validate against git, then compare with R2
#
#   R2_BUCKET_URL        Required for r2/both modes
#   R2_ACCESS_KEY_ID     Required for r2/both modes
#   R2_SECRET_ACCESS_KEY Required for r2/both modes
#
# Exit codes:
#   0: Every recipe checked matched, and at least one was compared
#   1: One or more recipes have mismatches
#   3: Nothing was compared (no golden files found, or none applied to --os). A run
#      that compared nothing is not a pass.
#
# The last line of output is always the summary, in this fixed form:
#   Checked <T> recipes: <P> matched, <F> failed, <N> not compared

set -euo pipefail

# Parse arguments
FILTER_OS=""
FILTER_CATEGORY=""
CUSTOM_GOLDEN_DIR=""
SHARD_INDEX=0
SHARD_COUNT=1
while [[ $# -gt 0 ]]; do
    case "$1" in
        --os)         FILTER_OS="$2"; shift 2 ;;
        --category)   FILTER_CATEGORY="$2"; shift 2 ;;
        --golden-dir) CUSTOM_GOLDEN_DIR="$2"; shift 2 ;;
        --shard)
            if [[ ! "$2" =~ ^([0-9]+)/([1-9][0-9]*)$ ]] || (( BASH_REMATCH[1] >= BASH_REMATCH[2] )); then
                echo "Invalid --shard: $2 (expected <i>/<n> with 0 <= i < n)" >&2
                exit 1
            fi
            SHARD_INDEX="${BASH_REMATCH[1]}"
            SHARD_COUNT="${BASH_REMATCH[2]}"
            shift 2
            ;;
        -h|--help)
            echo "Usage: $0 [--os <linux|darwin>] [--category <embedded|registry>] [--golden-dir <path>] [--shard <i>/<n>]"
            echo ""
            echo "Validate all golden files."
            echo ""
            echo "Options:"
            echo "  --os <os>          Only validate golden files for the specified OS"
            echo "  --category <cat>   Only validate embedded or registry recipes"
            echo "  --golden-dir <dir> Use custom golden files directory"
            echo "  --shard <i>/<n>    Only validate recipes whose index modulo n is i"
            exit 0
            ;;
        *)         echo "Unknown argument: $1" >&2; exit 1 ;;
    esac
done

# Validate category argument
if [[ -n "$FILTER_CATEGORY" && "$FILTER_CATEGORY" != "embedded" && "$FILTER_CATEGORY" != "registry" ]]; then
    echo "Invalid category: $FILTER_CATEGORY (must be 'embedded' or 'registry')" >&2
    exit 1
fi

# Script location for relative paths
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Paths - use custom golden dir if specified
if [[ -n "$CUSTOM_GOLDEN_DIR" ]]; then
    # Convert to absolute path if relative
    if [[ "$CUSTOM_GOLDEN_DIR" = /* ]]; then
        GOLDEN_BASE="$CUSTOM_GOLDEN_DIR"
    else
        GOLDEN_BASE="$REPO_ROOT/$CUSTOM_GOLDEN_DIR"
    fi
else
    GOLDEN_BASE="$REPO_ROOT/testdata/golden/plans"
fi

# Check golden directory exists
if [[ ! -d "$GOLDEN_BASE" ]]; then
    echo "NOTHING COMPARED: no golden files directory found: $GOLDEN_BASE"
    echo "Checked 0 recipes: 0 matched, 0 failed, 0 not compared"
    exit 3
fi

FAILED=()
NOT_COMPARED=()
TOTAL=0
MATCHED=0
FOUND=0

# in_shard: counts every recipe found, and succeeds for the ones this shard owns.
in_shard() {
    local index=$FOUND
    FOUND=$((FOUND + 1))
    (( index % SHARD_COUNT == SHARD_INDEX ))
}

# Run validate-golden.sh for one recipe and file the result. Exit 3 from it means no
# platform applied, which is counted on its own rather than as a match.
run_validation() {
    local recipe="$1"
    shift
    local rc=0
    "$SCRIPT_DIR/validate-golden.sh" "$@" || rc=$?
    case $rc in
        0) MATCHED=$((MATCHED + 1)) ;;
        3) NOT_COMPARED+=("$recipe") ;;
        *) FAILED+=("$recipe") ;;
    esac
}

print_summary() {
    if [[ $SHARD_COUNT -gt 1 ]]; then
        echo "Shard $SHARD_INDEX/$SHARD_COUNT owns $TOTAL of $FOUND recipes found"
    fi
    echo "Checked $TOTAL recipes: $MATCHED matched, ${#FAILED[@]} failed, ${#NOT_COMPARED[@]} not compared"
}

# Validate embedded recipes (flat structure: embedded/<recipe>/)
validate_embedded() {
    local embedded_dir="$GOLDEN_BASE/embedded"
    if [[ ! -d "$embedded_dir" ]]; then
        return
    fi

    for recipe_dir in "$embedded_dir"/*/; do
        [[ -d "$recipe_dir" ]] || continue

        recipe=$(basename "$recipe_dir")
        in_shard || continue
        TOTAL=$((TOTAL + 1))

        echo "Validating $recipe (embedded)..."
        VALIDATE_ARGS=("$recipe" "--category" "embedded")
        if [[ -n "$FILTER_OS" ]]; then
            VALIDATE_ARGS+=("--os" "$FILTER_OS")
        fi
        if [[ -n "$CUSTOM_GOLDEN_DIR" ]]; then
            VALIDATE_ARGS+=("--golden-dir" "$CUSTOM_GOLDEN_DIR")
        fi

        run_validation "$recipe" "${VALIDATE_ARGS[@]}"
    done
}

# Validate registry recipes (letter-based structure: <letter>/<recipe>/)
validate_registry() {
    for letter_dir in "$GOLDEN_BASE"/[a-z]/; do
        [[ -d "$letter_dir" ]] || continue

        # Iterate over recipe directories within each letter
        for recipe_dir in "$letter_dir"*/; do
            [[ -d "$recipe_dir" ]] || continue

            recipe=$(basename "$recipe_dir")
            in_shard || continue
            TOTAL=$((TOTAL + 1))

            echo "Validating $recipe (registry)..."
            VALIDATE_ARGS=("$recipe" "--category" "registry")
            if [[ -n "$FILTER_OS" ]]; then
                VALIDATE_ARGS+=("--os" "$FILTER_OS")
            fi
            if [[ -n "$CUSTOM_GOLDEN_DIR" ]]; then
                VALIDATE_ARGS+=("--golden-dir" "$CUSTOM_GOLDEN_DIR")
            fi

            # Check if this is a testdata recipe (not in main recipes directory)
            EMBEDDED_RECIPE="$REPO_ROOT/internal/recipe/recipes/$recipe.toml"
            first_letter="${recipe:0:1}"
            REGISTRY_RECIPE="$REPO_ROOT/recipes/$first_letter/$recipe.toml"
            TESTDATA_RECIPE="$REPO_ROOT/testdata/recipes/$recipe.toml"
            if [[ ! -f "$EMBEDDED_RECIPE" && ! -f "$REGISTRY_RECIPE" && -f "$TESTDATA_RECIPE" ]]; then
                VALIDATE_ARGS+=("--recipe" "$TESTDATA_RECIPE")
            fi

            run_validation "$recipe" "${VALIDATE_ARGS[@]}"
        done
    done
}

# Run validation based on category filter
if [[ -z "$FILTER_CATEGORY" ]]; then
    # Validate both categories
    validate_embedded
    validate_registry
elif [[ "$FILTER_CATEGORY" == "embedded" ]]; then
    validate_embedded
elif [[ "$FILTER_CATEGORY" == "registry" ]]; then
    validate_registry
fi

if [[ $TOTAL -eq 0 ]]; then
    echo "NOTHING COMPARED: no recipes with golden files found under $GOLDEN_BASE."
    print_summary
    exit 3
fi

if [[ ${#FAILED[@]} -gt 0 ]]; then
    echo ""
    echo "========================================"
    echo "VALIDATION FAILED"
    echo "========================================"
    echo ""
    echo "Failed recipes (${#FAILED[@]} of $TOTAL):"
    for recipe in "${FAILED[@]}"; do
        echo "  - $recipe"
    done
    echo ""
    echo "To regenerate specific recipes:"
    for recipe in "${FAILED[@]}"; do
        echo "  ./scripts/regenerate-golden.sh $recipe"
    done
    echo ""
    echo "To regenerate with constraints:"
    echo "  ./scripts/regenerate-golden.sh <recipe> --os linux --arch amd64"
    echo "  ./scripts/regenerate-golden.sh <recipe> --version v1.2.3"
    echo ""
    print_summary
    exit 1
fi

if [[ ${#NOT_COMPARED[@]} -gt 0 ]]; then
    echo ""
    echo "Not compared (no golden-checked platform${FILTER_OS:+ for --os $FILTER_OS}): ${NOT_COMPARED[*]}"
fi

if [[ $MATCHED -eq 0 ]]; then
    echo ""
    echo "NOTHING COMPARED: $TOTAL recipes found, none had a platform to compare."
    print_summary
    exit 3
fi

echo ""
print_summary
exit 0
