#!/usr/bin/env bash
# Validate golden files for a single recipe
# Usage: ./scripts/validate-golden.sh <recipe> [--recipe <path>] [--os <linux|darwin>] [--category <embedded|registry>] [--golden-dir <path>]
#
# Compares current plan generation output against stored golden files.
# Uses fast hash comparison first, then shows diff on mismatch.
#
# Golden files are organized by category:
#   - Embedded recipes: <golden-base>/embedded/<recipe>/
#   - Registry recipes: <golden-base>/<letter>/<recipe>/
#
# Examples:
#   ./scripts/validate-golden.sh go                    # auto-detects embedded
#   ./scripts/validate-golden.sh fzf                   # auto-detects registry
#   ./scripts/validate-golden.sh go --category embedded
#   ./scripts/validate-golden.sh fzf --os linux
#   ./scripts/validate-golden.sh build-tools-system --recipe testdata/recipes/build-tools-system.toml
#   ./scripts/validate-golden.sh fzf --golden-dir r2-golden-files/plans
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
TSUKU="$REPO_ROOT/tsuku"

# Golden base - use custom dir if specified (set after argument parsing)

# TSUKU_GOLDEN_SOURCE: git (default), r2, or both
# - git: Use git-based golden files (testdata/golden/plans)
# - r2: Download from R2 and validate against those
# - both: Validate against both sources and compare results
GOLDEN_SOURCE="${TSUKU_GOLDEN_SOURCE:-git}"

# Parse arguments
RECIPE=""
CUSTOM_RECIPE_PATH=""
FILTER_OS=""
CATEGORY=""
CUSTOM_GOLDEN_DIR=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --recipe)     CUSTOM_RECIPE_PATH="$2"; shift 2 ;;
        --os)         FILTER_OS="$2"; shift 2 ;;
        --category)   CATEGORY="$2"; shift 2 ;;
        --golden-dir) CUSTOM_GOLDEN_DIR="$2"; shift 2 ;;
        -h|--help)
            echo "Usage: $0 <recipe> [--recipe <path>] [--os <linux|darwin>] [--category <embedded|registry>] [--golden-dir <path>]"
            echo ""
            echo "Validate golden files for a recipe."
            echo ""
            echo "Options:"
            echo "  --recipe <path>       Use custom recipe path (e.g., testdata/recipes/foo.toml)"
            echo "  --os <os>             Only validate golden files for the specified OS (linux or darwin)"
            echo "  --category <cat>      Force category (embedded or registry). Auto-detected if not specified."
            echo "  --golden-dir <dir>    Use custom golden files directory"
            exit 0
            ;;
        -*)        echo "Unknown flag: $1" >&2; exit 2 ;;
        *)         RECIPE="$1"; shift ;;
    esac
done

# Validate arguments
if [[ -z "$RECIPE" ]]; then
    echo "Usage: $0 <recipe> [--recipe <path>] [--os <linux|darwin>]" >&2
    exit 2
fi

# Set golden base directory
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

# Validate TSUKU_GOLDEN_SOURCE
if [[ "$GOLDEN_SOURCE" != "git" && "$GOLDEN_SOURCE" != "r2" && "$GOLDEN_SOURCE" != "both" ]]; then
    echo "Invalid TSUKU_GOLDEN_SOURCE: $GOLDEN_SOURCE (must be git, r2, or both)" >&2
    exit 2
fi

# For R2 sources, validate credentials are available
if [[ "$GOLDEN_SOURCE" == "r2" || "$GOLDEN_SOURCE" == "both" ]]; then
    if [[ -z "${R2_BUCKET_URL:-}" ]] || [[ -z "${R2_ACCESS_KEY_ID:-}" ]] || [[ -z "${R2_SECRET_ACCESS_KEY:-}" ]]; then
        echo "Error: R2 credentials required for TSUKU_GOLDEN_SOURCE=$GOLDEN_SOURCE" >&2
        echo "Required: R2_BUCKET_URL, R2_ACCESS_KEY_ID, R2_SECRET_ACCESS_KEY" >&2
        exit 2
    fi
fi

# Build tsuku if not present
if [[ ! -x "$TSUKU" ]]; then
    echo "Building tsuku..."
    (cd "$REPO_ROOT" && go build -o tsuku ./cmd/tsuku)
fi

