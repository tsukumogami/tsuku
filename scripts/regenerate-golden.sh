#!/usr/bin/env bash
# Regenerate golden files for a single recipe
# Usage: ./scripts/regenerate-golden.sh <recipe> [--version <ver>] [--os <os>] [--arch <arch>] [--recipe <path>] [--category <embedded|registry>]
#
# Golden files are organized by category:
#   - Embedded recipes: testdata/golden/plans/embedded/<recipe>/
#   - Registry recipes: testdata/golden/plans/<letter>/<recipe>/
#
# Examples:
#   ./scripts/regenerate-golden.sh go                    # auto-detects embedded
#   ./scripts/regenerate-golden.sh fzf                   # auto-detects registry
#   ./scripts/regenerate-golden.sh fzf --version 0.60.0
#   ./scripts/regenerate-golden.sh fzf --os linux --arch amd64
#   ./scripts/regenerate-golden.sh build-tools-system --recipe testdata/recipes/build-tools-system.toml
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
CUSTOM_RECIPE_PATH=""
CATEGORY=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --version)   FILTER_VERSION="$2"; shift 2 ;;
        --os)        FILTER_OS="$2"; shift 2 ;;
        --arch)      FILTER_ARCH="$2"; shift 2 ;;
        --recipe)    CUSTOM_RECIPE_PATH="$2"; shift 2 ;;
        --category)  CATEGORY="$2"; shift 2 ;;
        -h|--help)
            echo "Usage: $0 <recipe> [--version <ver>] [--os <os>] [--arch <arch>] [--recipe <path>] [--category <embedded|registry>]"
            echo ""
            echo "Regenerate golden files for a recipe."
            echo ""
            echo "Options:"
            echo "  --version <ver>   Only regenerate for specific version"
            echo "  --os <os>         Only regenerate for specific OS (linux, darwin)"
            echo "  --arch <arch>     Only regenerate for specific arch (amd64, arm64)"
            echo "  --recipe <path>   Use custom recipe path (e.g., testdata/recipes/foo.toml)"
            echo "  --category <cat>  Force category (embedded or registry). Auto-detected if not specified."
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

# Detect recipe category (embedded or registry)
# Embedded recipes are in internal/recipe/recipes/<name>.toml (flat)
# Registry recipes are in recipes/<letter>/<name>.toml
detect_category() {
    local recipe="$1"
    local embedded_path="$REPO_ROOT/internal/recipe/recipes/$recipe.toml"
    local first_letter="${recipe:0:1}"
    local registry_path="$REPO_ROOT/recipes/$first_letter/$recipe.toml"

    if [[ -f "$embedded_path" ]]; then
        echo "embedded"
    elif [[ -f "$registry_path" ]]; then
        echo "registry"
    else
        # Default to registry for unknown recipes (testdata, etc.)
        echo "registry"
    fi
}

# Get golden directory path based on category
# Embedded: testdata/golden/plans/embedded/<recipe>/
# Registry: testdata/golden/plans/<letter>/<recipe>/
get_golden_dir() {
    local recipe="$1"
    local category="$2"
    local first_letter="${recipe:0:1}"

    if [[ "$category" == "embedded" ]]; then
        echo "$GOLDEN_BASE/embedded/$recipe"
    else
        echo "$GOLDEN_BASE/$first_letter/$recipe"
    fi
}

