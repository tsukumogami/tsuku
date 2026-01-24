#!/usr/bin/env bash
# Validate all golden files
# Usage: ./scripts/validate-all-golden.sh [--os <linux|darwin>] [--category <embedded|registry>]
#
# Runs validate-golden.sh for each recipe with golden files.
# Reports which recipes failed so you can investigate and selectively regenerate.
#
# Golden files are organized by category:
#   - Embedded recipes: testdata/golden/plans/embedded/<recipe>/
#   - Registry recipes: testdata/golden/plans/<letter>/<recipe>/
#
# Options:
#   --os <os>          Only validate golden files for the specified OS (linux or darwin)
#                      This is useful for platform-specific CI runners.
#   --category <cat>   Only validate recipes of the specified category (embedded or registry)
#                      If not specified, validates both categories.
#
# Exit codes:
#   0: All golden files match
#   1: One or more recipes have mismatches

set -euo pipefail

# Parse arguments
FILTER_OS=""
FILTER_CATEGORY=""
while [[ $# -gt 0 ]]; do
    case "$1" in
        --os)       FILTER_OS="$2"; shift 2 ;;
        --category) FILTER_CATEGORY="$2"; shift 2 ;;
        -h|--help)
            echo "Usage: $0 [--os <linux|darwin>] [--category <embedded|registry>]"
            echo ""
            echo "Validate all golden files."
            echo ""
            echo "Options:"
            echo "  --os <os>          Only validate golden files for the specified OS"
            echo "  --category <cat>   Only validate embedded or registry recipes"
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

# Validate embedded recipes (flat structure: embedded/<recipe>/)
validate_embedded() {
    local embedded_dir="$GOLDEN_BASE/embedded"
    if [[ ! -d "$embedded_dir" ]]; then
        return
    fi

    for recipe_dir in "$embedded_dir"/*/; do
        [[ -d "$recipe_dir" ]] || continue

        recipe=$(basename "$recipe_dir")
        TOTAL=$((TOTAL + 1))

        echo "Validating $recipe (embedded)..."
        VALIDATE_ARGS=("$recipe" "--category" "embedded")
        if [[ -n "$FILTER_OS" ]]; then
            VALIDATE_ARGS+=("--os" "$FILTER_OS")
        fi

        if ! "$SCRIPT_DIR/validate-golden.sh" "${VALIDATE_ARGS[@]}"; then
            FAILED+=("$recipe")
        fi
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
            TOTAL=$((TOTAL + 1))

            echo "Validating $recipe (registry)..."
            VALIDATE_ARGS=("$recipe" "--category" "registry")
            if [[ -n "$FILTER_OS" ]]; then
                VALIDATE_ARGS+=("--os" "$FILTER_OS")
            fi

            # Check if this is a testdata recipe (not in main recipes directory)
            EMBEDDED_RECIPE="$REPO_ROOT/internal/recipe/recipes/$recipe.toml"
            first_letter="${recipe:0:1}"
            REGISTRY_RECIPE="$REPO_ROOT/recipes/$first_letter/$recipe.toml"
            TESTDATA_RECIPE="$REPO_ROOT/testdata/recipes/$recipe.toml"
            if [[ ! -f "$EMBEDDED_RECIPE" && ! -f "$REGISTRY_RECIPE" && -f "$TESTDATA_RECIPE" ]]; then
                VALIDATE_ARGS+=("--recipe" "$TESTDATA_RECIPE")
            fi

            if ! "$SCRIPT_DIR/validate-golden.sh" "${VALIDATE_ARGS[@]}"; then
                FAILED+=("$recipe")
            fi
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