# Download golden files from R2 for a recipe
# Creates directory structure compatible with git golden files
# Returns path to the downloaded golden directory
download_r2_golden_files() {
    local recipe="$1"
    local category="$2"
    local target_base="$3"
    local first_letter="${recipe:0:1}"

    # Determine R2 category prefix (embedded or first letter)
    local r2_category
    if [[ "$category" == "embedded" ]]; then
        r2_category="embedded"
    else
        r2_category="$first_letter"
    fi

    # Create target directory
    local target_dir
    if [[ "$category" == "embedded" ]]; then
        target_dir="$target_base/embedded/$recipe"
    else
        target_dir="$target_base/$first_letter/$recipe"
    fi
    mkdir -p "$target_dir"

    # Export AWS credentials for aws cli
    export AWS_ACCESS_KEY_ID="$R2_ACCESS_KEY_ID"
    export AWS_SECRET_ACCESS_KEY="$R2_SECRET_ACCESS_KEY"
    export AWS_ENDPOINT_URL="$R2_BUCKET_URL"

    local bucket_name="${R2_BUCKET_NAME:-tsuku-golden-registry}"
    local prefix="plans/${r2_category}/${recipe}/"

    # List all objects for this recipe and download them
    local objects
    objects=$(aws s3api list-objects-v2 \
        --bucket "$bucket_name" \
        --prefix "$prefix" \
        --query 'Contents[].Key' \
        --output text 2>/dev/null) || {
        echo "Warning: Could not list R2 objects for $recipe" >&2
        return 1
    }

    if [[ -z "$objects" || "$objects" == "None" ]]; then
        echo "Warning: No golden files found in R2 for $recipe" >&2
        return 1
    fi

    local count=0
    for key in $objects; do
        # Skip if not a .json file
        [[ "$key" == *.json ]] || continue

        # Parse key: plans/{category}/{recipe}/v{version}/{platform}.json
        # Extract version and platform
        local filename
        filename=$(basename "$key")
        local version_dir
        version_dir=$(basename "$(dirname "$key")")

        # Convert R2 structure to git structure
        # R2: plans/f/fzf/v0.60.0/linux-amd64.json
        # Git: testdata/golden/plans/f/fzf/v0.60.0-linux-amd64.json
        local version="${version_dir#v}"
        local platform="${filename%.json}"
        local git_filename="${version_dir}-${platform}.json"

        # Download file
        aws s3 cp "s3://${bucket_name}/${key}" "$target_dir/$git_filename" --quiet 2>/dev/null || {
            echo "Warning: Failed to download $key" >&2
            continue
        }
        ((count++))
    done

    if [[ $count -eq 0 ]]; then
        echo "Warning: No golden files downloaded from R2 for $recipe" >&2
        return 1
    fi

    echo "$target_dir"
    return 0
}

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
    exit 2
fi

# Create temp directory for generated files (and R2 downloads if needed)
TEMP_DIR=$(mktemp -d)
trap 'rm -rf "$TEMP_DIR"' EXIT

# Handle R2 golden source
R2_GOLDEN_DIR=""
GIT_GOLDEN_DIR="$GOLDEN_DIR"

if [[ "$GOLDEN_SOURCE" == "r2" || "$GOLDEN_SOURCE" == "both" ]]; then
    R2_TEMP_BASE="$TEMP_DIR/r2-golden"
    mkdir -p "$R2_TEMP_BASE"

    echo "Downloading golden files from R2 for $RECIPE..."
    R2_GOLDEN_DIR=$(download_r2_golden_files "$RECIPE" "$CATEGORY" "$R2_TEMP_BASE") || {
        if [[ "$GOLDEN_SOURCE" == "r2" ]]; then
            echo "Error: Failed to download golden files from R2" >&2
            exit 2
        else
            echo "Warning: R2 download failed, will only validate against git" >&2
            R2_GOLDEN_DIR=""
        fi
    }
fi

# Set GOLDEN_DIR based on source
if [[ "$GOLDEN_SOURCE" == "r2" ]]; then
    if [[ -z "$R2_GOLDEN_DIR" ]]; then
        echo "Error: R2 golden files required but not available" >&2
        exit 2
    fi
    GOLDEN_DIR="$R2_GOLDEN_DIR"
elif [[ "$GOLDEN_SOURCE" == "git" ]]; then
    # Validate git golden directory exists
    if [[ ! -d "$GOLDEN_DIR" ]]; then
        echo "No golden files found for $RECIPE" >&2
        exit 2
    fi
fi
# For "both" mode, we validate against git first, then compare with R2 later

# Validate golden directory exists (for git and both modes)
if [[ "$GOLDEN_SOURCE" != "r2" && ! -d "$GOLDEN_DIR" ]]; then
    echo "No golden files found for $RECIPE" >&2
    exit 2