# Compute paths
FIRST_LETTER="${RECIPE:0:1}"
if [[ -n "$CUSTOM_RECIPE_PATH" ]]; then
    # Use custom recipe path (convert to absolute if relative)
    if [[ "$CUSTOM_RECIPE_PATH" = /* ]]; then
        RECIPE_PATH="$CUSTOM_RECIPE_PATH"
    else
        RECIPE_PATH="$REPO_ROOT/$CUSTOM_RECIPE_PATH"
    fi
else
    # Try embedded first (flat), then registry (letter-based)
    EMBEDDED_PATH="$REPO_ROOT/internal/recipe/recipes/$RECIPE.toml"
    REGISTRY_PATH="$REPO_ROOT/recipes/$FIRST_LETTER/$RECIPE.toml"
    if [[ -f "$EMBEDDED_PATH" ]]; then
        RECIPE_PATH="$EMBEDDED_PATH"
    else
        RECIPE_PATH="$REGISTRY_PATH"
    fi
fi

# Auto-detect category if not specified
if [[ -z "$CATEGORY" ]]; then
    CATEGORY=$(detect_category "$RECIPE")
fi

GOLDEN_DIR=$(get_golden_dir "$RECIPE" "$CATEGORY")

# Validate recipe exists
if [[ ! -f "$RECIPE_PATH" ]]; then
    echo "Recipe not found: $RECIPE_PATH" >&2
    exit 1
fi

# Create golden directory
mkdir -p "$GOLDEN_DIR"

# Get supported platforms as JSON objects (preserving linux_family if present)
# Format: {"os":"linux","arch":"amd64"} or {"os":"linux","arch":"amd64","linux_family":"debian"}
PLATFORMS_JSON=$("$TSUKU" info --recipe "$RECIPE_PATH" --metadata-only --json | \
    jq -c '.supported_platforms[]')

if [[ -z "$PLATFORMS_JSON" ]]; then
    echo "No supported platforms found for $RECIPE"
    exit 0
fi

# Apply platform filters and build list of platform descriptors
# Each descriptor is: os:arch:family (family may be empty)
PLATFORMS=""
ALL_PLATFORMS=""  # For cleanup logic later
while IFS= read -r platform_json; do
    os=$(echo "$platform_json" | jq -r '.os')
    arch=$(echo "$platform_json" | jq -r '.arch')
    family=$(echo "$platform_json" | jq -r '.linux_family // empty')

    # Skip linux-arm64 (no CI runner)
    if [[ "$os" == "linux" && "$arch" == "arm64" ]]; then
        continue
    fi

    # Build platform string for cleanup check (family-aware uses os-family-arch, agnostic uses os-arch)
    if [[ -n "$family" ]]; then
        ALL_PLATFORMS="$ALL_PLATFORMS $os-$family-$arch"
    else
        ALL_PLATFORMS="$ALL_PLATFORMS $os-$arch"
    fi

    # Apply filters
    if [[ -n "$FILTER_OS" && "$os" != "$FILTER_OS" ]]; then
        continue
    fi
    if [[ -n "$FILTER_ARCH" && "$arch" != "$FILTER_ARCH" ]]; then
        continue
    fi

    # Add to filtered platforms list (format: os:arch:family)
    PLATFORMS="$PLATFORMS $os:$arch:$family"
done <<< "$PLATFORMS_JSON"

PLATFORMS=$(echo "$PLATFORMS" | xargs)
ALL_PLATFORMS=$(echo "$ALL_PLATFORMS" | xargs)

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
    # For family-aware files: v1.0.0-linux-debian-amd64.json -> version is before last 3 components
    # For family-agnostic files: v0.60.0-linux-amd64.json -> version is before last 2 components
    # Detection: check if second-from-last component is a known family (e.g., debian in linux-debian-amd64)
    VERSIONS=$(for f in "$GOLDEN_DIR"/*.json; do
        filename=$(basename "$f" .json)
        # Count components
        num_parts=$(echo "$filename" | tr '-' '\n' | wc -l)
        if [[ $num_parts -ge 4 ]]; then
            # Check second-from-last: in linux-debian-amd64, that's "debian"
            second_from_last=$(echo "$filename" | rev | cut -d'-' -f2 | rev)
            if [[ "$second_from_last" =~ ^(debian|rhel|arch|alpine|suse)$ ]]; then
                # Family-aware: version is everything before last 3 components (os-family-arch)
                echo "$filename" | rev | cut -d'-' -f4- | rev
            else
                # Family-agnostic: version is everything before last 2 components (os-arch)
                echo "$filename" | rev | cut -d'-' -f3- | rev
            fi
        else
            # Family-agnostic: version is everything before last 2 components
            echo "$filename" | rev | cut -d'-' -f3- | rev
        fi
    done | sort -u)
else
    # Get latest version (versions may or may not have 'v' prefix depending on source)
    LATEST=$("$TSUKU" versions "$RECIPE" 2>/dev/null | grep -E '^\s+' | head -1 | xargs || true)
    if [[ -z "$LATEST" ]]; then
        echo "Could not resolve latest version for $RECIPE" >&2
        exit 1
    fi
    # Ensure version has v prefix for filename consistency
    if [[ "$LATEST" != v* ]]; then
        LATEST="v$LATEST"
    fi
    VERSIONS="$LATEST"
fi

# Regenerate for each version/platform combination
for VERSION in $VERSIONS; do
    # Remove v prefix for tsuku eval (it expects version without v)
    VERSION_NO_V="${VERSION#v}"

    echo "Regenerating $RECIPE@$VERSION..."

    for platform_desc in $PLATFORMS; do
        # Parse platform descriptor (os:arch:family)
        os="${platform_desc%%:*}"
        rest="${platform_desc#*:}"
        arch="${rest%%:*}"
        family="${rest#*:}"

        # Build eval command arguments
        eval_args=(--recipe "$RECIPE_PATH" --os "$os" --arch "$arch" --version "$VERSION_NO_V" --install-deps)

        # Determine output filename based on whether family is present
        if [[ -n "$family" ]]; then
            eval_args+=(--linux-family "$family")
            OUTPUT="$GOLDEN_DIR/${VERSION}-${os}-${family}-${arch}.json"
        else
            OUTPUT="$GOLDEN_DIR/${VERSION}-${os}-${arch}.json"
        fi

        if "$TSUKU" eval "${eval_args[@]}" 2>/dev/null | \
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
            # Extract platform from filename
            # Family-aware: v1.0.0-linux-debian-amd64.json -> linux-debian-amd64
            # Family-agnostic: v0.60.0-linux-amd64.json -> linux-amd64
            # Detection: check if second-from-last component is a known family
            filename=$(basename "$file" .json)
            num_parts=$(echo "$filename" | tr '-' '\n' | wc -l)

            if [[ $num_parts -ge 4 ]]; then
                # Check second-from-last: in linux-debian-amd64, that's "debian"
                second_from_last=$(echo "$filename" | rev | cut -d'-' -f2 | rev)
                if [[ "$second_from_last" =~ ^(debian|rhel|arch|alpine|suse)$ ]]; then
                    # Family-aware: platform is last 3 components (os-family-arch)
                    platform=$(echo "$filename" | rev | cut -d'-' -f1,2,3 | rev)
                else
                    # Family-agnostic: platform is last 2 components (os-arch)
                    platform=$(echo "$filename" | rev | cut -d'-' -f1,2 | rev)
                fi
            else
                # Family-agnostic: platform is last 2 components
                platform=$(echo "$filename" | rev | cut -d'-' -f1,2 | rev)
            fi

            if ! echo "$ALL_PLATFORMS" | grep -qw "$platform"; then
                echo "  Removing unsupported: $file"
                rm -f "$file"
            fi
        done
    fi
fi

echo "Done."
