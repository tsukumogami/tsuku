#!/usr/bin/env bash
# Validate all golden files
# Usage: ./scripts/validate-all-golden.sh
#
# Runs validate-golden.sh for each recipe with golden files.
# Reports which recipes failed so you can investigate and selectively regenerate.
#
# Exit codes:
#   0: All golden files match
#   1: One or more recipes have mismatches

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

        echo "Validating $recipe..."
        if ! "$SCRIPT_DIR/validate-golden.sh" "$recipe"; then
            FAILED+=("$recipe")
        fi
    done
done

if [[ $TOTAL -eq 0 ]]; then
    echo "No recipes with golden files found."
    exit 0
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
    exit 1
fi

echo ""
echo "All $TOTAL recipes validated successfully."
exit 0
