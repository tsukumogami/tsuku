#!/usr/bin/env bash
# Regenerate golden files for a single recipe
# Usage: ./scripts/regenerate-golden.sh <recipe> [--version <ver>] [--os <os>] [--arch <arch>]
#
# Examples:
#   ./scripts/regenerate-golden.sh fzf
#   ./scripts/regenerate-golden.sh fzf --version 0.60.0
#   ./scripts/regenerate-golden.sh fzf --os linux --arch amd64
#
# Exit codes:
#   0: Success
#   1: Invalid arguments or recipe not found
#   2: No platforms match filters

set -euo pipefail

# Auto-detect GitHub token if not set (for local development)
if [[ -z "${GITHUB_TOKEN:-}" ]] && command -v gh &>/dev/null; then
    GITHUB_TOKEN="$(gh auth token 2>/dev/null)" || true
    export GITHUB_TOKEN
fi

# Fail fast if no token available
if [[ -z "${GITHUB_TOKEN:-}" ]]; then
    echo "Error: GITHUB_TOKEN is not set and could not be detected from 'gh auth token'" >&2
    echo "Please set GITHUB_TOKEN or run 'gh auth login' first." >&2
    exit 1
fi

# Script location for relative paths
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Paths
RECIPE_BASE="$REPO_ROOT/internal/recipe/recipes"
GOLDEN_BASE="$REPO_ROOT/testdata/golden/plans"
TSUKU="$REPO_ROOT/tsuku"

# Parse arguments
RECIPE=""
FILTER_VERSION=""
FILTER_OS=""
FILTER_ARCH=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --version) FILTER_VERSION="$2"; shift 2 ;;
        --os)      FILTER_OS="$2"; shift 2 ;;
        --arch)    FILTER_ARCH="$2"; shift 2 ;;
        -h|--help)
            echo "Usage: $0 <recipe> [--version <ver>] [--os <os>] [--arch <arch>]"
            echo ""
            echo "Regenerate golden files for a recipe."
            echo ""
            echo "Options:"
            echo "  --version <ver>  Only regenerate for specific version"
            echo "  --os <os>        Only regenerate for specific OS (linux, darwin)"
            echo "  --arch <arch>    Only regenerate for specific arch (amd64, arm64)"
            exit 0
            ;;
        -*)        echo "Unknown flag: $1" >&2; exit 1 ;;
        *)         RECIPE="$1"; shift ;;
    esac
done

# Validate arguments
if [[ -z "$RECIPE" ]]; then
    echo "Usage: $0 <recipe> [--version <ver>] [--os <os>] [--arch <arch>]" >&2
    exit 1
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
    exit 1
fi

# Create golden directory
mkdir -p "$GOLDEN_DIR"

# Get supported platforms (format: linux/amd64)
# Exclude linux-arm64 (no CI runner available)
ALL_PLATFORMS=$("$TSUKU" info --recipe "$RECIPE_PATH" --metadata-only --json | \
    jq -r '.supported_platforms[]' | tr '/' '-' | grep -v '^linux-arm64$' || true)

if [[ -z "$ALL_PLATFORMS" ]]; then
    echo "No supported platforms found for $RECIPE (excluding linux-arm64)"
    exit 0
fi

# Apply platform filters
PLATFORMS=""
for platform in $ALL_PLATFORMS; do
    os="${platform%-*}"
    arch="${platform#*-}"

    if [[ -n "$FILTER_OS" && "$os" != "$FILTER_OS" ]]; then
        continue
    fi

    if [[ -n "$FILTER_ARCH" && "$arch" != "$FILTER_ARCH" ]]; then
        continue
    fi

    PLATFORMS="$PLATFORMS $platform"
done

PLATFORMS=$(echo "$PLATFORMS" | xargs)

if [[ -z "$PLATFORMS" ]]; then
    echo "No platforms match filters (--os=$FILTER_OS, --arch=$FILTER_ARCH)" >&2
    exit 2
fi

# Determine versions to regenerate
if [[ -n "$FILTER_VERSION" ]]; then
    # Normalize version (add v prefix if missing for filename)
    if [[ "$FILTER_VERSION" != v* ]]; then
        VERSION_FOR_FILE="v$FILTER_VERSION"
    else
        VERSION_FOR_FILE="$FILTER_VERSION"
    fi
    VERSIONS="$VERSION_FOR_FILE"
elif [[ -d "$GOLDEN_DIR" ]] && ls "$GOLDEN_DIR"/*.json >/dev/null 2>&1; then
    # Extract versions from existing files (with v prefix)
    VERSIONS=$(ls "$GOLDEN_DIR"/*.json | sed 's/.*\/\(v[^-]*\)-.*/\1/' | sort -u)
else
    # Get latest version
    LATEST=$("$TSUKU" versions "$RECIPE" 2>/dev/null | grep -E '^\s+v' | head -1 | xargs || true)
    if [[ -z "$LATEST" ]]; then
        echo "Could not resolve latest version for $RECIPE" >&2
        exit 1
    fi
    VERSIONS="$LATEST"
fi

# Regenerate for each version/platform combination
for VERSION in $VERSIONS; do
    # Remove v prefix for tsuku eval (it expects version without v)
    VERSION_NO_V="${VERSION#v}"

    echo "Regenerating $RECIPE@$VERSION..."

    for platform in $PLATFORMS; do
        os="${platform%-*}"
        arch="${platform#*-}"
        OUTPUT="$GOLDEN_DIR/${VERSION}-${platform}.json"

        if "$TSUKU" eval --recipe "$RECIPE_PATH" --os "$os" --arch "$arch" \
            --version "$VERSION_NO_V" --yes 2>/dev/null | \
            jq 'del(.generated_at, .recipe_source)' > "$OUTPUT.tmp"; then
            mv "$OUTPUT.tmp" "$OUTPUT"
            echo "  Generated: $OUTPUT"
        else
            rm -f "$OUTPUT.tmp"
            echo "  Failed: $OUTPUT" >&2
        fi
    done
done

# Clean up files for unsupported platforms (only when no filters applied)
if [[ -z "$FILTER_OS" && -z "$FILTER_ARCH" && -z "$FILTER_VERSION" ]]; then
    if [[ -d "$GOLDEN_DIR" ]]; then
        find "$GOLDEN_DIR" -name "*.json" | while read -r file; do
            # Extract platform from filename (e.g., v0.60.0-linux-amd64.json -> linux-amd64)
            filename=$(basename "$file")
            platform=$(echo "$filename" | sed 's/v[^-]*-//' | sed 's/\.json$//')

            if ! echo "$ALL_PLATFORMS" | grep -qw "$platform"; then
                echo "  Removing unsupported: $file"
                rm -f "$file"
            fi
        done
    fi
fi

echo "Done."
