#!/usr/bin/env bash
# Regenerate all golden files
# Usage: ./scripts/regenerate-all-golden.sh [--category <embedded|registry>]
#
# Runs regenerate-golden.sh for each recipe with golden files.
# Use this when code changes require full regeneration.
#
# Golden files are organized by category:
#   - Embedded recipes: testdata/golden/plans/embedded/<recipe>/
#   - Registry recipes: testdata/golden/plans/<letter>/<recipe>/
#
# Options:
#   --category <cat>   Only regenerate recipes of the specified category (embedded or registry)
#                      If not specified, regenerates both categories.
#
# Exit codes:
#   0: Success
#   1: One or more recipes failed to regenerate

set -euo pipefail

# Parse arguments
FILTER_CATEGORY=""
while [[ $# -gt 0 ]]; do
    case "$1" in
        --category) FILTER_CATEGORY="$2"; shift 2 ;;
        -h|--help)
            echo "Usage: $0 [--category <embedded|registry>]"
            echo ""
            echo "Regenerate all golden files."
            echo ""
            echo "Options:"
            echo "  --category <cat>   Only regenerate embedded or registry recipes"
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

# Paths
GOLDEN_BASE="$REPO_ROOT/testdata/golden/plans"

# Check golden directory exists
if [[ ! -d "$GOLDEN_BASE" ]]; then
    echo "No golden files directory found: $GOLDEN_BASE"
    exit 0
fi

FAILED=()
TOTAL=0

# Regenerate embedded recipes (flat structure: embedded/<recipe>/)
regenerate_embedded() {
    local embedded_dir="$GOLDEN_BASE/embedded"
    if [[ ! -d "$embedded_dir" ]]; then
        return
    fi

    for recipe_dir in "$embedded_dir"/*/; do
        [[ -d "$recipe_dir" ]] || continue

        recipe=$(basename "$recipe_dir")
        TOTAL=$((TOTAL + 1))

        echo ""
        echo "========================================"
        echo "Regenerating $recipe (embedded)..."
        echo "========================================"
        if ! "$SCRIPT_DIR/regenerate-golden.sh" "$recipe" --category embedded; then
            FAILED+=("$recipe")
            echo "  FAILED: $recipe"
        fi
    done
}

# Regenerate registry recipes (letter-based structure: <letter>/<recipe>/)
regenerate_registry() {
    for letter_dir in "$GOLDEN_BASE"/[a-z]/; do
        [[ -d "$letter_dir" ]] || continue

        # Iterate over recipe directories within each letter
        for recipe_dir in "$letter_dir"*/; do
            [[ -d "$recipe_dir" ]] || continue

            recipe=$(basename "$recipe_dir")
            TOTAL=$((TOTAL + 1))

            echo ""
            echo "========================================"
            echo "Regenerating $recipe (registry)..."
            echo "========================================"
            if ! "$SCRIPT_DIR/regenerate-golden.sh" "$recipe" --category registry; then
                FAILED+=("$recipe")
                echo "  FAILED: $recipe"
            fi
        done
    done
}

# Run regeneration based on category filter
if [[ -z "$FILTER_CATEGORY" ]]; then
    # Regenerate both categories
    regenerate_embedded
    regenerate_registry
elif [[ "$FILTER_CATEGORY" == "embedded" ]]; then
    regenerate_embedded
elif [[ "$FILTER_CATEGORY" == "registry" ]]; then
    regenerate_registry
fi

if [[ $TOTAL -eq 0 ]]; then
    echo "No recipes with golden files found."
    exit 0
fi

echo ""
echo "========================================"
echo "SUMMARY"
echo "========================================"

if [[ ${#FAILED[@]} -gt 0 ]]; then
    echo ""
    echo "Failed recipes (${#FAILED[@]} of $TOTAL):"
    for recipe in "${FAILED[@]}"; do
        echo "  - $recipe"
    done
    exit 1
fi

echo ""
echo "Successfully regenerated $TOTAL recipes."
exit 0
