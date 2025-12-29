#!/usr/bin/env bash
# Regenerate all golden files
# Usage: ./scripts/regenerate-all-golden.sh
#
# Runs regenerate-golden.sh for each recipe with golden files.
# Use this when code changes require full regeneration.
#
# Exit codes:
#   0: Success
#   1: One or more recipes failed to regenerate

set -euo pipefail

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

# Iterate over first-letter directories
for letter_dir in "$GOLDEN_BASE"/*/; do
    [[ -d "$letter_dir" ]] || continue

    # Iterate over recipe directories within each letter
    for recipe_dir in "$letter_dir"*/; do
        [[ -d "$recipe_dir" ]] || continue

        recipe=$(basename "$recipe_dir")
        TOTAL=$((TOTAL + 1))

        echo ""
        echo "========================================"
        echo "Regenerating $recipe..."
        echo "========================================"
        if ! "$SCRIPT_DIR/regenerate-golden.sh" "$recipe"; then
            FAILED+=("$recipe")
            echo "  FAILED: $recipe"
        fi
    done
done

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
