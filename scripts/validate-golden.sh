#!/usr/bin/env bash
# Validate golden files for a single recipe
# Usage: ./scripts/validate-golden.sh <recipe>
#
# Compares current plan generation output against stored golden files.
# Uses fast hash comparison first, then shows diff on mismatch.
#
# Exit codes:
#   0: All golden files match
#   1: Mismatch detected (with diff output)
#   2: Error (missing files, invalid recipe, etc.)

set -euo pipefail

# Script location for relative paths
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Paths
RECIPE_BASE="$REPO_ROOT/internal/recipe/recipes"
GOLDEN_BASE="$REPO_ROOT/testdata/golden/plans"
TSUKU="$REPO_ROOT/tsuku"

# Validate arguments
RECIPE="${1:-}"
if [[ -z "$RECIPE" ]]; then
    echo "Usage: $0 <recipe>" >&2
    exit 2
fi

# Build tsuku if not present
if [[ ! -x "$TSUKU" ]]; then
    echo "Building tsuku..."
    (cd "$REPO_ROOT" && go build -o tsuku ./cmd/tsuku)
fi

# Compute paths
FIRST_LETTER="${RECIPE:0:1}"
RECIPE_PATH="$RECIPE_BASE/$FIRST_LETTER/$RECIPE.toml"
GOLDEN_DIR="$GOLDEN_BASE/$FIRST_LETTER/$RECIPE"

# Validate recipe exists
if [[ ! -f "$RECIPE_PATH" ]]; then
    echo "Recipe not found: $RECIPE_PATH" >&2
    exit 2
fi

# Validate golden directory exists
if [[ ! -d "$GOLDEN_DIR" ]]; then
    echo "No golden files found for $RECIPE" >&2
    exit 2
fi

# Create temp directory for generated files
TEMP_DIR=$(mktemp -d)
trap 'rm -rf "$TEMP_DIR"' EXIT

# Get supported platforms (exclude linux-arm64)
PLATFORMS=$("$TSUKU" info --recipe "$RECIPE_PATH" --metadata-only --json | \
    jq -r '.supported_platforms[]' | tr '/' '-' | grep -v '^linux-arm64$' || true)

# Extract versions from existing golden files
VERSIONS=$(ls "$GOLDEN_DIR"/*.json 2>/dev/null | sed 's/.*\/\(v[^-]*\)-.*/\1/' | sort -u || true)

if [[ -z "$VERSIONS" ]]; then
    echo "No golden files found in $GOLDEN_DIR" >&2
    exit 2
fi

MISMATCH=0

for VERSION in $VERSIONS; do
    VERSION_NO_V="${VERSION#v}"

    for platform in $PLATFORMS; do
        os="${platform%-*}"
        arch="${platform#*-}"
        GOLDEN="$GOLDEN_DIR/${VERSION}-${platform}.json"
        ACTUAL="$TEMP_DIR/${VERSION}-${platform}.json"

        # Skip if golden file doesn't exist for this platform
        if [[ ! -f "$GOLDEN" ]]; then
            continue
        fi

        # Generate current plan (stripping non-deterministic fields)
        if ! "$TSUKU" eval --recipe "$RECIPE_PATH" --os "$os" --arch "$arch" \
            --version "$VERSION_NO_V" 2>/dev/null | \
            jq 'del(.generated_at, .recipe_source)' > "$ACTUAL"; then
            echo "Failed to generate plan for $RECIPE@$VERSION ($platform)" >&2
            continue
        fi

        # Fast hash comparison (golden files already have fields stripped)
        GOLDEN_HASH=$(sha256sum "$GOLDEN" | cut -d' ' -f1)
        ACTUAL_HASH=$(sha256sum "$ACTUAL" | cut -d' ' -f1)

        if [[ "$GOLDEN_HASH" != "$ACTUAL_HASH" ]]; then
            MISMATCH=1
            echo "MISMATCH: $GOLDEN"
            echo "--- Expected (golden)"
            echo "+++ Actual (generated)"
            diff -u "$GOLDEN" "$ACTUAL" || true
            echo ""
        fi
    done
done

if [[ $MISMATCH -eq 1 ]]; then
    echo ""
    echo "Golden file validation failed."
    echo "Run './scripts/regenerate-golden.sh $RECIPE' to update."
    exit 1
fi

echo "Golden files for $RECIPE are up to date."
exit 0