fi

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
# Detection: check if second-from-last component is a known family (e.g., debian in linux-debian-amd64)
VERSIONS=$(for f in "$GOLDEN_DIR"/*.json; do
    filename=$(basename "$f" .json)
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
        echo "$filename" | rev | cut -d'-' -f3- | rev
    fi
done 2>/dev/null | sort -u || true)

if [[ -z "$VERSIONS" ]]; then
    echo "No golden files found in $GOLDEN_DIR" >&2
    exit 2
fi

# Load exclusions files
EXCLUSIONS_FILE="$REPO_ROOT/testdata/golden/exclusions.json"
CODE_VALIDATION_EXCLUSIONS_FILE="$REPO_ROOT/testdata/golden/code-validation-exclusions.json"

# Helper function to check if a recipe is excluded from code validation
# (used for mismatch comparison, not for missing file checks)
is_recipe_excluded_from_code_validation() {
    local recipe="$1"

    if [[ ! -f "$CODE_VALIDATION_EXCLUSIONS_FILE" ]]; then
        return 1
    fi

    jq -e --arg r "$recipe" \
        '.exclusions[] | select(.recipe == $r)' \
        "$CODE_VALIDATION_EXCLUSIONS_FILE" > /dev/null 2>&1
}

# Helper function to check if a platform is excluded
is_platform_excluded() {
    local recipe="$1"
    local os="$2"
    local arch="$3"
    local family="$4"

    if [[ ! -f "$EXCLUSIONS_FILE" ]]; then
        return 1
    fi

    # Build jq query based on whether family is present
    if [[ -n "$family" ]]; then
        jq -e --arg r "$recipe" --arg o "$os" --arg a "$arch" --arg f "$family" \
            '.exclusions[] | select(.recipe == $r and .platform.os == $o and .platform.arch == $a and .platform.linux_family == $f)' \
            "$EXCLUSIONS_FILE" > /dev/null 2>&1
    else
        jq -e --arg r "$recipe" --arg o "$os" --arg a "$arch" \
            '.exclusions[] | select(.recipe == $r and .platform.os == $o and .platform.arch == $a and (.platform.linux_family == null or .platform.linux_family == ""))' \
            "$EXCLUSIONS_FILE" > /dev/null 2>&1
    fi
}

# Check that all supported platforms have golden files
MISSING_PLATFORMS=()
EXCLUDED_PLATFORMS=()
for VERSION in $VERSIONS; do
    for platform_desc in $PLATFORMS; do
        # Parse platform descriptor (os:arch:family)
        os="${platform_desc%%:*}"
        rest="${platform_desc#*:}"
        arch="${rest%%:*}"
        family="${rest#*:}"

        # Skip platforms that don't match the filter OS (if specified)
        if [[ -n "$FILTER_OS" && "$os" != "$FILTER_OS" ]]; then
            continue
        fi

        # Build expected filename
        if [[ -n "$family" ]]; then
            expected_file="${VERSION}-${os}-${family}-${arch}.json"
        else
            expected_file="${VERSION}-${os}-${arch}.json"
        fi

        GOLDEN="$GOLDEN_DIR/$expected_file"
        if [[ ! -f "$GOLDEN" ]]; then
            # Check if platform is excluded
            if is_platform_excluded "$RECIPE" "$os" "$arch" "$family"; then
                EXCLUDED_PLATFORMS+=("$expected_file")
            else
                MISSING_PLATFORMS+=("$expected_file")
            fi
        fi
    done
done

# Report excluded platforms
if [[ ${#EXCLUDED_PLATFORMS[@]} -gt 0 ]]; then
    echo "Excluded platforms (see testdata/golden/exclusions.json):"
    for excluded in "${EXCLUDED_PLATFORMS[@]}"; do
        echo "  - $excluded"
    done
fi

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
    echo "" >&2
    echo "  3. Add an exclusion with a tracking issue:" >&2
    echo "     Edit testdata/golden/exclusions.json" >&2
    exit 1
fi

# Check if recipe is excluded from code validation (toolchain drift, etc.)
if is_recipe_excluded_from_code_validation "$RECIPE"; then
    reason=$(jq -r --arg r "$RECIPE" '.exclusions[] | select(.recipe == $r) | .reason' "$CODE_VALIDATION_EXCLUSIONS_FILE" | head -1)
    issue=$(jq -r --arg r "$RECIPE" '.exclusions[] | select(.recipe == $r) | .issue' "$CODE_VALIDATION_EXCLUSIONS_FILE" | head -1)
    echo "SKIPPED: $RECIPE is excluded from code validation"
    echo "  Reason: $reason"
    echo "  Tracking: $issue"
    exit 0
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

        # Skip platforms that don't match the filter OS (if specified)
        if [[ -n "$FILTER_OS" && "$os" != "$FILTER_OS" ]]; then
            continue
        fi

        # Build expected filename and actual filename
        if [[ -n "$family" ]]; then
            filename="${VERSION}-${os}-${family}-${arch}.json"
        else
            filename="${VERSION}-${os}-${arch}.json"
        fi

        GOLDEN="$GOLDEN_DIR/$filename"
        ACTUAL="$TEMP_DIR/$filename"

        # Build eval command arguments with constrained evaluation
        eval_args=(--recipe "$RECIPE_PATH" --os "$os" --arch "$arch" --version "$VERSION_NO_V" --install-deps --pin-from "$GOLDEN")
        if [[ -n "$family" ]]; then
            eval_args+=(--linux-family "$family")
        fi

        # Generate current plan with constrained evaluation (stripping non-deterministic fields)
        # The --pin-from flag extracts constraints from the golden file (pip versions,
        # go.sum, cargo.lock, etc.) to produce deterministic output.
        # Note: missing platforms already caught by pre-check above
        # Strip format_version and recipe_hash (recursively) for forward compatibility during v3->v4 format migration
        if ! "$TSUKU" eval "${eval_args[@]}" 2>/dev/null | \
            jq 'del(.generated_at, .recipe_source, .format_version) | walk(if type == "object" then del(.recipe_hash) else . end)' > "$ACTUAL"; then
            echo "Failed to generate plan for $RECIPE@$VERSION ($filename)" >&2
            continue
        fi

        # Fast hash comparison
        # Strip format_version and recipe_hash (recursively) from golden files for forward compatibility during v3->v4 migration
        GOLDEN_NORMALIZED="$TEMP_DIR/golden-$filename"
        jq 'del(.format_version) | walk(if type == "object" then del(.recipe_hash) else . end)' "$GOLDEN" > "$GOLDEN_NORMALIZED"
        GOLDEN_HASH=$(sha256sum "$GOLDEN_NORMALIZED" | cut -d' ' -f1)
        ACTUAL_HASH=$(sha256sum "$ACTUAL" | cut -d' ' -f1)

        if [[ "$GOLDEN_HASH" != "$ACTUAL_HASH" ]]; then
            MISMATCH=1
            echo "MISMATCH: $GOLDEN"
            echo "--- Expected (golden)"
            echo "+++ Actual (generated)"
            diff -u "$GOLDEN_NORMALIZED" "$ACTUAL" || true
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

# For "both" mode, compare git and R2 golden files for consistency
if [[ "$GOLDEN_SOURCE" == "both" && -n "$R2_GOLDEN_DIR" && -d "$GIT_GOLDEN_DIR" ]]; then
    echo ""
    echo "Comparing git vs R2 golden files for $RECIPE..."
    CONSISTENCY_MISMATCH=0

    # Compare all files that exist in both sources
    for git_file in "$GIT_GOLDEN_DIR"/*.json; do
        [[ -f "$git_file" ]] || continue
        filename=$(basename "$git_file")
        r2_file="$R2_GOLDEN_DIR/$filename"

        if [[ ! -f "$r2_file" ]]; then
            echo "  GIT_ONLY: $filename"
            continue
        fi

        git_hash=$(sha256sum "$git_file" | cut -d' ' -f1)
        r2_hash=$(sha256sum "$r2_file" | cut -d' ' -f1)

        if [[ "$git_hash" != "$r2_hash" ]]; then
            CONSISTENCY_MISMATCH=1
            echo "  MISMATCH: $filename"
            echo "    Git:  $git_hash"
            echo "    R2:   $r2_hash"
        else
            echo "  MATCH: $filename"
        fi
    done

    # Check for files only in R2
    for r2_file in "$R2_GOLDEN_DIR"/*.json; do
        [[ -f "$r2_file" ]] || continue
        filename=$(basename "$r2_file")
        git_file="$GIT_GOLDEN_DIR/$filename"

        if [[ ! -f "$git_file" ]]; then
            echo "  R2_ONLY: $filename"
        fi
    done

    if [[ $CONSISTENCY_MISMATCH -eq 1 ]]; then
        echo ""
        echo "Warning: Git and R2 golden files differ for $RECIPE"
        echo "This may be expected if R2 has newer generated versions."
    fi
fi

echo "Golden files for $RECIPE are up to date."
exit 0
