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

# Auto-detect GitHub token if not set (for local development)
if [[ -z "${GITHUB_TOKEN:-}" ]] && command -v gh &>/dev/null; then
    GITHUB_TOKEN="$(gh auth token 2>/dev/null)" || true
    export GITHUB_TOKEN
fi

# Fail fast if no token available
if [[ -z "${GITHUB_TOKEN:-}" ]]; then
    echo "Error: GITHUB_TOKEN is not set and could not be detected from 'gh auth token'" >&2
    echo "Please set GITHUB_TOKEN or run 'gh auth login' first." >&2
    exit 2
fi

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

# Get supported platforms as JSON objects (preserving linux_family if present)
PLATFORMS_JSON=$("$TSUKU" info --recipe "$RECIPE_PATH" --metadata-only --json | \
    jq -c '.supported_platforms[]')

# Build platform descriptors (os:arch:family) and platform strings for file matching
PLATFORMS=""
while IFS= read -r platform_json; do
    os=$(echo "$platform_json" | jq -r '.os')
    arch=$(echo "$platform_json" | jq -r '.arch')
    family=$(echo "$platform_json" | jq -r '.linux_family // empty')

    # Skip linux-arm64 (no CI runner)
    if [[ "$os" == "linux" && "$arch" == "arm64" ]]; then
        continue
    fi

    # Add to platforms list (format: os:arch:family)
    PLATFORMS="$PLATFORMS $os:$arch:$family"
done <<< "$PLATFORMS_JSON"

PLATFORMS=$(echo "$PLATFORMS" | xargs)

# Extract versions from existing golden files
# For family-aware files: v1.0.0-linux-debian-amd64.json -> version is before last 3 components
# For family-agnostic files: v0.60.0-linux-amd64.json -> version is before last 2 components
VERSIONS=$(for f in "$GOLDEN_DIR"/*.json; do
    filename=$(basename "$f" .json)
    num_parts=$(echo "$filename" | tr '-' '\n' | wc -l)
    if [[ $num_parts -ge 4 ]]; then
        third_from_last=$(echo "$filename" | rev | cut -d'-' -f3 | rev)
        if [[ "$third_from_last" =~ ^(debian|rhel|arch|alpine|suse)$ ]]; then
            echo "$filename" | rev | cut -d'-' -f4- | rev
        else
            echo "$filename" | rev | cut -d'-' -f3- | rev
        fi
    else
        echo "$filename" | rev | cut -d'-' -f3- | rev
    fi
done 2>/dev/null | sort -u || true)

if [[ -z "$VERSIONS" ]]; then
    echo "No golden files found in $GOLDEN_DIR" >&2
    exit 2
fi

# Check that all supported platforms have golden files
MISSING_PLATFORMS=()
for VERSION in $VERSIONS; do
    for platform_desc in $PLATFORMS; do
        # Parse platform descriptor (os:arch:family)
        os="${platform_desc%%:*}"
        rest="${platform_desc#*:}"
        arch="${rest%%:*}"
        family="${rest#*:}"

        # Build expected filename
        if [[ -n "$family" ]]; then
            expected_file="${VERSION}-${os}-${family}-${arch}.json"
        else
            expected_file="${VERSION}-${os}-${arch}.json"
        fi

        GOLDEN="$GOLDEN_DIR/$expected_file"
        if [[ ! -f "$GOLDEN" ]]; then
            MISSING_PLATFORMS+=("$expected_file")
        fi
    done
done

if [[ ${#MISSING_PLATFORMS[@]} -gt 0 ]]; then
    echo "ERROR: Missing golden files for supported platforms:" >&2
    for missing in "${MISSING_PLATFORMS[@]}"; do
        echo "  - $GOLDEN_DIR/${missing}" >&2
    done
    echo "" >&2
    echo "To fix, either:" >&2
    echo "" >&2
    echo "  1. Generate locally (if you have the required toolchain):" >&2
    echo "     ./scripts/regenerate-golden.sh $RECIPE" >&2
    echo "" >&2
    echo "  2. Generate via CI (for cross-platform generation):" >&2
    echo "     gh workflow run generate-golden-files.yml -f recipe=$RECIPE -f commit_back=true -f branch=\$(git branch --show-current)" >&2
    exit 1
fi

MISMATCH=0

for VERSION in $VERSIONS; do
    VERSION_NO_V="${VERSION#v}"

    for platform_desc in $PLATFORMS; do
        # Parse platform descriptor (os:arch:family)
        os="${platform_desc%%:*}"
        rest="${platform_desc#*:}"
        arch="${rest%%:*}"
        family="${rest#*:}"

        # Build expected filename and actual filename
        if [[ -n "$family" ]]; then
            filename="${VERSION}-${os}-${family}-${arch}.json"
        else
            filename="${VERSION}-${os}-${arch}.json"
        fi

        GOLDEN="$GOLDEN_DIR/$filename"
        ACTUAL="$TEMP_DIR/$filename"

        # Build eval command arguments
        eval_args=(--recipe "$RECIPE_PATH" --os "$os" --arch "$arch" --version "$VERSION_NO_V" --install-deps)
        if [[ -n "$family" ]]; then
            eval_args+=(--linux-family "$family")
        fi

        # Generate current plan (stripping non-deterministic fields)
        # Note: missing platforms already caught by pre-check above
        if ! "$TSUKU" eval "${eval_args[@]}" 2>/dev/null | \
            jq 'del(.generated_at, .recipe_source)' > "$ACTUAL"; then
            echo "Failed to generate plan for $RECIPE@$VERSION ($filename)" >&2
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
    echo "Golden file validation failed. To fix, either:"
    echo ""
    echo "  1. Generate locally (if you have the required toolchain):"
    echo "     ./scripts/regenerate-golden.sh $RECIPE"
    echo ""
    echo "  2. Generate via CI (for cross-platform generation):"
    echo "     gh workflow run generate-golden-files.yml -f recipe=$RECIPE -f commit_back=true -f branch=\$(git branch --show-current)"
    exit 1
fi

echo "Golden files for $RECIPE are up to date."
exit 0
